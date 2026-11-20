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
  const announce = (job: Job) => options.emit({ type: "job", session: job.session, job: { ...job } })

  return {
    start(session, work) {
      const jobs = ofSession(session)
      // Ids count up within a session, so they are short enough to type.
      const id = String(jobs.length + 1)
      const job: Job = {
        id,
        session,
        kind: work.kind,
        title: work.title,
        status: "running",
        started: Date.now(),
        outputFile: join(options.outputFolder, session, `job-${id}.log`),
      }
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
        options.tell(session, `Background job ${id} (${job.title}) ${how}.\n\n${result}`, running.stoppedBy === undefined)
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
      running.work.stop()
      return { ...running.job }
    },

    async stopAll() {
      const live = [...all.values()].flat().filter(({ job }) => job.status === "running")
      for (const running of live) {
        running.stoppedBy = "closing"
        running.work.stop()
      }
      // Work that will not stop must not keep Evie from closing.
      const patience = new Promise<void>((resolve) => setTimeout(resolve, 3000).unref())
      await Promise.race([Promise.allSettled(live.map(({ work }) => work.done)), patience])
    },
  }
}
