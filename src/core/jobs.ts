import { mkdirSync, readdirSync, writeFileSync } from "node:fs"
import { join } from "node:path"
import type { EvieEvent, Job, JobWork } from "./types.ts"

// Jobs: work that carries on after the tool call that started it has
// returned, such as a long command or, later, a subagent.
//
// A tool hands work over with `start`. From then on the core keeps track of
// it: it is listed, it can be stopped, it is stopped when Evie closes, and
// when it ends the model is told and, if the session is idle, woken to deal
// with the result. Each kind of tool decides what its work is; what is
// common to all of them is here, once.

export type Jobs = {
  start(session: string, work: JobWork): Job
  // The session's jobs, oldest first, the ended ones included.
  list(session: string): Job[]
  // Stops a running job. `by` says who asked, which decides who is told.
  stop(session: string, id: string, by: "owner" | "model"): Job | undefined
  // Stops every running job and waits for them to end. Used when Evie closes.
  stopAll(): Promise<void>
  // A tool that is waiting on work it could hand over listens here. Returns
  // a function that ends the listening.
  whileWaiting(session: string, asked: () => void): () => void
  // Asks every tool waiting in the session to hand its work over, and
  // returns how many were asked.
  background(session: string): number
}

type Running = { job: Job; work: JobWork; stoppedBy?: "owner" | "model" | "closing" }

export function createJobs(options: {
  // Where each job's output file goes: a folder per session.
  outputFolder: string
  emit(event: EvieEvent): void
  // Adds a note for the model to a session, and with `wake` starts a turn
  // for it when the session is idle.
  tell(session: string, text: string, wake: boolean): void
}): Jobs {
  const all = new Map<string, Running[]>()
  const ofSession = (session: string) => all.get(session) ?? all.set(session, []).get(session)!
  // The highest id each session has used. Every job leaves an output file,
  // so the count carries on from there when a session is continued after
  // Evie was stopped, and an id is never used for two different jobs.
  const lastId = new Map<string, number>()
  const nextId = (session: string): number => {
    if (!lastId.has(session)) {
      let highest = 0
      try {
        for (const name of readdirSync(join(options.outputFolder, session))) {
          const used = /^job-(\d+)\.log$/.exec(name)
          if (used) highest = Math.max(highest, Number(used[1]))
        }
      } catch {
        // No folder yet: this is the session's first job.
      }
      lastId.set(session, highest)
    }
    lastId.set(session, lastId.get(session)! + 1)
    return lastId.get(session)!
  }
  // A plugin's own code must not be able to break the core by throwing.
  const safely = (work: () => void) => {
    try {
      work()
    } catch {}
  }
  // What each session's turn is waiting on right now that could be moved.
  const waiting = new Map<string, Set<() => void>>()
  const announce = (job: Job) => options.emit({ type: "job", session: job.session, job: { ...job } })

  return {
    start(session, work) {
      const jobs = ofSession(session)
      // Ids count up within a session, so they are short enough to type.
      const id = String(nextId(session))
      const job: Job = {
        id,
        session,
        kind: work.kind,
        title: work.title,
        status: "running",
        started: Date.now(),
        outputFile: join(options.outputFolder, session, `job-${id}.log`),
      }
      // The file is there from the start, empty, even for work that writes nothing.
      safely(() => {
        mkdirSync(join(options.outputFolder, session), { recursive: true })
        writeFileSync(job.outputFile, "")
      })
      const running: Running = { job, work }
      jobs.push(running)
      announce(job)

      const settle = (status: "finished" | "failed", result: string) => {
        job.status = running.stoppedBy ? "stopped" : status
        job.ended = Date.now()
        announce(job)
        // The model that stopped a job already knows, and when Evie is
        // closing there is no one to tell.
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
      return { ...job }
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

    async stopAll() {
      const live = [...all.values()].flat().filter(({ job }) => job.status === "running")
      for (const { job, work } of live) {
        const running = live.find((other) => other.job === job)!
        running.stoppedBy = "closing"
        // Written now, while the log is still open, so that a session
        // continued later does not go on expecting to hear from the job.
        safely(() => options.tell(job.session, `Background job ${job.id} (${job.title}) was stopped because Evie was closed. Start it again if it is still needed.`, false))
        safely(() => work.stop())
      }
      // Work that will not stop must not keep Evie from closing.
      const patience = new Promise<void>((resolve) => setTimeout(resolve, 3000).unref())
      await Promise.race([Promise.allSettled(live.map(({ work }) => work.done)), patience])
    },
  }
}
