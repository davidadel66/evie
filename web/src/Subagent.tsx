import { useEffect, useRef, useState, useSyncExternalStore } from "react"
import type { Job } from "../../src/core/types.ts"
import { addressOf } from "./address.ts"
import { followSubagents, type Lineage } from "./card.ts"
import { CardButton, CardLane } from "./CardPlace.tsx"
import type { Connection } from "./connection.ts"
import { holdConversation, type Conversation } from "./conversation.ts"
import { Reading } from "./Conversation.tsx"
import { CaretLeft, Check } from "./icons.tsx"
import { stateWord, subagentsOf } from "./project-card.ts"
import { ConnectionUnderHeader } from "./ConnectionLine.tsx"
import { ListButton } from "./SessionList.tsx"
import { Spinner } from "./Spinner.tsx"
import { openSubagent, type Subagent } from "./subagent.ts"

// A subagent's conversation in the middle column, read only, under a
// header that names it, with a line below that leads back to the session
// that started it. Its name and how it stands come from that session's
// subagents and jobs, which the session details follow too; its
// conversation is read as it works.
export function SubagentView({ connection, parent, parentTitle, subagent }: { connection: Connection; parent: string; parentTitle: string; subagent: string }) {
  const [followed, setFollowed] = useState<{ conversation: Conversation; lineage: Lineage }>()
  useEffect(() => {
    const held = holdConversation(connection, parent)
    const lineage = followSubagents(connection, parent)
    setFollowed({ conversation: held.conversation, lineage })
    return () => {
      held.release()
      lineage.close()
    }
  }, [connection, parent])
  if (followed === undefined) return null
  return <Opened key={subagent} connection={connection} {...followed} parentId={parent} parentTitle={parentTitle} subagent={subagent} />
}

function Opened({
  connection,
  conversation,
  lineage,
  parentId,
  parentTitle,
  subagent,
}: {
  connection: Connection
  conversation: Conversation
  lineage: Lineage
  parentId: string
  parentTitle: string
  subagent: string
}) {
  const state = useSyncExternalStore(conversation.subscribe, conversation.state)
  const { listed } = useSyncExternalStore(lineage.subscribe, lineage.state)
  const found = subagentsOf(listed, state.jobs).find((each) => each.session === subagent)
  const working = found?.status === "running"
  const workingNow = useRef(working)
  workingNow.current = working

  const [child, setChild] = useState<Subagent>()
  useEffect(() => {
    const opened = openSubagent(connection, subagent, { working: () => workingNow.current })
    setChild(opened)
    return () => opened.close()
  }, [connection, subagent])
  // A new round of work, as when it is sent a further message, is read from at once.
  useEffect(() => {
    if (working) child?.poke()
  }, [working, child])

  return (
    <>
      <header className="flex h-header-height shrink-0 items-center gap-2 px-3">
        <ListButton />
        <h1 className="min-w-0 flex-1 truncate text-label text-text">{found?.name ?? "Subagent"}</h1>
        {found && <StateWord status={found.status} />}
        <CardButton />
      </header>
      <ConnectionUnderHeader />
      <a
        href={addressOf({ session: parentId })}
        className="flex min-h-control shrink-0 items-center gap-1 border-b border-line px-gutter pb-2 text-caption text-text-muted hover:text-text"
      >
        <CaretLeft className="size-icon-sm shrink-0" />
        <span className="min-w-0 truncate">Subagent of {parentTitle}</span>
      </a>
      <CardLane>{child && <Reading conversation={child} readOnly />}</CardLane>
    </>
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
