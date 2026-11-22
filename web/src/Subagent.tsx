import { useEffect, useRef, useState, useSyncExternalStore } from "react"
import { addressOf } from "./address.ts"
import { buttonVariants } from "./components/ui/button.tsx"
import type { Connection } from "./connection.ts"
import { holdConversation, type Conversation } from "./conversation.ts"
import { Reading } from "./Conversation.tsx"
import { CaretLeft } from "./icons.tsx"
import { subagentsOf } from "./project-card.ts"
import { CardButton, StateWord } from "./ProjectCard.tsx"
import { openSubagent, type Subagent } from "./subagent.ts"

// A subagent's conversation in the middle column, read only, under a
// header that names it and leads back to the session that started it.
// Its name and how it stands come from that session's jobs, which the
// project card follows too; its conversation is read as it works.
export function SubagentView({ connection, parent, parentTitle, subagent }: { connection: Connection; parent: string; parentTitle: string; subagent: string }) {
  const [held, setHeld] = useState<Conversation>()
  useEffect(() => {
    const holding = holdConversation(connection, parent)
    setHeld(holding.conversation)
    return holding.release
  }, [connection, parent])
  if (held === undefined) return null
  return <Opened key={subagent} connection={connection} parent={held} parentId={parent} parentTitle={parentTitle} subagent={subagent} />
}

function Opened({ connection, parent, parentId, parentTitle, subagent }: { connection: Connection; parent: Conversation; parentId: string; parentTitle: string; subagent: string }) {
  const state = useSyncExternalStore(parent.subscribe, parent.state)
  const found = subagentsOf(state.jobs).find((each) => each.session === subagent)
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
      <header className="flex h-header-height shrink-0 items-center gap-2 border-b border-line pr-4 pl-4">
        <a href={addressOf({ session: parentId })} aria-label={`Back to ${parentTitle}`} title={`Back to ${parentTitle}`} className={buttonVariants({ variant: "icon" })}>
          <CaretLeft />
        </a>
        <div className="flex min-w-0 flex-1 flex-col">
          <h1 className="truncate text-title text-text">{found?.name ?? "Subagent"}</h1>
          <p className="truncate text-caption text-text-muted">Subagent of {parentTitle}</p>
        </div>
        {found && <StateWord status={found.status} />}
        <CardButton />
      </header>
      {child && <Reading conversation={child} />}
    </>
  )
}
