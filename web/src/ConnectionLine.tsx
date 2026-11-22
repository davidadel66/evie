import { useContext } from "react"
import { connectionLine, type ConnectionState } from "./connection.ts"
import { ConnectionShown } from "./connection-shown.ts"
import { useListShown } from "./list-shown.ts"
import { Spinner } from "./Spinner.tsx"
import { Words } from "./Words.tsx"

// Nothing while connected. While the line is not up, one quiet line, as
// Working is said, with why, until it is back. The status region is always
// on the page, and only its words come and go, so that a screen reader,
// which listens to a region already there, says them.
export function ConnectionLine({ state }: { state: ConnectionState }) {
  const line = connectionLine(state)
  return (
    <div role="status">
      {line !== undefined && (
        <p className="flex items-start gap-2 px-gutter pb-3 text-caption text-text-muted">
          <span className="flex h-lh items-center">
            <Spinner />
          </span>
          <span>
            {line.word}
            {line.reason !== undefined && (
              <>
                {". "}
                <Words text={line.reason} />
              </>
            )}
          </span>
        </p>
      )}
    </div>
  )
}

// The same line under a view's header at phone width, where the list's
// sheet, which has it at desktop width, is out of sight: under a session's
// conversation's header, or a subagent's.
export function ConnectionUnderHeader() {
  const state = useContext(ConnectionShown)
  const list = useListShown()
  if (list.wide || state === undefined) return null
  return (
    <div className="pt-3">
      <ConnectionLine state={state} />
    </div>
  )
}
