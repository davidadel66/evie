import { useEffect, useId, useLayoutEffect, useRef, useState, useSyncExternalStore, type ReactNode } from "react"
import type { Job, ListedProject } from "../../src/core/types.ts"
import { addressOf } from "./address.ts"
import { ask, followOutput, stopJob, undo, type Following } from "./card.ts"
import { closeSheet, toggleCard, useCardShown } from "./card-shown.ts"
import { Button } from "./components/ui/button.tsx"
import { Sheet, SheetContent, SheetTitle } from "./components/ui/sheet.tsx"
import type { Connection } from "./connection.ts"
import { holdConversation, type Conversation } from "./conversation.ts"
import { CaretRight, Check, SidebarRight, X } from "./icons.tsx"
import { backgroundJobs, gitWhyNot, REQUESTS, shortFolder, stateWord, subagentsOf, undoWhyNot, type Subagent } from "./project-card.ts"
import { ProjectIcon } from "./ProjectIcon.tsx"
import { useSelected } from "./selection.ts"
import type { Sessions } from "./sessions.ts"
import { Spinner } from "./Spinner.tsx"

// The project card, on the right: the selected session's project, its
// folder and branch; what can be done with its changes; its subagents,
// each of which opens its conversation in the middle; and its background
// jobs, each of which opens to its output. At desktop width it is a column
// the owner can hide; at phone width it is a sheet over the page.
export function CardPlace({ connection, sessions }: { connection: Connection; sessions: Sessions }) {
  const shown = useCardShown()
  if (shown.wide) {
    if (!shown.column) return null
    return (
      <aside aria-label="Project" className="flex w-project-card shrink-0 flex-col border-l border-line bg-bg">
        <ProjectCard connection={connection} sessions={sessions} />
      </aside>
    )
  }
  return (
    <Sheet open={shown.sheet} onOpenChange={(open) => !open && closeSheet()}>
      <SheetContent aria-label="Project">
        <SheetTitle className="sr-only">Project</SheetTitle>
        <ProjectCard connection={connection} sessions={sessions} />
      </SheetContent>
    </Sheet>
  )
}

// The button in the session's header that shows and hides the card.
export function CardButton() {
  const shown = useCardShown()
  const open = shown.wide ? shown.column : shown.sheet
  const label = open ? "Hide the project card" : "Show the project card"
  const keys = /Mac|iPhone|iPad/.test(navigator.userAgent) ? "⌘⌥B" : "Ctrl+Alt+B"
  return (
    <Button variant="icon" aria-label={label} aria-pressed={open} title={`${label} (${keys})`} onClick={toggleCard}>
      <SidebarRight />
    </Button>
  )
}

// What the card holds for what is selected: a session, a draft of one, or nothing.
function ProjectCard({ connection, sessions }: { connection: Connection; sessions: Sessions }) {
  const list = useSyncExternalStore(sessions.subscribe, sessions.state)
  const selected = useSelected()
  const projectOf = (id: string | null) => (id === null ? null : list.projects.find((each) => each.id === id))
  if (selected === undefined) return <Empty text="No session is open." />
  if ("draft" in selected) {
    return (
      <Body>
        <Header projectId={selected.draft} project={projectOf(selected.draft)} />
        <Where projectId={selected.draft} project={projectOf(selected.draft)} />
        <p className="px-7 pt-4 text-caption text-text-muted">The rest is shown once the session's first message is sent.</p>
      </Body>
    )
  }
  if (!list.listed) return <Empty text="" />
  const summary = list.sessions.find((each) => each.session === selected.session)
  if (summary === undefined) return <Empty text="This session is not in the list." />
  return (
    <SessionCard
      key={selected.session}
      connection={connection}
      session={selected.session}
      opened={selected.subagent}
      projectId={summary.project}
      project={projectOf(summary.project)}
    />
  )
}

// The card with nothing of a session's to show, but a line saying so.
function Empty({ text }: { text: string }) {
  return (
    <Body>
      <Header projectId={null} title="Project" />
      {text !== "" && <p className="px-7 text-caption text-text-muted">{text}</p>}
    </Body>
  )
}

function Body({ children }: { children: ReactNode }) {
  return <div className="flex min-h-0 flex-1 flex-col overflow-y-auto pb-6">{children}</div>
}

// The card's top row: the project's tile and name, or that there is none,
// and the button that hides the card.
function Header({ projectId, project, title }: { projectId: string | null; project?: ListedProject | null | undefined; title?: string }) {
  const name = title ?? (projectId === null ? "No project" : (project?.name ?? "Project"))
  return (
    <header className="flex h-header-height shrink-0 items-center gap-2 pr-4 pl-7">
      {projectId !== null && <ProjectIcon project={projectId} />}
      <h2 className="min-w-0 flex-1 truncate text-title text-text">{name}</h2>
      <Button variant="icon" aria-label="Hide the project card" onClick={toggleCard}>
        <X />
      </Button>
    </header>
  )
}

// Where the project is: its folder, and the branch it is on, as rows.
function Where({ projectId, project }: { projectId: string | null; project: ListedProject | null | undefined }) {
  if (projectId === null) return null
  if (project == null) return <p className="border-t border-line px-7 pt-4 text-caption text-text-muted">The project's details have not been read yet.</p>
  return (
    <dl className="flex flex-col px-4">
      <Row label="Folder" value={shortFolder(project.folder)} whole={project.folder} path />
      <Row label="Branch" value={project.branch ?? "Not a git repository"} code={project.branch !== undefined} />
    </dl>
  )
}

// A row of a list: its label on the left and its value on the right, with
// a line on top. A long value, such as a folder outside the home folder,
// goes on over more lines rather than lose its end: a path breaks before
// one of its folders where it can.
function Row({ label, value, whole, code = true, path = false }: { label: string; value: string; whole?: string; code?: boolean; path?: boolean }) {
  const shown = path ? value.split("/").flatMap((part, at) => (at === 0 ? [part] : [<wbr key={at} />, `/${part}`])) : value
  return (
    <div className="flex min-h-row-height items-center justify-between gap-4 border-t border-line px-3 py-3">
      <dt className="shrink-0 text-body font-medium text-text-muted">{label}</dt>
      <dd title={whole ?? value} className={`min-w-0 text-right break-words ${code ? "font-mono text-code text-text" : "text-body text-text-muted"}`}>
        {shown}
      </dd>
    </div>
  )
}

// A section of the card, under a caption that says what it is.
function Section({ title, count, children }: { title: string; count?: string; children: ReactNode }) {
  return (
    <section aria-label={title} className="flex flex-col px-4 pt-6">
      <h3 className="px-3 pb-2 text-caption text-text-muted">
        {title}
        {count !== undefined && ` · ${count}`}
      </h3>
      {children}
    </section>
  )
}

// The card of a session: everything above, from the session as it stands,
// which it follows with the middle column, and as it changes.
function SessionCard({
  connection,
  session,
  opened,
  projectId,
  project,
}: {
  connection: Connection
  session: string
  // The subagent whose conversation is open in the middle, if one is.
  opened: string | undefined
  projectId: string | null
  project: ListedProject | null | undefined
}) {
  const [conversation, setConversation] = useState<Conversation>()
  useEffect(() => {
    const held = holdConversation(connection, session)
    setConversation(held.conversation)
    return held.release
  }, [connection, session])
  return (
    <Body>
      <Header projectId={projectId} project={project} />
      <Where projectId={projectId} project={project} />
      {conversation && <Live connection={connection} conversation={conversation} session={session} opened={opened} project={project} />}
    </Body>
  )
}

function Live({
  connection,
  conversation,
  session,
  opened,
  project,
}: {
  connection: Connection
  conversation: Conversation
  session: string
  opened: string | undefined
  project: ListedProject | null | undefined
}) {
  const state = useSyncExternalStore(conversation.subscribe, conversation.state)
  const connected = useSyncExternalStore(connection.subscribe, connection.state).status === "connected"
  const facts = { connected, project, turnRunning: state.turnRunning }
  const subagents = subagentsOf(state.jobs)
  const jobs = backgroundJobs(state.jobs)
  const stop = (id: string) => void stopJob(connection, conversation, session, id)
  const working = (count: number) => (count > 0 ? `${count} working` : undefined)

  return (
    <>
      <Changes connection={connection} conversation={conversation} session={session} ready={state.opened} gitWhy={gitWhyNot(facts)} undoWhy={undoWhyNot(facts)} />
      {subagents.length > 0 && (
        <Section title="Subagents" {...withCount(working(subagents.filter((each) => each.status === "running").length))}>
          <ul className="flex flex-col">
            {subagents.map((subagent) => (
              <SubagentRow key={subagent.session} parent={session} subagent={subagent} open={subagent.session === opened} stop={() => stop(subagent.job)} />
            ))}
          </ul>
        </Section>
      )}
      {jobs.running.length + jobs.ended.length > 0 && (
        <Section title="Background jobs" {...withCount(working(jobs.running.length))}>
          <Jobs connection={connection} session={session} running={jobs.running} ended={jobs.ended} stop={stop} />
        </Section>
      )}
    </>
  )
}
const withCount = (count: string | undefined) => (count === undefined ? {} : { count })

// Commit, push and pull request, each a request in words sent to evie, and
// undo, each disabled with why while it does not apply.
function Changes({
  connection,
  conversation,
  session,
  ready,
  gitWhy,
  undoWhy,
}: {
  connection: Connection
  conversation: Conversation
  session: string
  // Whether the session has been opened, and so how it stands is known.
  ready: boolean
  gitWhy: string | undefined
  undoWhy: string | undefined
}) {
  // Why a request could not be sent, until the next is.
  const [refused, setRefused] = useState<string>()
  const [sending, setSending] = useState(false)
  const [undoing, setUndoing] = useState(false)
  const gitNote = useId()
  const undoNote = useId()
  const why = ready ? (gitWhy ?? refused) : undefined
  async function request(kind: keyof typeof REQUESTS) {
    setSending(true)
    setRefused(await ask(connection, session, REQUESTS[kind]))
    setSending(false)
  }
  async function undoLast() {
    setUndoing(true)
    await undo(connection, conversation, session)
    setUndoing(false)
  }
  const gitOff = !ready || gitWhy !== undefined || sending
  const undoOff = !ready || undoWhy !== undefined || undoing
  return (
    <Section title="Changes">
      {/* Each button's own padding brings its word to the card's 28px edge. */}
      <div className="flex flex-wrap gap-2">
        {(["commit", "push", "pull request"] as const).map((kind) => (
          <Button key={kind} disabled={gitOff} aria-describedby={why === undefined ? undefined : gitNote} onClick={() => void request(kind)} className="px-3">
            {{ commit: "Commit", push: "Push", "pull request": "Pull request" }[kind]}
          </Button>
        ))}
      </div>
      {why !== undefined && (
        <p id={gitNote} className="px-3 pt-2 text-caption text-text-muted">
          {why}
        </p>
      )}
      <div className="flex pt-3">
        <Button disabled={undoOff} aria-describedby={ready && undoWhy !== undefined ? undoNote : undefined} onClick={() => void undoLast()} className="px-3">
          Undo last change
        </Button>
      </div>
      {ready && undoWhy !== undefined && (
        <p id={undoNote} className="px-3 pt-2 text-caption text-text-muted">
          {undoWhy}
        </p>
      )}
    </Section>
  )
}

// How a job or a subagent stands, in its word, as the design system's
// Status says it: Working with a spinner, Done with a check, Failed, and
// the rest in quiet words.
export function StateWord({ status }: { status: Job["status"] }) {
  const word = stateWord(status)
  if (status === "running") {
    return (
      <span className="flex shrink-0 items-center gap-1 text-caption text-status-working">
        <Spinner />
        {word}
      </span>
    )
  }
  if (status === "finished") {
    return (
      <span className="flex shrink-0 items-center gap-1 text-caption text-status-done">
        <Check className="size-icon-sm" />
        {word}
      </span>
    )
  }
  return <span className={`shrink-0 text-caption ${status === "failed" ? "text-status-failed" : "text-text-muted"}`}>{word}</span>
}

// A subagent, as a row that opens its conversation in the middle, with
// Stop while it works. The one open is raised.
function SubagentRow({ parent, subagent, open, stop }: { parent: string; subagent: Subagent; open: boolean; stop(): void }) {
  return (
    <li className={`flex h-row-height items-center border-t border-line ${open ? "bg-surface-raised" : ""}`}>
      <a href={addressOf({ session: parent, subagent: subagent.session })} aria-current={open ? "page" : undefined} className="flex h-full min-w-0 flex-1 items-center gap-3 px-3">
        <span className="min-w-0 flex-1 truncate text-body font-medium text-text">{subagent.name}</span>
        <StateWord status={subagent.status} />
      </a>
      {subagent.status === "running" && (
        <Button variant="quiet" aria-label={`Stop ${subagent.name}`} onClick={stop}>
          Stop
        </Button>
      )}
    </li>
  )
}

// The session's background jobs: those running, then those that have
// ended, folded away under their count. Each opens to its output.
function Jobs({ connection, session, running, ended, stop }: { connection: Connection; session: string; running: Job[]; ended: Job[]; stop(id: string): void }) {
  const [open, setOpen] = useState<ReadonlySet<string>>(new Set())
  const [showEnded, setShowEnded] = useState(false)
  const toggle = (id: string) =>
    setOpen((was) => {
      const now = new Set(was)
      if (!now.delete(id)) now.add(id)
      return now
    })
  const row = (job: Job) => <JobRow key={job.id} connection={connection} session={session} job={job} open={open.has(job.id)} toggle={() => toggle(job.id)} stop={() => stop(job.id)} />
  return (
    <ul className="flex flex-col">
      {running.map(row)}
      {ended.length > 0 && (
        <li className="border-t border-line">
          <button type="button" aria-expanded={showEnded} onClick={() => setShowEnded(!showEnded)} className="flex h-row-height w-full items-center gap-2 px-3 text-body text-text-muted hover:text-text">
            {ended.length} ended
            <Caret open={showEnded} />
          </button>
        </li>
      )}
      {showEnded && ended.map(row)}
    </ul>
  )
}

// A job, as a row that opens to its output, with Stop while it runs.
function JobRow({ connection, session, job, open, toggle, stop }: { connection: Connection; session: string; job: Job; open: boolean; toggle(): void; stop(): void }) {
  return (
    <li className="flex flex-col border-t border-line">
      <div className="flex h-row-height items-center">
        <button type="button" aria-expanded={open} onClick={toggle} title={job.title} className="flex h-full min-w-0 flex-1 items-center gap-2 px-3 text-left">
          <Caret open={open} />
          <span className="min-w-0 flex-1 truncate font-mono text-code text-text">{job.title}</span>
          <StateWord status={job.status} />
        </button>
        {job.status === "running" && (
          <Button variant="quiet" aria-label={`Stop job ${job.id}`} onClick={stop}>
            Stop
          </Button>
        )}
      </div>
      {open && <JobOutput connection={connection} session={session} job={job} />}
    </li>
  )
}

function Caret({ open }: { open: boolean }) {
  return <CaretRight className={`size-icon-sm shrink-0 text-text-muted transition-transform motion-reduce:transition-none ${open ? "rotate-90" : ""}`} />
}

// A job's output, followed while it runs. It stays at its end as it
// grows, unless the owner has scrolled up in it.
function JobOutput({ connection, session, job }: { connection: Connection; session: string; job: Job }) {
  const running = useRef(job.status === "running")
  running.current = job.status === "running"
  const [following, setFollowing] = useState<Following>()
  useEffect(() => {
    const followed = followOutput(connection, session, job.id, { running: () => running.current })
    setFollowing(followed)
    return () => followed.close()
  }, [connection, session, job.id])
  if (following === undefined) return null
  return <Output following={following} running={job.status === "running"} />
}

// How near its end, in pixels, counts as at its end, so that a last line
// wrapping does not count as scrolling up.
const AT_END = 8

function Output({ following, running }: { following: Following; running: boolean }) {
  const { output, problem } = useSyncExternalStore(following.subscribe, following.state)
  const box = useRef<HTMLPreElement>(null)
  const stuck = useRef(true)
  useLayoutEffect(() => {
    if (stuck.current && box.current) box.current.scrollTop = box.current.scrollHeight
  })
  const quiet = (text: string) => <p className="text-caption text-text-muted">{text}</p>
  return (
    <div className="flex flex-col gap-1 px-3 pb-3">
      {problem !== undefined && quiet(problem)}
      {output?.cut && quiet("Earlier output is not shown.")}
      {output === undefined && problem === undefined && quiet("Reading its output.")}
      {output?.text === "" && quiet(running ? "No output yet." : "It wrote no output.")}
      {output !== undefined && output.text !== "" && (
        <pre
          ref={box}
          onScroll={() => {
            const at = box.current!
            stuck.current = at.scrollHeight - at.scrollTop - at.clientHeight <= AT_END
          }}
          className="max-h-job-output overflow-auto font-mono text-code break-words whitespace-pre-wrap text-text-muted"
        >
          {output.text}
        </pre>
      )}
    </div>
  )
}
