import { useEffect, useId, useLayoutEffect, useRef, useState, useSyncExternalStore, type ReactNode } from "react"
import type { Job, ListedProject, PullRequest } from "../../src/core/types.ts"
import { addressOf } from "./address.ts"
import { followOutput, followPullRequest, followSubagents, request, stopJob, undo, type Following, type Lineage } from "./card.ts"
import { closeSheet, toggleCard, useCardShown } from "./card-shown.ts"
import { Counts } from "./ChangedFiles.tsx"
import { holdChanges, type Changes as SessionChanges } from "./changes.ts"
import { Button } from "./components/ui/button.tsx"
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuLabel, DropdownMenuTrigger } from "./components/ui/dropdown-menu.tsx"
import { sessionLine } from "./diff.ts"
import { openDiff, useDiffShown } from "./diff-shown.ts"
import { Sheet, SheetContent, SheetTitle } from "./components/ui/sheet.tsx"
import type { Connection } from "./connection.ts"
import { holdConversation, startedHere, type Conversation } from "./conversation.ts"
import { FolderPath } from "./FolderPath.tsx"
import { ArrowSquareOut, CaretDown, CaretRight, Check, SidebarRight, X } from "./icons.tsx"
import { backgroundJobs, branchWords, gitWhyNot, stateWord, subagentsOf, undoWhyNot, type CardFacts, type Subagent } from "./project-card.ts"
import { ProjectIcon } from "./ProjectIcon.tsx"
import { checkLook, checksSummary, checkWord, pullRequestWord } from "./pull-request.ts"
import { addressOfWorkspace } from "./workspace.ts"
import { select, useSelected } from "./selection.ts"
import type { Sessions } from "./sessions.ts"
import { Spinner } from "./Spinner.tsx"

// The project card, on the right: the selected session's project, its
// folder and branch; what can be done with its changes; its subagents,
// each of which opens its conversation in the middle; and its background
// jobs, each of which opens to its output. At desktop width it is a column
// the owner can hide; narrower, it is a sheet over the page, as T3 Code's
// right panel is, which goes once something is picked in it or elsewhere.
export function CardPlace({ connection, sessions }: { connection: Connection; sessions: Sessions }) {
  const shown = useCardShown()
  const diff = useDiffShown()
  const selected = useSelected()
  useEffect(() => {
    if (!shown.wide) closeSheet()
  }, [selected])
  if (shown.wide) {
    // The diff panel, while it is open, is in the card's place.
    if (!shown.column || diff.open) return null
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

// The button in the middle column's header that shows and hides the card.
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
        <Where connection={connection} projectId={selected.draft} project={projectOf(selected.draft)} />
        <p className="px-7 pt-4 text-caption text-text-muted">The rest is shown once the session's first message is sent.</p>
      </Body>
    )
  }
  if (!list.listed) return <Empty text="" />
  // A session this page has just started is shown before the list has it.
  const summary = list.sessions.find((each) => each.session === selected.session)
  const started = startedHere(selected.session)
  if (summary === undefined && started === undefined) return <Empty text="This session is not in the list." />
  const projectId = summary !== undefined ? summary.project : started!
  return <SessionCard key={selected.session} connection={connection} session={selected.session} opened={selected.subagent} projectId={projectId} project={projectOf(projectId)} />
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
      {projectId === null ? (
        <h2 className="min-w-0 flex-1 truncate text-title text-text">{name}</h2>
      ) : (
        // The project's tile and name open its workspace.
        <h2 className="flex min-w-0 flex-1 text-title text-text">
          <a href={addressOfWorkspace({ workspace: projectId, tab: "sessions" })} title={`Open the ${name} workspace`} className="flex h-control min-w-0 items-center gap-2">
            <ProjectIcon project={projectId} />
            <span className="min-w-0 truncate">{name}</span>
          </a>
        </h2>
      )}
      <Button variant="icon" aria-label="Hide the project card" onClick={toggleCard}>
        <X />
      </Button>
    </header>
  )
}

// Where the project is: its folder, and the branch it is on, as rows, and
// the branch's pull request, when it has one. A session with no project
// has none, which the header says.
function Where({ connection, projectId, project }: { connection: Connection; projectId: string | null; project: ListedProject | null | undefined }) {
  if (projectId === null) return null
  if (project == null) return <p className="border-t border-line px-7 pt-4 text-caption text-text-muted">The project's details have not been read yet.</p>
  const branch = branchWords(project)
  return (
    <dl className="flex flex-col px-4">
      <Row label="Folder">
        <dd className="min-w-0">
          <FolderPath folder={project.folder} className="text-text" />
        </dd>
      </Row>
      <Row label="Branch">
        <dd title={branch.text} className={`min-w-0 truncate ${branch.branch ? "font-mono text-code text-text" : "text-body text-text-muted"}`}>
          {branch.text}
        </dd>
      </Row>
      {project.branch !== undefined && <PullRequestRow key={`${project.id} ${project.branch}`} connection={connection} project={project.id} />}
    </dl>
  )
}

// A row of a list: its label on the left and its value on the right, on
// one line, with a line on top.
function Row({ label, children }: { label: string; children: ReactNode }) {
  return (
    <div className="flex h-row-height items-center justify-between gap-4 border-t border-line px-3">
      <dt className="shrink-0 text-body font-medium text-text-muted">{label}</dt>
      {children}
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
  const [followed, setFollowed] = useState<{ conversation: Conversation; lineage: Lineage; changes: SessionChanges }>()
  useEffect(() => {
    const held = holdConversation(connection, session)
    const lineage = followSubagents(connection, session)
    // With the whole session's files, which the card sums up.
    const changes = holdChanges(connection, session, { whole: true })
    setFollowed({ conversation: held.conversation, lineage, changes: changes.changes })
    return () => {
      changes.release()
      held.release()
      lineage.close()
    }
  }, [connection, session])
  return (
    <Body>
      <Header projectId={projectId} project={project} />
      <Where connection={connection} projectId={projectId} project={project} />
      {followed && <Live connection={connection} {...followed} session={session} opened={opened} project={project} />}
    </Body>
  )
}

function Live({
  connection,
  conversation,
  lineage,
  changes,
  session,
  opened,
  project,
}: {
  connection: Connection
  conversation: Conversation
  lineage: Lineage
  changes: SessionChanges
  session: string
  opened: string | undefined
  project: ListedProject | null | undefined
}) {
  const state = useSyncExternalStore(conversation.subscribe, conversation.state)
  const { listed } = useSyncExternalStore(lineage.subscribe, lineage.state)
  const connected = useSyncExternalStore(connection.subscribe, connection.state).status === "connected"
  const facts = {
    connected,
    session: state.opened ? "open" : state.problem !== undefined ? "failed" : "opening",
    project,
    turnRunning: state.turnRunning,
  } as const
  const subagents = subagentsOf(listed, state.jobs)
  const jobs = backgroundJobs(state.jobs)
  // Why the last Stop could not be asked for, until the next.
  const [stopRefused, setStopRefused] = useState<string>()
  const stop = (id: string) => void stopJob(connection, session, id).then(setStopRefused)
  const working = (count: number) => (count > 0 ? `${count} working` : undefined)
  const refusal = stopRefused !== undefined && <p className="px-3 pb-2 text-caption text-text-muted">{stopRefused}</p>

  return (
    <>
      <Changes conversation={conversation} changes={changes} session={session} opened={opened} facts={facts} />
      {subagents.length > 0 && (
        <Section title="Subagents" {...withCount(working(subagents.filter((each) => each.status === "running").length))}>
          {refusal}
          <ul className="flex flex-col">
            {subagents.map((subagent) => (
              <SubagentRow key={subagent.session} parent={session} subagent={subagent} open={subagent.session === opened} stop={(job) => stop(job)} />
            ))}
          </ul>
        </Section>
      )}
      {jobs.running.length + jobs.ended.length > 0 && (
        <Section title="Background jobs" {...withCount(working(jobs.running.length))}>
          {subagents.length === 0 && refusal}
          <Jobs connection={connection} session={session} running={jobs.running} ended={jobs.ended} stop={stop} />
        </Section>
      )}
    </>
  )
}
const withCount = (count: string | undefined) => (count === undefined ? {} : { count })

// How long a git button's request keeps the buttons off when no turn
// starts for it, in milliseconds.
const REQUEST_WAIT = 5000

// Commit, push and pull request, each a request in words sent to evie, and
// undo, each disabled with why while it does not apply. With a subagent's
// conversation open in the middle, a press goes back to the session's own
// first, so that what it says is in view.
function Changes({
  conversation,
  changes,
  session,
  opened,
  facts,
}: {
  conversation: Conversation
  changes: SessionChanges
  session: string
  opened: string | undefined
  facts: Omit<CardFacts, "requested">
}) {
  // Why a request could not be sent, until the next is.
  const [refused, setRefused] = useState<string>()
  const [sending, setSending] = useState(false)
  // A request sent that evie has not started on: one at a time.
  const [requested, setRequested] = useState(false)
  const [undoing, setUndoing] = useState(false)
  useEffect(() => {
    if (facts.turnRunning) setRequested(false)
  }, [facts.turnRunning])
  useEffect(() => {
    if (!requested) return
    const timer = setTimeout(() => setRequested(false), REQUEST_WAIT)
    return () => clearTimeout(timer)
  }, [requested])
  const gitNote = useId()
  const undoNote = useId()
  const gitWhy = gitWhyNot({ ...facts, requested })
  const undoWhy = undoWhyNot({ ...facts, requested })
  const why = gitWhy ?? refused
  const back = () => opened !== undefined && select({ session })
  async function ask(kind: Parameters<typeof request>[1]) {
    back()
    setSending(true)
    const failed = await request(conversation, kind)
    setRefused(failed)
    setRequested(failed === undefined)
    setSending(false)
  }
  async function undoLast() {
    back()
    setUndoing(true)
    await undo(conversation)
    setUndoing(false)
  }
  return (
    <Section title="Changes">
      <SessionChanged changes={changes} />
      {/* Each button's own padding brings its word to the card's 28px edge. */}
      <div className="flex flex-wrap gap-2">
        {(["commit", "push", "pull request"] as const).map((kind) => (
          <Button key={kind} disabled={gitWhy !== undefined || sending} aria-describedby={why === undefined ? undefined : gitNote} onClick={() => void ask(kind)} className="px-3">
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
        <Button disabled={undoWhy !== undefined || undoing} aria-describedby={undoWhy === undefined ? undefined : undoNote} onClick={() => void undoLast()} className="px-3">
          Undo last change
        </Button>
      </div>
      {undoWhy !== undefined && (
        <p id={undoNote} className="px-3 pt-2 text-caption text-text-muted">
          {undoWhy}
        </p>
      )}
    </Section>
  )
}

// What the session has changed, in one row, as T3 Code's details card
// says it: how many files, and lines added and removed, from before its
// first change to now; and Open diff, which opens the diff panel on the
// whole session.
function SessionChanged({ changes }: { changes: SessionChanges }) {
  const { listed } = useSyncExternalStore(changes.subscribe, changes.state)
  const line = sessionLine(listed)
  const keys = /Mac|iPhone|iPad/.test(navigator.userAgent) ? "⌘D" : "Ctrl+D"
  return (
    <div className="mb-3 flex min-h-row-height items-center gap-2 border-t border-b border-line pl-3">
      <span className="min-w-0 flex-1 py-2 text-body text-text-muted">{typeof line === "string" ? line : line.files}</span>
      {typeof line !== "string" && <Counts added={line.added} removed={line.removed} />}
      <Button variant="quiet" title={`Open the diff (${keys})`} onClick={() => openDiff({ scope: "session" })}>
        Open diff
      </Button>
    </div>
  )
}

// The branch's pull request, as T3 Code's details card shows it: its title,
// which opens it on GitHub; its number and state in words; and its checks
// summed up, which open a menu of each check, with how it stands and a
// link to its details. Read when the card opens and each minute while it
// is shown. Nothing is shown when there is none, or the server cannot ask
// GitHub.
function PullRequestRow({ connection, project }: { connection: Connection; project: string }) {
  const [followed, setFollowed] = useState<ReturnType<typeof followPullRequest>>()
  useEffect(() => {
    const following = followPullRequest(connection, project, { window, shown: () => document.visibilityState === "visible" })
    setFollowed(following)
    return following.close
  }, [connection, project])
  const pull = useSyncExternalStore(followed?.subscribe ?? noPull.subscribe, followed?.state ?? noPull.state)
  if (pull == null) return null
  return (
    <div className="flex flex-col border-t border-line px-3 pt-2 pb-1">
      <div className="flex min-h-control items-center justify-between gap-4">
        <dt className="shrink-0 text-body font-medium text-text-muted">Pull request</dt>
        <dd className="shrink-0 text-body text-text">
          #{pull.number} <span className="text-text-muted">·</span> {pullRequestWord(pull.state)}
        </dd>
      </div>
      <dd>
        {/* The server gives only an https link, and none for any other. */}
        {pull.url !== undefined ? (
          <a href={pull.url} target="_blank" rel="noreferrer" title={`Open pull request #${pull.number} on GitHub`} className="flex min-h-control items-start gap-2 py-2 text-body font-medium text-text hover:underline">
            <span className="line-clamp-2 min-w-0 flex-1 break-words">{pull.title}</span>
            <ArrowSquareOut className="mt-1 size-icon-sm shrink-0 text-text-muted" />
            <span className="sr-only">(opens GitHub)</span>
          </a>
        ) : (
          <p className="line-clamp-2 py-2 text-body font-medium break-words text-text">{pull.title}</p>
        )}
      </dd>
      {pull.checks.length > 0 && (
        <dd>
          <Checks checks={pull.checks} />
        </dd>
      )}
    </div>
  )
}
const noPull = { subscribe: () => () => {}, state: () => undefined }

function Checks({ checks }: { checks: PullRequest["checks"] }) {
  const summary = checksSummary(checks)
  const failing = checks.some((check) => check.state === "failed" || check.state === "cancelled")
  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button variant="quiet" aria-label={`Checks: ${summary}`} className={`-ml-3 gap-1 ${failing ? "text-status-failed hover:text-status-failed" : ""}`}>
          {summary}
          <CaretDown className="size-icon-sm" />
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="start" className="max-w-card-sheet">
        <DropdownMenuLabel>{summary}</DropdownMenuLabel>
        {checks.map((check, at) => {
          const row = (
            <>
              <span className="min-w-0 flex-1 truncate">{check.name}</span>
              <CheckState state={check.state} />
              {check.url !== undefined && <ArrowSquareOut className="text-text-muted" />}
            </>
          )
          return check.url !== undefined ? (
            <DropdownMenuItem key={at} asChild className="gap-3">
              <a href={check.url} target="_blank" rel="noreferrer" title={`${check.name}: open its details`}>
                {row}
              </a>
            </DropdownMenuItem>
          ) : (
            <DropdownMenuItem key={at} disabled className="gap-3">
              {row}
            </DropdownMenuItem>
          )
        })}
      </DropdownMenuContent>
    </DropdownMenu>
  )
}

// How a check stands, as the design system's Status draws a state: passed
// with a check, in Done's colour; failed with its mark, in Failed's;
// running with the spinner, as Working; the rest in quiet words.
function CheckState({ state }: { state: PullRequest["checks"][number]["state"] }) {
  const look = checkLook(state)
  const word = checkWord(state)
  if (look === "done") {
    return (
      <span className="flex shrink-0 items-center gap-1 text-caption text-status-done">
        <Check className="size-icon-sm" />
        {word}
      </span>
    )
  }
  if (look === "failed") {
    return (
      <span className="flex shrink-0 items-center gap-1 text-caption text-status-failed">
        <X className="size-icon-sm" />
        {word}
      </span>
    )
  }
  if (look === "working") {
    return (
      <span className="flex shrink-0 items-center gap-1 text-caption text-status-working">
        <Spinner />
        {word}
      </span>
    )
  }
  return <span className="shrink-0 text-caption text-text-muted">{word}</span>
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

// A subagent, as a row whose name, on a line of its own, opens its
// conversation in the middle; under it, how it stands, with Stop while it
// works. The one open is raised.
function SubagentRow({ parent, subagent, open, stop }: { parent: string; subagent: Subagent; open: boolean; stop(job: string): void }) {
  return (
    <li className={`flex flex-col border-t border-line ${open ? "bg-surface-raised" : ""}`}>
      <a
        href={addressOf({ session: parent, subagent: subagent.session })}
        aria-current={open ? "page" : undefined}
        title={subagent.name}
        className="flex min-h-control min-w-0 items-center px-3 pt-1 text-body font-medium text-text"
      >
        <span className="min-w-0 truncate">{subagent.name}</span>
      </a>
      <Standing status={subagent.status} stop={subagent.status === "running" && subagent.job !== undefined ? () => stop(subagent.job!) : undefined} label={`Stop ${subagent.name}`} />
    </li>
  )
}

// The second line of a row: how it stands, and Stop while it works.
function Standing({ status, stop, label }: { status: Job["status"]; stop: (() => void) | undefined; label: string }) {
  return (
    <div className="flex min-h-control items-center justify-between gap-2 pb-1 pl-3">
      <StateWord status={status} />
      {stop && (
        <Button variant="quiet" aria-label={label} onClick={stop}>
          Stop
        </Button>
      )}
    </div>
  )
}

// The session's background jobs: those running, then those that have
// ended, folded away under their count. Each opens to its output. They are
// one list, so that a job open as it ends stays open, with its output, in
// the ended group, which opens for it.
function Jobs({ connection, session, running, ended, stop }: { connection: Connection; session: string; running: Job[]; ended: Job[]; stop(id: string): void }) {
  const [open, setOpen] = useState<ReadonlySet<string>>(new Set())
  const [showEnded, setShowEnded] = useState(false)
  const openEnded = ended.some((job) => open.has(job.id))
  const endedIds = ended.map((job) => job.id).join(" ")
  useEffect(() => {
    if (openEnded) setShowEnded(true)
  }, [endedIds])
  const toggle = (id: string) =>
    setOpen((was) => {
      const now = new Set(was)
      if (!now.delete(id)) now.add(id)
      return now
    })
  const row = (job: Job) => <JobRow key={job.id} connection={connection} session={session} job={job} open={open.has(job.id)} toggle={() => toggle(job.id)} stop={() => stop(job.id)} />
  const fold =
    ended.length > 0 ? (
      <li key="ended" className="border-t border-line">
        <button type="button" aria-expanded={showEnded} onClick={() => setShowEnded(!showEnded)} className="flex h-row-height w-full items-center gap-2 px-3 text-body text-text-muted hover:text-text">
          {ended.length} ended
          <Caret open={showEnded} />
        </button>
      </li>
    ) : null
  return <ul className="flex flex-col">{[...running.map(row), fold, ...(showEnded ? ended.map(row) : [])]}</ul>
}

// A job, as a row: its command, on a line of its own, which opens to its
// output; under it, how it stands, with Stop while it runs.
function JobRow({ connection, session, job, open, toggle, stop }: { connection: Connection; session: string; job: Job; open: boolean; toggle(): void; stop(): void }) {
  return (
    <li className="flex flex-col border-t border-line">
      <button type="button" aria-expanded={open} onClick={toggle} title={job.title} className="flex min-h-control w-full min-w-0 items-center gap-2 px-3 pt-1 text-left">
        <Caret open={open} />
        <span className="min-w-0 flex-1 truncate font-mono text-code text-text">{job.title}</span>
      </button>
      <Standing status={job.status} stop={job.status === "running" ? stop : undefined} label={`Stop job ${job.id}`} />
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
