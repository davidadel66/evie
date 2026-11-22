import * as PopoverPrimitive from "@radix-ui/react-popover"
import { useEffect, useId, useLayoutEffect, useRef, useState, useSyncExternalStore, type ComponentProps, type ReactNode } from "react"
import type { ChangedFile, GitStatus, Job, ListedProject, PullRequest } from "../../src/core/types.ts"
import { addressOf } from "./address.ts"
import { followGitStatus, followOutput, followPullRequest, followSubagents, openFolder, pullRequestOf, stopJob, undo, type FollowedGitStatus, type Following, type Lineage, type PullRequestState } from "./card.ts"
import { closeFloating, measured, pinCard, resetItems, showItem, toggleCard, useCardShown } from "./card-shown.ts"
import { holdChanges, type Changes as SessionChanges } from "./changes.ts"
import { Button } from "./components/ui/button.tsx"
import {
  DropdownMenu,
  DropdownMenuCheckboxItem,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "./components/ui/dropdown-menu.tsx"
import { Popover, PopoverContent, PopoverTrigger } from "./components/ui/popover.tsx"
import { Sheet, SheetContent, SheetTitle } from "./components/ui/sheet.tsx"
import type { Connection } from "./connection.ts"
import { holdConversation, startedHere, type Conversation } from "./conversation.ts"
import { openDiff } from "./diff-shown.ts"
import { FolderPath } from "./FolderPath.tsx"
import {
  ArrowBendLeftUp,
  Plus,
  ArrowCounterClockwise,
  ArrowSquareOut,
  Article,
  CaretDown,
  Check,
  CheckCircle,
  CircleDashed,
  CloudArrowUp,
  Folder,
  GitBranch,
  GitCommit,
  GitDiff,
  GitPullRequest,
  PushPin,
  PushPinFill,
  Robot,
  SlidersHorizontal,
  StopFill,
  TerminalWindow,
  X,
  XCircle,
} from "./icons.tsx"
import { cn } from "./lib/utils.ts"
import {
  backgroundJobs,
  cardDensity,
  DETAILS_ITEMS,
  gitAction,
  gitMenu,
  gitWhyNot,
  isDefaultShown,
  LINEAGE_FIRST,
  LINEAGE_PAGE,
  lineageOf,
  lineageWindow,
  parentWord,
  REQUESTS,
  sectionsShown,
  stateWord,
  subagentsOf,
  switchRequest,
  undoWhyNot,
  type CardFacts,
  type Density,
  type Request,
  type Shown,
} from "./project-card.ts"
import { ProjectTile } from "./ProjectIcon.tsx"
import { onThisComputer } from "./server-settings.ts"
import { checkLook, checksSummary, checkWord, fixRequest, pullRequestWord } from "./pull-request.ts"
import { select, useSelected } from "./selection.ts"
import { titleOf } from "./session-list.ts"
import type { Sessions } from "./sessions.ts"
import { label as keyLabel, onMac } from "./shortcuts.ts"
import { Spinner } from "./Spinner.tsx"
import { addressOfWorkspace } from "./workspace.ts"

// The session details, as T3 Code's details card, drawn as the owner's
// drawing of it: a card that floats at the top right of the conversation,
// pinned beside it or opened over it from its header (see card-shown.ts),
// with a slim row naming it, which holds Choose what to show and Pin, and
// its sections: the workspace, with the project, its folder and Undo last
// change; version control, with the branch, its pull request and checks,
// the git button and the changes against the base; the lineage, with the
// parent and the subagents; and background work. At phone width it is a
// sheet from the right.

// The card's place in the conversation's column, which it measures to know
// whether a pinned card has the room to sit there. It is given its place
// by the page: in place, in the column's top right corner; floating, from
// the same corner over the conversation; at phone width, as a sheet. A
// card floating closes when another session is picked.
export function CardPlace({ connection, sessions }: { connection: Connection; sessions: Sessions }) {
  const shown = useCardShown()
  const selected = useSelected()
  const corner = useRef<HTMLDivElement>(null)
  // A click outside closes it, as does Escape, but a click on the header's
  // button, which toggles it; the focus goes back to that button, unless
  // the click outside took it elsewhere.
  const pointedAway = useRef(false)
  useEffect(() => closeFloating(), [selected])
  useLayoutEffect(() => {
    const column = corner.current?.parentElement
    if (!column) return
    const observer = new ResizeObserver(() => measured(column.clientWidth, column.clientHeight))
    observer.observe(column)
    measured(column.clientWidth, column.clientHeight)
    return () => observer.disconnect()
  }, [])
  const card = <DetailsCard connection={connection} sessions={sessions} />
  return (
    <>
      <PopoverPrimitive.Root open={shown.place === "popover" && shown.open} onOpenChange={(open) => !open && closeFloating()}>
        <PopoverPrimitive.Anchor asChild>
          <div ref={corner} aria-hidden="true" className="pointer-events-none absolute top-details-top right-3 size-0" />
        </PopoverPrimitive.Anchor>
        <PopoverPrimitive.Portal>
          <PopoverPrimitive.Content
            aria-label="Session details"
            side="bottom"
            align="end"
            sideOffset={0}
            collisionPadding={12}
            onOpenAutoFocus={() => (pointedAway.current = false)}
            onPointerDownOutside={() => (pointedAway.current = true)}
            onInteractOutside={(event) => event.target instanceof Element && event.target.closest("[data-card-button]") !== null && event.preventDefault()}
            onCloseAutoFocus={(event) => {
              event.preventDefault()
              if (!pointedAway.current) document.querySelector<HTMLElement>("[data-card-button]")?.focus()
            }}
            className="z-40 flex max-h-(--radix-popover-content-available-height) w-details-card max-w-[calc(100vw-var(--spacing-6))] flex-col overflow-y-auto rounded-lg border border-text/10 text-detail text-text shadow-float details-glass"
          >
            {card}
          </PopoverPrimitive.Content>
        </PopoverPrimitive.Portal>
      </PopoverPrimitive.Root>
      {(shown.place === "inline" || shown.place === "over") && shown.open && (
        // Pinned without the room beside the conversation, it stays open over it, with the floating shadow, as drawn.
        <aside
          aria-label="Session details"
          className={cn(
            "absolute top-details-top right-3 z-20 flex max-h-details-height w-details-card max-w-[calc(100%-var(--spacing-6))] flex-col overflow-y-auto rounded-lg border border-text/10 text-detail text-text details-glass",
            shown.place === "over" && "shadow-float",
          )}
        >
          {card}
        </aside>
      )}
      <Sheet open={shown.place === "sheet" && shown.open} onOpenChange={(open) => !open && closeFloating()}>
        <SheetContent aria-label="Session details" className="overflow-y-auto text-detail">
          <SheetTitle className="sr-only">Session details</SheetTitle>
          {card}
        </SheetContent>
      </Sheet>
    </>
  )
}

// The button in the conversation's header that opens and closes the card,
// raised while it is open.
export function CardButton() {
  const shown = useCardShown()
  return (
    <Button
      variant="icon"
      data-card-button=""
      aria-label="Show or hide session details"
      aria-pressed={shown.open}
      title={`Session details (${keyLabel("card", onMac())})`}
      onClick={toggleCard}
      className={cn("size-control-sm", shown.open && "bg-surface-raised text-text")}
    >
      <Article />
    </Button>
  )
}

// What is under the conversation's header, kept clear of the card while it
// is pinned open beside it.
export function CardLane({ children }: { children: ReactNode }) {
  const shown = useCardShown()
  return <div className={cn("flex min-h-0 flex-1 flex-col", shown.place === "inline" && shown.open && "pr-details-clear")}>{children}</div>
}

// What the card holds for what is selected: a session, a draft of one, or
// nothing. Taller than its room, it drops to a compact density, and below
// that an essential one, as T3 Code's does, by its height measured at
// each; lists that scroll as they grow, the lineage and background work,
// are not counted, so that asking to see more of one hides nothing.
function DetailsCard({ connection, sessions }: { connection: Connection; sessions: Sessions }) {
  const list = useSyncExternalStore(sessions.subscribe, sessions.state)
  const selected = useSelected()
  const shown = useCardShown()
  const [menu, setMenu] = useState(false)
  const content = useRef<HTMLDivElement>(null)
  const key = `${selected === undefined ? "" : "draft" in selected ? `draft ${selected.draft}` : selected.session} ${DETAILS_ITEMS.filter((item) => shown.shown[item.id]).map((item) => item.id)}`
  const [heights, setHeights] = useState({ key, full: 0, compact: 0 })
  const kept = heights.key === key ? heights : { key, full: 0, compact: 0 }
  const density: Density = shown.place === "sheet" ? "full" : cardDensity(shown.room, kept)
  useLayoutEffect(() => {
    const element = content.current
    if (element === null || density === "essential") return
    const measure = () => {
      const frame = element.parentElement
      const lists = [...element.querySelectorAll<HTMLElement>("[data-details-list]")].reduce((sum, each) => sum + each.offsetHeight, 0)
      const height = element.offsetHeight + (frame ? frame.offsetHeight - frame.clientHeight : 0) - lists
      setHeights((now) => {
        const base = now.key === key ? now : { key, full: 0, compact: 0 }
        return now.key === key && base[density] === height ? now : { ...base, [density]: height }
      })
    }
    measure()
    const observer = new ResizeObserver(measure)
    observer.observe(element)
    return () => observer.disconnect()
  }, [density, key])
  const frame = (children: ReactNode) => (
    <div ref={content} className="flex flex-col">
      <Heading menu={menu} setMenu={setMenu} />
      {children}
    </div>
  )
  const projectOf = (id: string | null) => (id === null ? null : list.projects.find((each) => each.id === id))
  if (selected === undefined) return frame(<Quiet>No session is open.</Quiet>)
  if ("draft" in selected) {
    return frame(
      <>
        <Section label="Workspace" first>
          <ProjectRows connection={connection} projectId={selected.draft} project={projectOf(selected.draft)} density={density} />
        </Section>
        <Quiet>The rest is shown once the session's first message is sent.</Quiet>
      </>,
    )
  }
  if (!list.listed) return frame(null)
  // A session this page has just started is shown before the list has it.
  const summary = list.sessions.find((each) => each.session === selected.session)
  const started = startedHere(selected.session)
  if (summary === undefined && started === undefined) return frame(<Quiet>This session is not in the list.</Quiet>)
  const projectId = summary !== undefined ? summary.project : started!
  const parent = { title: summary === undefined ? "New session" : titleOf(summary), state: summary?.state }
  return frame(
    <SessionDetails
      key={selected.session}
      connection={connection}
      session={selected.session}
      opened={selected.subagent}
      parent={parent}
      projectId={projectId}
      project={projectOf(projectId)}
      density={density}
      chooseWhatToShow={() => setMenu(true)}
    />,
  )
}

// A line in the card saying there is nothing more to show, or why.
function Quiet({ children }: { children: ReactNode }) {
  return <p className="px-4.5 pb-3 text-caption text-text-muted">{children}</p>
}

// The slim row at the card's top: its name, Choose what to show, and Pin,
// or at phone width, Close.
function Heading({ menu, setMenu }: { menu: boolean; setMenu(open: boolean): void }) {
  const shown = useCardShown()
  const pin = shown.pinned ? "Unpin: open it from the header instead" : "Pin beside the conversation"
  return (
    // A 32px row under 6px of room, as drawn.
    <div className="flex min-h-row-height items-center gap-0.5 pt-1.5 pr-3 pl-2 box-content">
      <h2 className="min-w-0 flex-1 pl-2.5 text-detail-heading text-text-muted">Session details</h2>
      <DropdownMenu open={menu} onOpenChange={setMenu}>
        <DropdownMenuTrigger asChild>
          <IconAction label="Choose what to show" pressed={menu}>
            <SlidersHorizontal />
          </IconAction>
        </DropdownMenuTrigger>
        <ItemsMenu shown={shown.shown} />
      </DropdownMenu>
      {shown.place === "sheet" ? (
        <IconAction label="Close session details" onClick={closeFloating}>
          <X />
        </IconAction>
      ) : (
        <IconAction label={pin} pressed={shown.pinned} aria-pressed={shown.pinned} onClick={() => pinCard(!shown.pinned)}>
          {shown.pinned ? <PushPinFill /> : <PushPin />}
        </IconAction>
      )}
    </div>
  )
}

// A 24px button holding an icon alone, as T3 Code's card's, named by its label.
function IconAction({ label, pressed = false, className, children, ...props }: { label: string; pressed?: boolean } & ComponentProps<"button">) {
  return (
    <button
      type="button"
      aria-label={label}
      title={label}
      className={cn(
        "flex size-6 shrink-0 items-center justify-center rounded-chip text-text-muted hover:bg-text/8 [&>svg]:size-icon-caret",
        pressed && "bg-text/8 text-text",
        className,
      )}
      {...props}
    >
      {children}
    </button>
  )
}

// Choose what to show: each item, ticked while it shows, which stays open
// to choose another, and Reset to default, off while all are as at first.
function ItemsMenu({ shown }: { shown: Shown }) {
  return (
    <DropdownMenuContent align="end" className="w-details-menu text-detail">
      <DropdownMenuLabel>Show in session details</DropdownMenuLabel>
      {DETAILS_ITEMS.map((item) => (
        <DropdownMenuCheckboxItem key={item.id} checked={shown[item.id]} onCheckedChange={(on) => showItem(item.id, on)} onSelect={(event) => event.preventDefault()} className="text-detail">
          {item.label}
        </DropdownMenuCheckboxItem>
      ))}
      <DropdownMenuSeparator />
      <DropdownMenuItem disabled={isDefaultShown(shown)} onSelect={(event) => (event.preventDefault(), resetItems())} className="pl-8 text-detail data-disabled:text-text-muted">
        Reset to default
      </DropdownMenuItem>
    </DropdownMenuContent>
  )
}

// A section of the card: the first has no line above it, nor one not
// `separated`, as version control at a smaller density, as T3 Code's; and
// one named on the card has its heading.
function Section({ label, first = false, separated = true, heading, children }: { label: string; first?: boolean; separated?: boolean; heading?: string; children: ReactNode }) {
  const id = useId()
  return (
    <section
      {...(heading !== undefined ? { "aria-labelledby": id } : { "aria-label": label })}
      className={cn("flex flex-col px-2 pb-2.5", first ? "pt-0.5" : "pt-2", !first && separated && "border-t border-line")}
    >
      {heading !== undefined && (
        <div className="mb-1 flex min-h-row-height items-center px-2.5">
          <h3 id={id} className="text-detail-heading text-text-muted">
            {heading}
          </h3>
        </div>
      )}
      {children}
    </section>
  )
}

// A row's look, as T3 Code's: 32px, its icon and words 10px in, raised a
// little on hover; and its icon's.
const ROW = "flex h-row-height w-full min-w-0 items-center gap-2.5 rounded-md px-2.5 text-left text-detail font-medium text-text/85 hover:bg-text/8"
const ROW_ICON = "size-icon-sm shrink-0 text-text-muted"

// The project, which opens its workspace, and its folder, which opens on
// the computer evie runs on, as T3 Code's "Open in"; or that the session
// has none. `density` drops the project, compact, and the folder too,
// essential, as T3 Code's card drops its environment and its Open in.
function ProjectRows({ connection, projectId, project, density }: { connection: Connection; projectId: string | null; project: ListedProject | null | undefined; density: Density }) {
  const [failed, setFailed] = useState<string>()
  const showProject = density === "full"
  const showFolder = density !== "essential"
  if (projectId === null) {
    return (
      <>
        {showProject && (
          <div className={cn(ROW, "hover:bg-transparent")}>
            <span aria-hidden="true" className="size-icon-sm shrink-0 rounded-sm border border-dashed border-text-muted" />
            <span className="min-w-0 truncate">No project</span>
          </div>
        )}
        {showFolder && (
          <div className={cn(ROW, "font-normal hover:bg-transparent")}>
            <Folder className={ROW_ICON} />
            <span className="min-w-0 truncate font-mono text-caption">No folder</span>
          </div>
        )}
      </>
    )
  }
  const name = project?.name ?? "Project"
  return (
    <>
      {showProject && (
        <a href={addressOfWorkspace({ workspace: projectId, tab: "sessions" })} title={`Open the ${name} workspace`} className={ROW}>
          <ProjectTile project={projectId} size="detail" />
          <span className="min-w-0 truncate">{name}</span>
        </a>
      )}
      {showFolder && project != null && (
        <>
          <Act
            why={HERE ? undefined : "The folder can be opened only on the computer evie runs on."}
            onPress={() => void openFolder(connection, projectId).then(setFailed)}
            title="Open the folder"
            className={cn(ROW, "font-normal")}
          >
            <Folder className={ROW_ICON} />
            <FolderPath folder={project.folder} shown={project.fromHome ?? project.folder} className="text-caption" />
          </Act>
          <Refused why={failed} />
        </>
      )}
    </>
  )
}

// Whether the page is on the computer evie runs on, where a project's
// folder can be opened, as the settings window asks.
const HERE = onThisComputer(location.origin)

// Why what a row asked for could not be done, under it, until the next press.
function Refused({ why }: { why: string | undefined }) {
  return why === undefined ? null : (
    <p role="status" className="px-2.5 pb-1 pl-detail-indent text-caption text-text-muted">
      {why}
    </p>
  )
}

// How long a git button's request keeps the buttons off when no turn
// starts for it, in milliseconds.
const REQUEST_WAIT = 5000

// The card of a session: everything it shows, from the session as it
// stands, which it follows with the middle column, and as it changes.
function SessionDetails({
  connection,
  session,
  opened,
  parent,
  projectId,
  project,
  density,
  chooseWhatToShow,
}: {
  connection: Connection
  session: string
  // The subagent whose conversation is open in the middle, if one is.
  opened: string | undefined
  parent: { title: string; state: string | undefined }
  projectId: string | null
  project: ListedProject | null | undefined
  density: Density
  chooseWhatToShow(): void
}) {
  const [followed, setFollowed] = useState<{ conversation: Conversation; lineage: Lineage; changes: SessionChanges }>()
  useEffect(() => {
    const held = holdConversation(connection, session)
    const lineage = followSubagents(connection, session)
    // With the whole session's files, which a folder not in git sums up.
    const changes = holdChanges(connection, session, { whole: true })
    setFollowed({ conversation: held.conversation, lineage, changes: changes.changes })
    return () => {
      changes.release()
      held.release()
      lineage.close()
    }
  }, [connection, session])
  const repository = projectId !== null && project?.git !== false ? projectId : undefined
  const pull = usePullRequest(connection, repository, project?.branch)
  const git = useGitStatus(connection, repository)
  if (followed === undefined) {
    return (
      <Section label="Workspace" first>
        <ProjectRows connection={connection} projectId={projectId} project={project} density={density} />
      </Section>
    )
  }
  return <Live connection={connection} {...followed} session={session} opened={opened} parent={parent} projectId={projectId} project={project} pull={pull} git={git} density={density} chooseWhatToShow={chooseWhatToShow} />
}

// The branch's pull request, read when the card opens and each minute
// while it is shown; undefined until read, null when there is none, and
// unavailable where pull requests cannot be read.
function usePullRequest(connection: Connection, project: string | undefined, branch: string | undefined): PullRequestState {
  const [followed, setFollowed] = useState<ReturnType<typeof followPullRequest>>()
  useEffect(() => {
    if (project === undefined) return setFollowed(undefined)
    const following = followPullRequest(connection, project, { window, shown: () => document.visibilityState === "visible" })
    setFollowed(following)
    return following.close
  }, [connection, project, branch])
  return useSyncExternalStore(followed?.subscribe ?? nothing.subscribe, followed?.state ?? nothing.state)
}

// How the branch stands, as followGitStatus reads it, with what follows it,
// to read it again when a turn ends.
function useGitStatus(connection: Connection, project: string | undefined) {
  const [followed, setFollowed] = useState<FollowedGitStatus>()
  useEffect(() => {
    if (project === undefined) return setFollowed(undefined)
    const following = followGitStatus(connection, project, { window, shown: () => document.visibilityState === "visible", base: () => pullRequestOf(project)?.base ?? undefined })
    setFollowed(following)
    return following.close
  }, [connection, project])
  const status = useSyncExternalStore(followed?.subscribe ?? nothing.subscribe, followed?.state ?? nothing.state) as GitStatus | null | undefined
  return { status, again: () => followed?.again() }
}
const nothing = { subscribe: () => () => {}, state: () => undefined }

function Live({
  connection,
  conversation,
  lineage,
  changes,
  session,
  opened,
  parent,
  projectId,
  project,
  pull,
  git,
  density,
  chooseWhatToShow,
}: {
  connection: Connection
  conversation: Conversation
  lineage: Lineage
  changes: SessionChanges
  session: string
  opened: string | undefined
  parent: { title: string; state: string | undefined }
  projectId: string | null
  project: ListedProject | null | undefined
  pull: PullRequestState
  git: { status: GitStatus | null | undefined; again(): void }
  density: Density
  chooseWhatToShow(): void
}) {
  const shown = useCardShown().shown
  const state = useSyncExternalStore(conversation.subscribe, conversation.state)
  const { listed: stored } = useSyncExternalStore(lineage.subscribe, lineage.state)
  const { listed } = useSyncExternalStore(changes.subscribe, changes.state)
  const connected = useSyncExternalStore(connection.subscribe, connection.state).status === "connected"
  const pullRequest = pull != null && "number" in pull ? pull : undefined

  // A request sent that evie has not started on: one at a time.
  const [requested, setRequested] = useState(false)
  useEffect(() => {
    if (state.turnRunning) setRequested(false)
  }, [state.turnRunning])
  useEffect(() => {
    if (!requested) return
    const timer = setTimeout(() => setRequested(false), REQUEST_WAIT)
    return () => clearTimeout(timer)
  }, [requested])
  // How the branch stands is read again once a turn has ended since the card
  // opened, and once the pull request's base is read or changes, which the
  // changes are counted against; it was read once already as it opened.
  const ended = state.events.findLast((event) => event.type === "turn_end")?.seq
  const readAt = useRef({ ended, base: pullRequest?.base })
  useEffect(() => {
    if (readAt.current.ended === ended && readAt.current.base === pullRequest?.base) return
    readAt.current = { ended, base: pullRequest?.base }
    git.again()
  }, [ended, pullRequest?.base])

  // A request on its way: a second press waits for it, so it is sent once.
  const sendingNow = useRef(false)
  const [sending, setSending] = useState(false)
  const facts: CardFacts = {
    connected,
    session: state.opened ? "open" : state.problem !== undefined ? "failed" : "opening",
    project,
    turnRunning: state.turnRunning,
    requested,
    sending,
  }
  // With a subagent's conversation open in the middle, a press goes back to
  // the session's own first, so that what it says is in view. Gives back
  // why the request could not be sent, or nothing.
  const back = () => opened !== undefined && select({ session })
  async function ask(text: string): Promise<string | undefined> {
    if (sendingNow.current) return undefined
    sendingNow.current = true
    setSending(true)
    back()
    try {
      const failed = await conversation.send(text)
      setRequested(failed === undefined)
      return failed
    } finally {
      sendingNow.current = false
      setSending(false)
    }
  }

  const subagents = subagentsOf(stored, state.jobs)
  const jobs = backgroundJobs(state.jobs)
  const latest = listed?.kept ? listed.turns.at(-1) : undefined
  const sessionFiles = listed?.kept ? listed.session : undefined
  const has = {
    undo: latest !== undefined,
    git: git.status !== null && projectId !== null && project?.git !== false,
    sessionChanges: sessionFiles !== undefined,
    related: opened !== undefined || subagents.length > 0,
    jobs: jobs.running.length + jobs.ended.length > 0,
  }
  const sections = sectionsShown(shown, has, density)
  // Why the last Stop could not be asked for, until the next.
  const [stopRefused, setStopRefused] = useState<string>()
  const stop = (id: string) => void stopJob(connection, session, id).then(setStopRefused)
  const why = gitWhyNot(facts)

  return (
    <>
      {sections.workspace && (
        <Section label="Workspace" first>
          {(sections.project || sections.folder) && <ProjectRows connection={connection} projectId={projectId} project={project} density={density} />}
          {sections.undo && latest !== undefined && (
            <UndoRow
              files={latest.files.length}
              why={undoWhyNot(facts)}
              undo={async () => {
                back()
                await undo(conversation)
              }}
            />
          )}
          {sections.sessionChanges && sessionFiles !== undefined && <SessionChangesRow files={sessionFiles} back={back} />}
        </Section>
      )}
      {sections.versionControl && (
        <Section label="Version control" first={!sections.workspace} separated={density === "full"}>
          {sections.branch && <BranchRow status={git.status} branch={git.status?.branch ?? project?.branch} why={why} ask={ask} />}
          {sections.branch && pullRequest !== undefined && <PullRow pull={pullRequest} why={why} ask={ask} />}
          {sections.gitButton && <GitButton name={project?.name ?? "the project"} status={git.status} pull={pull} why={why} ask={ask} />}
          {sections.changes && <ChangesRow status={git.status} back={back} />}
        </Section>
      )}
      {sections.lineage && (
        <LineageSection
          first={!sections.workspace && !sections.versionControl}
          session={session}
          parent={{ title: parent.title, word: parentWord({ needsYou: parent.state === "needs_you", turnRunning: state.turnRunning, subagentsWorking: subagents.some((each) => each.status === "running") }) }}
          opened={opened}
          subagents={subagents}
          stop={stop}
          refused={stopRefused}
        />
      )}
      {sections.jobs && (
        <JobsSection
          first={!sections.workspace && !sections.versionControl && !sections.lineage}
          connection={connection}
          session={session}
          running={jobs.running}
          ended={jobs.ended}
          stop={stop}
          refused={sections.lineage ? undefined : stopRefused}
        />
      )}
      {sections.nothing && (
        <div className="px-2 pt-1 pb-2">
          <button type="button" onClick={chooseWhatToShow} className={ROW}>
            <SlidersHorizontal className={ROW_ICON} />
            Choose what to show
          </button>
        </div>
      )}
    </>
  )
}

// A control that does something, or while it does not apply, says why
// when pressed or pointed at, as T3 Code's git button does, rather than
// being off and silent. It stays in the order of the keyboard. `title` is
// what it says when pointed at while it applies.
function Act({ why, onPress, className, label, title, children }: { why: string | undefined; onPress(): void; className: string; label?: string; title?: string; children: ReactNode }) {
  const [open, setOpen] = useState(false)
  const note = useId()
  useEffect(() => {
    if (why === undefined) setOpen(false)
  }, [why])
  return (
    <Popover open={open && why !== undefined} onOpenChange={setOpen}>
      <PopoverTrigger asChild>
        <button
          type="button"
          aria-label={label}
          aria-disabled={why !== undefined}
          aria-describedby={why === undefined ? undefined : note}
          title={why ?? title ?? label}
          onClick={(event) => {
            if (why !== undefined) return
            event.preventDefault()
            onPress()
          }}
          className={cn(className, why !== undefined && "text-text-muted")}
        >
          {children}
        </button>
      </PopoverTrigger>
      {why !== undefined && (
        <span id={note} hidden>
          {why}
        </span>
      )}
      <PopoverContent side="bottom" className="max-w-details-card text-caption text-text-muted">
        {why}
      </PopoverContent>
    </Popover>
  )
}

function UndoRow({ files, why, undo }: { files: number; why: string | undefined; undo(): Promise<void> }) {
  const [undoing, setUndoing] = useState(false)
  return (
    <Act
      why={why ?? (undoing ? "Undoing…" : undefined)}
      onPress={() => {
        setUndoing(true)
        void undo().finally(() => setUndoing(false))
      }}
      title="Put back the files as they were before the last turn"
      className={ROW}
    >
      <ArrowCounterClockwise className={ROW_ICON} />
      <span className="flex-1">Undo last change</span>
      <span className="text-caption font-normal text-text-muted">{files === 1 ? "1 file" : `${files} files`}</span>
    </Act>
  )
}

// In a folder not in git, what the session's turns have changed, from
// undo's copies, as decision 93's diffs come from them there: the lines
// added and removed; it opens the diff on the whole session.
function SessionChangesRow({ files, back }: { files: readonly ChangedFile[]; back(): void }) {
  const added = files.reduce((sum, file) => sum + file.added, 0)
  const removed = files.reduce((sum, file) => sum + file.removed, 0)
  return (
    <button
      type="button"
      title="Open the changes this session made"
      onClick={() => {
        back()
        openDiff({ scope: "session" })
      }}
      className={ROW}
    >
      <GitDiff className={ROW_ICON} />
      <span className="flex min-w-0 flex-1 items-baseline gap-1.5">
        <span className="shrink-0">Changes</span>
        <span className="min-w-0 truncate font-normal text-text-muted">in this session</span>
        <Counted added={added} removed={removed} />
      </span>
    </button>
  )
}

// Lines added and removed, as T3 Code's rows count them.
function Counted({ added, removed }: { added: number; removed: number }) {
  return (
    <span className="ml-auto flex shrink-0 gap-2 pl-2 font-mono text-caption font-normal tabular-nums">
      <span className={added > 0 ? "text-diff-added" : "text-text-muted"}>+{added}</span>
      <span className={removed > 0 ? "text-diff-removed" : "text-text-muted"}>−{removed}</span>
    </span>
  )
}

// Two buttons in one row, as T3 Code's split controls: the main one, and a
// caret that opens a menu, parted by a line only while the row is pointed at.
const SPLIT = "group/split flex h-row-height w-full items-center rounded-md hover:bg-text/8 has-[[data-state=open]]:bg-text/8"
const SPLIT_MAIN = "flex h-row-height min-w-0 flex-1 items-center gap-2.5 rounded-l-md px-2.5 text-left text-detail font-medium text-text/85 hover:bg-text/8"
const SPLIT_CARET = "flex size-8 shrink-0 items-center justify-center rounded-r-md text-text-muted hover:bg-text/8 [&>svg]:size-icon-sm"
const SPLIT_LINE = "h-4 w-(--line-width) shrink-0 bg-transparent group-hover/split:bg-line group-has-[[data-state=open]]/split:bg-line"

// The branch, and the caret that asks evie to switch to another of the
// repository's branches, in words, as the git buttons ask.
function BranchRow({ status, branch, why, ask }: { status: GitStatus | null | undefined; branch: string | undefined; why: string | undefined; ask(text: string): Promise<string | undefined> }) {
  const [open, setOpen] = useState(false)
  // Why the last switch asked for could not be sent, until the next.
  const [failed, setFailed] = useState<string>()
  const others = (status?.branches ?? []).filter((each) => each !== branch)
  const named = branch ?? "No branch"
  return (
    <>
    <DropdownMenu open={open} onOpenChange={setOpen}>
      <div className={SPLIT}>
        <button type="button" aria-label={`Branch: ${named}`} title={named} onClick={() => setOpen(true)} className={SPLIT_MAIN}>
          <GitBranch className={ROW_ICON} />
          <span className="min-w-0 truncate">{named}</span>
        </button>
        <span aria-hidden="true" className={SPLIT_LINE} />
        <DropdownMenuTrigger asChild>
          <button type="button" aria-label="Change branch" title="Change branch" className={SPLIT_CARET}>
            <CaretDown />
          </button>
        </DropdownMenuTrigger>
      </div>
      <DropdownMenuContent align="end" className="w-details-card max-w-menu-wide text-detail">
        <DropdownMenuLabel>{others.length > 0 ? "Ask evie to switch to" : "No other branch to switch to"}</DropdownMenuLabel>
        {others.map((each) => (
          <DropdownMenuItem key={each} disabled={why !== undefined} title={why} onSelect={() => void ask(switchRequest(each)).then(setFailed)} className="text-detail">
            <GitBranch className="text-text-muted" />
            <span className="min-w-0 truncate">{each}</span>
          </DropdownMenuItem>
        ))}
      </DropdownMenuContent>
    </DropdownMenu>
    <Refused why={failed} />
    </>
  )
}

// The branch's pull request, as T3 Code's row: its title, on up to two
// lines, which opens it on GitHub; under it its number, its checks summed
// up, which open a menu of each, and Fix while any fails, which asks evie
// to fix them in this session, in words, as the git buttons ask.
function PullRow({ pull, why, ask }: { pull: PullRequest; why: string | undefined; ask(text: string): Promise<string | undefined> }) {
  const fix = fixRequest(pull)
  // Why the last Fix could not be sent, until the next.
  const [failed, setFailed] = useState<string>()
  const state = pullRequestWord(pull.state).toLowerCase()
  const title = (
    <>
      <GitPullRequest className={cn("mt-px size-icon-sm shrink-0", PULL_COLOUR[pull.state])} />
      <span className="line-clamp-2 min-w-0 flex-1 break-words">{pull.title}</span>
    </>
  )
  const look = "flex min-h-row-height w-full items-start gap-2.5 rounded-md px-2.5 pt-1.5 text-left text-detail/4.5 font-medium text-text/85 hover:bg-text/8"
  return (
    <div className="pb-1.5">
      {/* The server gives only an https link, and none for any other. */}
      {pull.url !== undefined ? (
        <a href={pull.url} target="_blank" rel="noreferrer" aria-label={`Pull request #${pull.number}, ${state}: ${pull.title}`} title={`Open pull request #${pull.number} on GitHub`} className={look}>
          {title}
        </a>
      ) : (
        <p aria-label={`Pull request #${pull.number}, ${state}: ${pull.title}`} className={cn(look, "hover:bg-transparent")}>
          {title}
        </p>
      )}
      <div className="flex h-5 min-w-0 items-center gap-0.5 pr-2.5 pl-detail-indent text-caption text-text-muted">
        <span className="mr-1 shrink-0 tabular-nums">#{pull.number}</span>
        {pull.checks.length > 0 && <Checks checks={pull.checks} />}
        <span className="flex-1" />
        {fix !== undefined && (
          <Act
            why={why}
            onPress={() => void ask(fix).then(setFailed)}
            label="Fix the failing checks in this session"
            className="h-5 shrink-0 rounded-chip bg-status-failed/12 px-1.5 text-caption font-medium text-status-failed hover:bg-status-failed/16"
          >
            Fix
          </Act>
        )}
      </div>
      <Refused why={failed} />
    </div>
  )
}

// A pull request's icon in the colour of how it stands: open in Done's,
// merged in plum, as T3 Code's in violet, closed in Failed's, and a draft
// quiet.
const PULL_COLOUR: Record<PullRequest["state"], string> = {
  open: "text-status-done",
  merged: "text-project-plum",
  closed: "text-status-failed",
  draft: "text-text-muted",
}

function Checks({ checks }: { checks: PullRequest["checks"] }) {
  const summary = checksSummary(checks)
  const failing = checks.some((check) => check.state === "failed" || check.state === "cancelled")
  const pending = checks.some((check) => check.state === "running" || check.state === "waiting")
  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <button
          type="button"
          aria-label={`Open checks: ${summary}`}
          className="flex h-6 min-w-0 items-center gap-1 rounded-chip px-1.5 text-caption font-medium text-text-muted hover:bg-text/8 data-[state=open]:bg-text/8"
        >
          {failing ? (
            <XCircle className="size-icon-caret shrink-0 text-status-failed" />
          ) : pending ? (
            <CircleDashed className="size-icon-caret shrink-0 text-status-working" />
          ) : (
            <CheckCircle className="size-icon-caret shrink-0 text-status-done" />
          )}
          <span className="min-w-0 truncate tabular-nums">{summary}</span>
        </button>
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

// The git button: its label follows how the branch stands (gitAction), and
// a press sends evie its request in words; the caret opens Commit, Push
// and Create pull request, each sent the same way. While none applies, or
// evie is busy, it says why.
function GitButton({
  name,
  status,
  pull,
  why,
  ask,
}: {
  name: string
  status: GitStatus | null | undefined
  pull: PullRequestState
  why: string | undefined
  ask(text: string): Promise<string | undefined>
}) {
  const action = gitAction(status ?? undefined, pull)
  const menu = gitMenu(status ?? undefined, pull)
  // Why the last request could not be sent, said under the button until
  // the next, or until the branch stands otherwise.
  const [refused, setRefused] = useState<string>()
  useEffect(() => setRefused(undefined), [status])
  const send = async (kind: Request) => setRefused(await ask(REQUESTS[kind]))
  const Icon = action.label === "Commit" ? GitCommit : action.label === "Commit, push & PR" ? GitPullRequest : CloudArrowUp
  const cannotRead = pull != null && "unavailable" in pull ? pull.unavailable : undefined
  return (
    <>
      <div role="group" aria-label={`Git actions for ${name}`} className={SPLIT}>
        <Act why={why ?? action.why} onPress={() => void send(action.request)} className={SPLIT_MAIN} label={action.label}>
          <Icon className={ROW_ICON} />
          <span className="min-w-0 truncate">{action.label}</span>
        </Act>
        <span aria-hidden="true" className={SPLIT_LINE} />
        <DropdownMenu>
          <DropdownMenuTrigger asChild>
            <button type="button" aria-label="Git action options" title="Git action options" disabled={menu.length === 0} className={SPLIT_CARET}>
              <CaretDown />
            </button>
          </DropdownMenuTrigger>
          <DropdownMenuContent align="end" className="w-details-card max-w-menu-wide text-detail">
            {menu.map((item) => {
              const off = why ?? item.why
              const Glyph = item.request === "commit" ? GitCommit : item.request === "push" ? CloudArrowUp : GitPullRequest
              return (
                <DropdownMenuItem key={item.request} disabled={off !== undefined} title={off} onSelect={() => void send(item.request)} className="text-detail data-disabled:text-text-muted">
                  <Glyph className="text-text-muted" />
                  {item.label}
                </DropdownMenuItem>
              )
            })}
          </DropdownMenuContent>
        </DropdownMenu>
      </div>
      <Refused why={refused ?? (cannotRead === undefined ? undefined : `Pull requests cannot be read here, so only pushing is offered: ${cannotRead}`)} />
    </>
  )
}

// What the branch has changed, as T3 Code's row: against the branch it is
// compared with, or on the main branch the work not committed, with the
// lines added and removed; it opens the diff on the same.
function ChangesRow({ status, back }: { status: GitStatus | null | undefined; back(): void }) {
  const changes = status?.changes
  const against = changes?.against ?? null
  // A branch that cannot be compared says why, as the diff would.
  const words = changes === undefined ? "" : changes.unavailable !== undefined ? changes.unavailable : against !== null ? `vs ${against}` : "Uncommitted"
  return (
    <button
      type="button"
      title={changes?.unavailable ?? (against !== null ? `Open the changes against ${against}` : "Open the changes since the last commit")}
      onClick={() => {
        back()
        openDiff({ scope: against !== null ? "branch" : "uncommitted" })
      }}
      className={ROW}
    >
      <GitDiff className={ROW_ICON} />
      <span className="flex min-w-0 flex-1 items-baseline gap-1.5">
        <span className="shrink-0">Changes</span>
        <span className="min-w-0 truncate font-normal text-text-muted">{words}</span>
        {changes !== undefined && changes.unavailable === undefined && <Counted added={changes.added} removed={changes.removed} />}
      </span>
    </button>
  )
}

// The time now, again each second while `ticking`, for how long a
// subagent's round has run.
function useNow(ticking: boolean) {
  const [now, setNow] = useState(Date.now)
  useEffect(() => {
    if (!ticking) return
    setNow(Date.now())
    const timer = setInterval(() => setNow(Date.now()), 1000)
    return () => clearInterval(timer)
  }, [ticking])
  return now
}

// The lineage: the session that started this one, while one of its
// subagents' conversations is open, which leads back to it, with how it
// stands, "Waiting" while it waits on its subagents, as drawn; the
// subagents working, each with how long it has run and Stop; and those
// that have ended, folded away under Previous agents. Each opens its
// conversation in the middle, read only; the one open is raised. Each list
// shows six at first, then offers more, as T3 Code's, and is not counted
// when the card measures whether it fits, since it scrolls as it grows.
function LineageSection({
  first,
  session,
  parent,
  opened,
  subagents,
  stop,
  refused,
}: {
  first: boolean
  session: string
  parent: { title: string; word: string }
  opened: string | undefined
  subagents: ReturnType<typeof subagentsOf>
  stop(job: string): void
  refused: string | undefined
}) {
  const working = subagents.some((each) => each.status === "running")
  const now = useNow(working)
  const shown = lineageOf(subagents, now)
  const [folded, setFolded] = useState(true)
  const openEnded = shown.previous.some((each) => each.session === opened)
  useEffect(() => {
    if (openEnded) setFolded(false)
  }, [openEnded])
  const related = [...(opened !== undefined ? ["parent" as const] : []), ...shown.running]
  const [showing, setShowing] = useState(LINEAGE_FIRST)
  const [showingEarlier, setShowingEarlier] = useState(LINEAGE_FIRST)
  const rows = lineageWindow(related, showing)
  const earlier = lineageWindow(shown.previous, showingEarlier)
  return (
    <Section label="Lineage" heading={shown.heading} first={first}>
      {refused !== undefined && <p className="px-2.5 pb-1 text-caption text-text-muted">{refused}</p>}
      <div data-details-list="">
        <ul aria-label="Related sessions" className="flex flex-col">
          {rows.shown.map((each) =>
            each === "parent" ? (
              <li key="parent" className="flex h-row-height">
                <a href={addressOf({ session })} title="Open the parent session" className={ROW}>
                  <ArrowBendLeftUp className={ROW_ICON} />
                  <span className="min-w-0 flex-1 truncate text-body/4">{parent.title}</span>
                  <span className="shrink-0 text-detail-heading font-normal text-text-muted">{parent.word}</span>
                </a>
              </li>
            ) : (
              <SubagentRow key={each.session} parent={session} subagent={each} open={each.session === opened} stop={each.job !== undefined ? () => stop(each.job!) : undefined} />
            ),
          )}
        </ul>
        <ShowMore words={rows.more} more={() => setShowing(showing + LINEAGE_PAGE)} />
      </div>
      {shown.previous.length > 0 && (
        <>
          <button type="button" aria-expanded={!folded} onClick={() => setFolded(!folded)} className={ROW}>
            <Robot className={ROW_ICON} />
            <span className="flex-1">Previous agents</span>
            <span className="text-caption font-normal text-text-muted tabular-nums">{shown.previous.length}</span>
            <CaretDown className={cn(ROW_ICON, "transition-transform motion-reduce:transition-none", folded && "-rotate-90")} />
          </button>
          {!folded && (
            <div data-details-list="">
              <ul aria-label="Previous agents" className="flex flex-col">
                {earlier.shown.map((each) => (
                  <SubagentRow key={each.session} parent={session} subagent={each} open={each.session === opened} stop={undefined} />
                ))}
              </ul>
              <ShowMore words={earlier.more} more={() => setShowingEarlier(showingEarlier + LINEAGE_PAGE)} />
            </div>
          )}
        </>
      )}
    </Section>
  )
}

// "Show 12 more", as T3 Code's lineage offers the rest of a long list.
function ShowMore({ words, more }: { words: string | undefined; more(): void }) {
  if (words === undefined) return null
  return (
    <button type="button" onClick={more} className={cn(ROW, "text-text-muted hover:text-text")}>
      <Plus className={ROW_ICON} />
      {words}
    </button>
  )
}

// The dot on a subagent's robot: the accent while it works, Done's or
// Failed's once it has ended so, and quiet otherwise.
const DOT: Record<Job["status"], string> = { running: "bg-accent", finished: "bg-status-done", failed: "bg-status-failed", stopped: "bg-text-muted", unknown: "bg-text-muted" }

function SubagentRow({ parent, subagent, open, stop }: { parent: string; subagent: ReturnType<typeof lineageOf>["running"][number]; open: boolean; stop: (() => void) | undefined }) {
  return (
    <li className="relative flex h-row-height">
      <a
        href={addressOf({ session: parent, subagent: subagent.session })}
        aria-current={open ? "page" : undefined}
        title={`Open subagent: ${subagent.name}`}
        className={cn(ROW, stop !== undefined && "pr-8", open && "bg-text/8")}
      >
        <span className="relative flex size-icon-sm shrink-0">
          <Robot className={ROW_ICON} />
          <span aria-hidden="true" className={cn("absolute -right-0.5 -bottom-0.5 size-2 rounded-full border-(length:--indicator-width) border-surface", DOT[subagent.status])} />
        </span>
        <span className="min-w-0 flex-1 truncate text-body/4">{subagent.name}</span>
        <span className="shrink-0 text-detail-heading font-normal text-text-muted tabular-nums">{subagent.state}</span>
      </a>
      {stop !== undefined && <StopButton label={`Stop subagent ${subagent.name}`} title="Stop subagent" stop={stop} />}
    </li>
  )
}

function StopButton({ label, title, stop }: { label: string; title: string; stop(): void }) {
  return (
    <button type="button" aria-label={label} title={title} onClick={stop} className="absolute top-1 right-1 flex size-6 items-center justify-center rounded-chip text-status-failed hover:bg-text/8">
      <StopFill className="size-icon-xxs" />
    </button>
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

// Background work, shaped as T3 Code's servers: each job running, which
// opens to its output, with Stop; and those that have ended, folded away
// under Earlier background work, as Previous agents is, each opening to
// its output too. A job open as it ends stays open, and its fold opens for
// it. The list is not counted when the card measures whether it fits.
function JobsSection({
  first,
  connection,
  session,
  running,
  ended,
  stop,
  refused,
}: {
  first: boolean
  connection: Connection
  session: string
  running: Job[]
  ended: Job[]
  stop(id: string): void
  refused: string | undefined
}) {
  const [open, setOpen] = useState<ReadonlySet<string>>(new Set())
  const [folded, setFolded] = useState(true)
  const openEnded = ended.some((job) => open.has(job.id))
  const endedIds = ended.map((job) => job.id).join(" ")
  useEffect(() => {
    if (openEnded) setFolded(false)
  }, [endedIds])
  const toggle = (id: string) =>
    setOpen((was) => {
      const now = new Set(was)
      if (!now.delete(id)) now.add(id)
      return now
    })
  const row = (job: Job) => <JobRow key={job.id} connection={connection} session={session} job={job} open={open.has(job.id)} toggle={() => toggle(job.id)} stop={() => stop(job.id)} />
  return (
    <Section label="Background work" heading="Background work" first={first}>
      {refused !== undefined && <p className="px-2.5 pb-1 text-caption text-text-muted">{refused}</p>}
      {running.length > 0 && (
        <ul aria-label="Background work" data-details-list="" className="flex flex-col">
          {running.map(row)}
        </ul>
      )}
      {ended.length > 0 && (
        <>
          <button type="button" aria-expanded={!folded} onClick={() => setFolded(!folded)} className={ROW}>
            <TerminalWindow className={ROW_ICON} />
            <span className="flex-1">Earlier background work</span>
            <span className="text-caption font-normal text-text-muted tabular-nums">{ended.length}</span>
            <CaretDown className={cn(ROW_ICON, "transition-transform motion-reduce:transition-none", folded && "-rotate-90")} />
          </button>
          {!folded && (
            <ul aria-label="Earlier background work" data-details-list="" className="flex flex-col">
              {ended.map(row)}
            </ul>
          )}
        </>
      )}
    </Section>
  )
}

// A job, as a row, shaped as T3 Code's servers: how it stands and its
// command, which opens to its output; and Stop while it runs.
function JobRow({ connection, session, job, open, toggle, stop }: { connection: Connection; session: string; job: Job; open: boolean; toggle(): void; stop(): void }) {
  const running = job.status === "running"
  return (
    <li className="flex flex-col">
      <div className="relative flex h-row-height">
        <button type="button" aria-expanded={open} onClick={toggle} title={job.title} className={cn(ROW, "font-normal", running && "pr-8")}>
          <TerminalWindow className={ROW_ICON} />
          <span className="min-w-0 flex-1 truncate">
            <span className="font-medium">{running ? "Running" : stateWord(job.status)}</span>
            <span className="text-text-muted"> · </span>
            <span className="font-mono text-caption text-text-muted">{job.title}</span>
          </span>
        </button>
        {running && <StopButton label={`Stop job ${job.id}`} title="Stop the command" stop={stop} />}
      </div>
      {open && <JobOutput connection={connection} session={session} job={job} />}
    </li>
  )
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
    <div className="flex flex-col gap-1 px-2.5 pt-1 pb-2">
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
          className="max-h-job-output overflow-auto font-mono text-caption break-words whitespace-pre-wrap text-text-muted"
        >
          {output.text}
        </pre>
      )}
    </div>
  )
}
