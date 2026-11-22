import { useEffect, useRef, useState, useSyncExternalStore } from "react"
import { addressOf } from "./address.ts"
import { followSubagents, type Lineage } from "./card.ts"
import type { Connection } from "./connection.ts"
import { holdConversation, type Conversation } from "./conversation.ts"
import { Reading } from "./Conversation.tsx"
import { CaretLeft } from "./icons.tsx"
import { subagentsOf } from "./project-card.ts"
import { ConnectionUnderHeader } from "./ConnectionLine.tsx"
import { CardButton, StateWord } from "./ProjectCard.tsx"
import { ListButton } from "./SessionList.tsx"
import { openSubagent, type Subagent } from "./subagent.ts"

// A subagent's conversation in the middle column, read only, under a
// header that names it, with a line below that leads back to the session
// that started it. Its name and how it stands come from that session's
// subagents and jobs, which the project card follows too; its
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
      <header className="flex h-header-height shrink-0 items-center gap-3 pr-4 pl-4">
        <ListButton />
        <h1 className="min-w-0 flex-1 truncate text-title text-text">{found?.name ?? "Subagent"}</h1>
        {found && <StateWord status={found.status} />}
        <CardButton />
      </header>
      <ConnectionUnderHeader />
      <a
        href={addressOf({ session: parentId })}
        className="flex min-h-control shrink-0 items-center gap-1 border-b border-line px-7 pb-2 text-caption text-text-muted hover:text-text"
      >
        <CaretLeft className="size-icon-sm shrink-0" />
        <span className="min-w-0 truncate">Subagent of {parentTitle}</span>
      </a>
      {child && <Reading conversation={child} readOnly />}
    </>
  )
}
