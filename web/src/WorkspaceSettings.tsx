import { useEffect, useState, useSyncExternalStore, type ReactNode } from "react"
import type { ListedProject } from "../../src/core/types.ts"
import { Button } from "./components/ui/button.tsx"
import type { Connection } from "./connection.ts"
import { openProjectInstructions, type ProjectInstructionsView } from "./project-instructions.ts"
import { Words } from "./Words.tsx"

// The workspace's Settings tab: what is set for this workspace alone, in
// sections divided by lines, as the Storage tab's content sits on the page
// with no cards. Its sections are read from evie's server as the tab opens.
// For now it holds the workspace's instructions: the repository's own
// `AGENTS.md` or `CLAUDE.md`, shown as it is and never written, and the
// owner's notes for this workspace, kept in `.evie/instructions.md` and
// given to the model after the repository's file.
export function WorkspaceSettings({ connection, project }: { connection: Connection; project: ListedProject }) {
  return (
    <section aria-label="Workspace settings" className="flex max-w-settings-content flex-col px-gutter py-5 [&>*+*]:mt-6 [&>*+*]:border-t [&>*+*]:border-line [&>*+*]:pt-6">
      <InstructionsSection connection={connection} project={project} />
    </section>
  )
}

// A section: its title, what it is for, then its content.
function Section({ title, about, children }: { title: string; about: ReactNode; children: ReactNode }) {
  return (
    <div className="flex flex-col gap-3">
      <div className="flex flex-col gap-1">
        <h2 className="text-label font-medium text-text">{title}</h2>
        <div className="max-w-settings-about text-caption text-text-muted">{about}</div>
      </div>
      {children}
    </div>
  )
}

const Code = ({ children }: { children: ReactNode }) => <code className="font-mono text-code">{children}</code>

function InstructionsSection({ connection, project }: { connection: Connection; project: ListedProject }) {
  const [opened, setOpened] = useState<{ project: string; view: ProjectInstructionsView }>()
  useEffect(() => {
    const view = openProjectInstructions(connection, project.id)
    setOpened({ project: project.id, view })
    return () => view.close()
  }, [connection, project.id])
  if (opened?.project !== project.id) return null
  return <Instructions view={opened.view} />
}

function Instructions({ view }: { view: ProjectInstructionsView }) {
  const { read, draft, problem } = useSyncExternalStore(view.subscribe, view.state)
  const [saved, setSaved] = useState(false)
  const [refused, setRefused] = useState<string>()
  const [busy, setBusy] = useState(false)
  async function save() {
    setBusy(true)
    const said = await view.save()
    setBusy(false)
    setRefused(said)
    setSaved(said === undefined)
  }
  const about = (
    <p>
      How you want evie to work in this workspace, told to it before every message, from the next one. A repository's own <Code>AGENTS.md</Code> or <Code>CLAUDE.md</Code> is
      used as it is and never changed here. Your notes are kept in <Code>.evie/instructions.md</Code>, come after it, and win where they differ.
    </p>
  )
  if (read === undefined) {
    return (
      <Section title="Instructions" about={about}>
        <p role="status" className="text-caption text-text-muted">
          {problem === undefined ? "Reading the instructions from evie's server" : <Words text={problem} />}
        </p>
      </Section>
    )
  }
  return (
    <Section title="Instructions" about={about}>
      <div className="flex flex-col gap-1">
        <h3 className="text-body font-medium text-text">From the repository</h3>
        {read.repo === undefined ? (
          <p className="text-caption text-text-muted">
            This workspace's folder has no <Code>AGENTS.md</Code> or <Code>CLAUDE.md</Code>.
          </p>
        ) : (
          <>
            <p className="text-caption text-text-muted">
              <Code>{read.repo.name}</Code> is in use{read.repo.cut ? ", up to its size limit" : ""}. Read only.
              {read.ignored.length > 0 && (
                <>
                  {" "}
                  <Code>{read.ignored.join(", ")}</Code> also {read.ignored.length === 1 ? "has" : "have"} text and is left out: evie uses the first it finds.
                </>
              )}
            </p>
            <pre
              tabIndex={0}
              aria-label={`${read.repo.name}, read only`}
              className="max-h-64 overflow-auto rounded-md border border-line p-3 font-mono text-code whitespace-pre-wrap text-text-muted"
            >
              {read.repo.text}
            </pre>
          </>
        )}
      </div>
      <div className="flex flex-col gap-2">
        <h3 className="text-body font-medium text-text">Your notes for this workspace</h3>
        <textarea
          aria-label="Your notes for this workspace"
          value={draft ?? read.notes}
          disabled={!read.canWrite}
          onChange={(event) => {
            view.edit(event.target.value)
            setSaved(false)
          }}
          rows={10}
          className="w-full resize-y rounded-md border border-line bg-transparent p-3 text-field text-text placeholder:text-text-muted disabled:opacity-60"
          placeholder="Such as: run the tests before saying a change is done, and never touch the generated files."
        />
        {!read.canWrite && (
          <p role="alert" className="text-caption text-text">
            This workspace's .evie folder is a link, or inside evie's data folder, so notes cannot be saved for it.
          </p>
        )}
        <div className="flex min-h-control items-center justify-end gap-3">
          {draft !== undefined && <span className="text-caption text-text-muted">Not saved yet</span>}
          {draft === undefined && saved && <span className="text-caption text-text-muted">Saved</span>}
          {draft !== undefined && (
            <Button disabled={busy} onClick={() => void save()}>
              Save
            </Button>
          )}
        </div>
        {refused !== undefined && (
          <p role="alert" className="text-body text-text">
            <Words text={refused} />
          </p>
        )}
      </div>
    </Section>
  )
}
