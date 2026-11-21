import { mkdirSync, readdirSync, readFileSync, renameSync, rmSync, writeFileSync } from "node:fs"
import { join } from "node:path"
import type { EvieEvent, Job, JobRecord, JobWork, ReservedJob } from "./types.ts"

// Jobs: work that carries on after the tool call that started it has
// returned, such as a long command or a subagent.
//
// A tool hands work over with `start`. From then on the core keeps track of
// it: it is listed, it can be stopped, it is stopped when Evie closes for
// good, and when it ends the model is told and, if the session is idle,
// woken to deal with the result. Each kind of tool decides what its work is;
// what is common to all of them is here, once.
//
// Each job has a record in a file beside its output, so that when Evie
// restarts, the next start finds the jobs still running and those that
// ended with no one there to tell, and the plugin that started each picks
// it up again. Work a call waits on that may become a job, such as a
// command, takes its number and record from the start with `reserve`.

export type Jobs = {
  start(session: string, work: JobWork): Job
  reserve(session: string, work: { kind: string; title: string; call: string }): ReservedJob
  // The records earlier runs of Evie left of jobs the model has not been
  // told the end of, in every session or in one.
  left(session?: string): JobRecord[]
  // Takes back a job an earlier run left, with the work that picks it up.
  adopt(record: JobRecord, work: JobWork): Job
  // Gives back the number and files of work left that never became a job.
  release(record: JobRecord): void
  // The session's jobs, oldest first, the ended ones included.
  list(session: string): Job[]
  // Stops a running job. `by` says who asked, which decides who is told.
  stop(session: string, id: string, by: "owner" | "model"): Job | undefined
  // Stops every running job and waits for them to end. Used when Evie
  // closes for good. Each is marked and noted as stopped by the closing at
  // once, before the returned promise is waited for, and work handed over
  // from then on is stopped as it arrives.
  stopAll(): Promise<void>
  // Lets every job go on as it is, for a restart: none is stopped, and
  // nothing more is told or recorded of any, so the next start finds each
  // as it was.
  leave(): void
  // Settles when the jobs now running in a session have ended.
  whenEnded(session: string): Promise<void>
  // A tool that is waiting on work it could hand over listens here. Returns
  // a function that ends the listening.
  whileWaiting(session: string, asked: () => void): () => void
  // Asks every tool waiting in the session to hand its work over, and
  // returns how many were asked.
  background(session: string): number
}

type Running = { job: Job; work: JobWork; record: JobRecord; stoppedBy?: "owner" | "model" | "closing" }

// The name of a job's output file, in its session's folder. Its record and
// any file its plugin keeps beside it share the name, with other endings.
export const jobFileName = (id: string) => `job-${id}.log`

// The id of the job whose file has this name, its output, its record or
// another of its files, or undefined when it is none of a job's files.
export function jobOfFile(name: string): number | undefined {
  const match = /^job-(\d+)\.[a-z]+$/.exec(name)
  return match ? Number(match[1]) : undefined
}

const RECORD = /^job-(\d+)\.json$/

export function createJobs(options: {
  // Where each job's output file goes: a folder per session.
  outputFolder: string
  emit(event: EvieEvent): void
  // Adds a note for the model to a session, and with `wake` starts a turn
  // for it when the session is idle.
  tell(session: string, text: string, wake: boolean): void
}): Jobs {
  const all = new Map<string, Running[]>()
  let closing = false
  // Set once Evie has let its jobs go for a restart.
  let gone = false
  const ofSession = (session: string) => all.get(session) ?? all.set(session, []).get(session)!
  const folderOf = (session: string) => join(options.outputFolder, session)
  // The highest id each session has used. Every job leaves its files, so
  // the count carries on from there when a session is continued after Evie
  // was stopped, and an id is never used for two different jobs.
  const lastId = new Map<string, number>()
  const nextId = (session: string): number => {
    if (!lastId.has(session)) {
      let highest = 0
      for (const name of namesIn(folderOf(session))) highest = Math.max(highest, jobOfFile(name) ?? 0)
      lastId.set(session, highest)
    }
    lastId.set(session, lastId.get(session)! + 1)
    return lastId.get(session)!
  }
  // A plugin's own code must not be able to break the core by throwing, and
  // nor must a file that cannot be written.
  const safely = (work: () => void) => {
    try {
      work()
    } catch {}
  }
  // Written whole under another name first, so a record is never found half written.
  const save = (record: JobRecord) => {
    if (gone) return
    safely(() => {
      const { outputFile: _, ...kept } = record
      const file = join(folderOf(record.session), `job-${record.id}.json`)
      writeFileSync(`${file}.new`, JSON.stringify(kept))
      renameSync(`${file}.new`, file)
    })
  }
  // What the tool's call is still waiting on right now, in each session, that could be moved.
  const waiting = new Map<string, Set<() => void>>()
  const announce = (job: Job) => options.emit({ type: "job", session: job.session, job: { ...job } })

  // From here the work is a job: listed, told of when it ends, and stopped by the closing.
  function track(record: JobRecord, work: JobWork): Job {
    const { id, session } = record
    delete record.inCall
    const job: Job = { id, session, kind: record.kind, title: record.title, status: "running", started: record.started, outputFile: record.outputFile }
    const running: Running = { job, work, record }
    ofSession(session).push(running)
    save(record)
    announce(job)

    const settle = (status: "finished" | "failed", result: string) => {
      job.status = running.stoppedBy ? "stopped" : status
      job.ended = Date.now()
      // Left for a restart: the next start tells of it.
      if (gone) return
      announce(job)
      Object.assign(record, { status: job.status, ended: job.ended, told: true })
      save(record)
      // The model that stopped a job already knows, and when Evie is
      // closing it was told then.
      if (running.stoppedBy === "model" || running.stoppedBy === "closing") return
      const how = job.status === "stopped" ? "was stopped by the owner" : job.status === "failed" ? "failed" : "has finished"
      // A job that ended by itself wakes the session, so the result is
      // dealt with even if the turn that started it is long over.
      safely(() => options.tell(session, `Background job ${id} (${job.title}) ${how}.\n\n${result}`, running.stoppedBy === undefined))
    }
    work.done.then(
      (result) => settle("finished", result),
      (cause) => settle("failed", cause instanceof Error ? cause.message : String(cause)),
    )
    // Evie is closing: nothing new is left running.
    if (closing) {
      running.stoppedBy = "closing"
      safely(() => work.stop())
    }
    return { ...job }
  }

  // Removes every file of a job, and gives its number back when it is the last one taken.
  function release(record: JobRecord) {
    if (gone) return
    const folder = folderOf(record.session)
    for (const name of namesIn(folder)) if (jobOfFile(name) === Number(record.id)) safely(() => rmSync(join(folder, name), { force: true }))
    if (lastId.get(record.session) === Number(record.id)) lastId.set(record.session, Number(record.id) - 1)
  }

  return {
    start(session, work) {
      // Ids count up within a session, so they are short enough to type.
      const id = String(nextId(session))
      const outputFile = join(folderOf(session), jobFileName(id))
      // The file is there from the start, empty, even for work that writes nothing.
      safely(() => {
        mkdirSync(folderOf(session), { recursive: true })
        writeFileSync(outputFile, "")
      })
      return track({ id, session, kind: work.kind, title: work.title, started: Date.now(), outputFile, status: "running", told: false, keep: { ...work.keep } }, work)
    },

    reserve(session, { kind, title, call }) {
      const id = String(nextId(session))
      const outputFile = join(folderOf(session), jobFileName(id))
      safely(() => {
        mkdirSync(folderOf(session), { recursive: true })
        writeFileSync(outputFile, "")
      })
      const record: JobRecord = { id, session, kind, title, started: Date.now(), outputFile, call, inCall: true, status: "running", told: false, keep: {} }
      save(record)
      let released = false
      return {
        id,
        outputFile,
        keep(data) {
          if (released) return
          Object.assign(record.keep, data)
          save(record)
        },
        start(work) {
          Object.assign(record, { kind: work.kind, title: work.title })
          Object.assign(record.keep, work.keep)
          return track(record, work)
        },
        release() {
          released = true
          release(record)
        },
      }
    },

    left(session) {
      const found: JobRecord[] = []
      for (const each of session === undefined ? namesIn(options.outputFolder) : [session]) {
        for (const name of namesIn(folderOf(each))) {
          const id = RECORD.exec(name)?.[1]
          if (id === undefined) continue
          // A record that cannot be read is left alone: there is nothing to go on.
          try {
            const record = JSON.parse(readFileSync(join(folderOf(each), name), "utf8")) as JobRecord
            if (record.told === false) found.push({ ...record, keep: record.keep ?? {}, id, session: each, outputFile: join(folderOf(each), jobFileName(id)) })
          } catch {}
        }
      }
      return found
    },

    adopt(record, work) {
      return track({ ...record, keep: { ...record.keep } }, work)
    },

    release,

    list(session) {
      return (all.get(session) ?? []).map(({ job }) => ({ ...job }))
    },

    stop(session, id, by) {
      const running = (all.get(session) ?? []).find(({ job }) => job.id === id)
      if (!running || running.job.status !== "running") return running && { ...running.job }
      running.stoppedBy = by
      safely(() => running.work.stop())
      return { ...running.job }
    },

    whileWaiting(session, asked) {
      const listeners = waiting.get(session) ?? waiting.set(session, new Set()).get(session)!
      listeners.add(asked)
      return () => void listeners.delete(asked)
    },

    background(session) {
      const listeners = [...(waiting.get(session) ?? [])]
      for (const asked of listeners) safely(asked)
      return listeners.length
    },

    stopAll() {
      closing = true
      const live = [...all.values()].flat().filter(({ job }) => job.status === "running")
      for (const running of live) {
        const { job, work, record } = running
        running.stoppedBy = "closing"
        // Written now, while the log is still open, so that a session
        // continued later does not go on expecting to hear from the job.
        safely(() => options.tell(job.session, `Background job ${job.id} (${job.title}) was stopped because Evie was closed. Start it again if it is still needed.`, false))
        record.told = true
        save(record)
        safely(() => work.stop())
      }
      // Work that will not stop must not keep Evie from closing.
      const patience = new Promise<void>((resolve) => setTimeout(resolve, 3000).unref())
      return Promise.race([Promise.allSettled(live.map(({ work }) => work.done)).then(() => {}), patience])
    },

    leave() {
      gone = true
    },

    async whenEnded(session) {
      await Promise.allSettled((all.get(session) ?? []).filter(({ job }) => job.status === "running").map(({ work }) => work.done))
    },
  }
}

// The names in a folder, or none when it cannot be read, as before it is made.
function namesIn(folder: string): string[] {
  try {
    return readdirSync(folder)
  } catch {
    return []
  }
}
