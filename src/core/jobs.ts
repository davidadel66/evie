import { mkdirSync, readdirSync, readFileSync, renameSync, rmSync, writeFileSync } from "node:fs"
import { stat } from "node:fs/promises"
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
  // Without `wake`, its ending is told without starting a turn.
  adopt(record: JobRecord, work: JobWork, wake: boolean): Job
  // Gives back the number and files of work left that never became a job.
  release(record: JobRecord): void
  // Records that the call waiting on work left has been told it carries
  // on as a job, which it is from now on, whenever it is taken back.
  handedOver(record: JobRecord): void
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

// `quiet` is a job whose ending does not start a turn.
type Running = { job: Job; work: JobWork; record: JobRecord; stoppedBy?: "owner" | "model" | "closing"; quiet?: true }

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

// What work that has ended rejects with when no one can tell how it ended.
export class EndNotKnown extends Error {}

const HOW: Record<Exclude<Job["status"], "running">, string> = {
  finished: "has finished",
  failed: "failed",
  stopped: "was stopped by the owner",
  unknown: "has ended, and how it ended is not known",
}

export function createJobs(options: {
  // Where each job's output file goes: a folder per session.
  outputFolder: string
  emit(event: EvieEvent): void
  // Adds a note for the model to a session, and with `wake` starts a turn
  // for it when the session is idle. `kept` is called once the note is in
  // the log, which may be later: a note is held during a turn until its next step.
  tell(session: string, text: string, wake: boolean, kept: () => void): void
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
  // The model has been told how a job ended, or knows: it is not told again
  // when Evie starts again.
  const told = (record: JobRecord) => {
    record.told = true
    save(record)
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
  // What each session's turn is waiting on right now that could be moved.
  const waiting = new Map<string, Set<() => void>>()
  const announce = (job: Job) => options.emit({ type: "job", session: job.session, job: { ...job } })

  // From here the work is a job: listed, told of when it ends, and stopped by the closing.
  function track(record: JobRecord, work: JobWork, wake = true): Job {
    const { id, session } = record
    delete record.inCall
    const job: Job = {
      id,
      session,
      kind: record.kind,
      title: record.title,
      status: "running",
      started: record.started,
      outputFile: record.outputFile,
      ...(work.subagent !== undefined && { subagent: work.subagent }),
    }
    const running: Running = { job, work, record, ...(!wake && { quiet: true as const }) }
    ofSession(session).push(running)
    save(record)
    announce(job)

    const settle = (status: "finished" | "failed" | "unknown", result: string) => {
      job.status = running.stoppedBy ? "stopped" : status
      job.ended = Date.now()
      // Left for a restart: the next start tells of it.
      if (gone) return
      announce(job)
      Object.assign(record, { status: job.status, ended: job.ended })
      save(record)
      // The model that stopped a job already knows, and when Evie is
      // closing it was told then.
      if (running.stoppedBy === "model") told(record)
      if (running.stoppedBy === "model" || running.stoppedBy === "closing") return
      const how = HOW[job.status as keyof typeof HOW]
      // A job that ended by itself wakes the session, so the result is
      // dealt with even if the turn that started it is long over. It counts
      // as told only once its note is in the log, so a restart before then
      // leaves it to be told by the next start.
      safely(() => options.tell(session, `Background job ${id} (${job.title}) ${how}.\n\n${result}`, running.stoppedBy === undefined && !running.quiet, () => told(record)))
    }
    work.done.then(
      (result) => settle("finished", result),
      (cause) => settle(cause instanceof EndNotKnown ? "unknown" : "failed", cause instanceof Error ? cause.message : String(cause)),
    )
    // Evie is closing: nothing new is left running.
    if (closing) stopForClosing(running)
    return { ...job }
  }

  // Stops a job because Evie is closing for good. The note is written now,
  // while the log is still open, so that a session continued later does
  // not go on expecting to hear from the job.
  function stopForClosing(running: Running) {
    const { job, work, record } = running
    running.stoppedBy = "closing"
    safely(() => options.tell(job.session, `Background job ${job.id} (${job.title}) was stopped because evie was closed. Start it again if it is still needed.`, false, () => told(record)))
    safely(() => work.stop())
  }

  // The next number in a session, and a record for it. Ids count up within a
  // session, so they are short enough to type. The output file is there
  // from the start, empty, even for work that writes nothing.
  function take(session: string, kind: string, title: string): JobRecord {
    const id = String(nextId(session))
    const outputFile = join(folderOf(session), jobFileName(id))
    safely(() => {
      mkdirSync(folderOf(session), { recursive: true })
      writeFileSync(outputFile, "")
    })
    return { id, session, kind, title, started: Date.now(), outputFile, status: "running", told: false, keep: {} }
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
      return track({ ...take(session, work.kind, work.title), keep: { ...work.keep } }, work)
    },

    reserve(session, { kind, title, call }) {
      const record: JobRecord = { ...take(session, kind, title), call, inCall: true }
      save(record)
      let released = false
      return {
        id: record.id,
        outputFile: record.outputFile,
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

    adopt(record, work, wake) {
      return track({ ...record, keep: { ...record.keep } }, work, wake)
    },

    release,

    handedOver(record) {
      delete record.inCall
      save(record)
    },

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
      for (const running of live) stopForClosing(running)
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

// The most of a job's output one read gives: its latest part.
const OUTPUT_READ = 64 * 1024

// A job's output from byte `after` of its file, or from the start, as the
// `read_job_output` command gives it: the last 64 KiB at most, in whole
// characters. A file shorter than `after`, as when the work began it
// again, is read from its start. A file that cannot be read, as one that
// has gone, throws, so that a client keeps what it has read.
export async function readOutput(file: string, after = 0): Promise<{ text: string; from: number; to: number }> {
  const { size } = await stat(file)
  const from = Math.max(after <= size ? after : 0, size - OUTPUT_READ)
  const bytes = await Bun.file(file).slice(from, size).bytes()
  // A cut can fall inside a character: the part of one at the start is
  // left out, and one not yet written whole at the end is left for later.
  let start = 0
  while (start < bytes.length && (bytes[start]! & 0xc0) === 0x80) start++
  let end = bytes.length
  for (let back = 1; back <= Math.min(4, end - start); back++) {
    const byte = bytes[end - back]!
    if ((byte & 0xc0) === 0x80) continue
    const length = byte >= 0xf0 ? 4 : byte >= 0xe0 ? 3 : byte >= 0xc0 ? 2 : 1
    if (length > back) end -= back
    break
  }
  return { text: new TextDecoder().decode(bytes.subarray(start, end)), from: from + start, to: from + end }
}

// The names in a folder, or none when it cannot be read, as before it is made.
function namesIn(folder: string): string[] {
  try {
    return readdirSync(folder)
  } catch {
    return []
  }
}
