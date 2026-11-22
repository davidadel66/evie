import { useState } from "react"
import { Button } from "./components/ui/button.tsx"
import { FileChip } from "./Composer.tsx"
import type { Conversation } from "./conversation.ts"
import { ArrowUp, CaretDown, PencilSimple, X } from "./icons.tsx"
import { cn } from "./lib/utils.ts"
import type { Queue } from "./queue.ts"

// The messages queued while evie works, at the end of the conversation,
// just above the message box, as T3 Code's queue: each on the owner's
// side, dashed and unfilled, as a message not yet sent, with its files,
// and under it Send now, which sends it at once, into the turn running or
// as a turn of its own; Edit, which puts it in the box; and Remove. One
// being edited says so, with Cancel. A queue longer than three shows its
// first two and a way to show the rest.
export function QueuedMessages({ conversation, queue, turnRunning }: { conversation: Conversation; queue: Queue; turnRunning: boolean }) {
  const [all, setAll] = useState(false)
  const { items } = queue
  const folds = items.length > 3
  const shown = folds && !all ? items.slice(0, 2) : items
  return (
    <section aria-label={`${items.length} queued ${items.length === 1 ? "message" : "messages"}`} className="mt-4 flex flex-col gap-2">
      <ol id="queued-messages" className="flex flex-col items-end gap-3">
        {shown.map((item, at) => {
          const editing = queue.editing === item.key
          return (
            <li key={item.key} aria-label={`Queued message ${at + 1} of ${items.length}, not sent`} aria-current={editing ? "true" : undefined} className="flex w-full flex-col items-end gap-1">
              <div className={cn("flex max-w-4/5 min-w-0 flex-col gap-2 rounded-lg border border-dashed px-3 py-2 text-body", editing ? "border-accent text-text" : "border-text-muted/40 text-text-muted")}>
                {item.files.length > 0 && (
                  <ul aria-label="Files queued" className="flex flex-wrap gap-x-4">
                    {item.files.map((each, index) => (
                      <li key={index} className="flex h-control-sm min-w-0 items-center gap-2">
                        <FileChip file={each.file} />
                      </li>
                    ))}
                  </ul>
                )}
                {item.text !== "" && <p className="break-words whitespace-pre-wrap">{item.text}</p>}
              </div>
              <div className="flex items-center gap-0.5 pr-1 text-caption text-text-muted">
                {editing ? (
                  <>
                    <span className="px-1 text-text">Editing in the message box</span>
                    <Button variant="quiet" onClick={() => conversation.stopEditing()} className="h-control-xs px-2 text-caption">
                      Cancel
                    </Button>
                  </>
                ) : (
                  <>
                    <Button
                      variant="quiet"
                      title={turnRunning ? "Send into the turn running" : "Start a turn with this message"}
                      onClick={() => void conversation.sendQueuedNow(item.key)}
                      className="h-control-xs gap-1 px-2 text-caption [&>svg]:size-icon-row"
                    >
                      <ArrowUp />
                      Send now
                    </Button>
                    <Button variant="icon" aria-label="Edit queued message" title="Edit in the message box" onClick={() => conversation.editQueued(item.key)} className="size-control-xs [&>svg]:size-icon-row">
                      <PencilSimple />
                    </Button>
                    <Button variant="icon" aria-label="Remove queued message" title="Remove from the queue" onClick={() => conversation.unqueue(item.key)} className="size-control-xs [&>svg]:size-icon-row">
                      <X />
                    </Button>
                  </>
                )}
              </div>
            </li>
          )
        })}
      </ol>
      {folds && (
        <button
          type="button"
          aria-expanded={all}
          aria-controls="queued-messages"
          onClick={() => setAll(!all)}
          className="flex items-center gap-1 self-end pr-1 text-caption text-text-muted hover:text-text"
        >
          {all ? "Show fewer" : `Show all ${items.length} queued`}
          <CaretDown className={cn("size-icon-row transition-transform motion-reduce:transition-none", all && "rotate-180")} />
        </button>
      )}
    </section>
  )
}
