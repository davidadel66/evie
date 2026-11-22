import { useEffect, useId, useState, type ReactNode } from "react"
import { Button } from "./components/ui/button.tsx"
import { Dialog, DialogClose, DialogContent, DialogTitle, DialogTrigger } from "./components/ui/dialog.tsx"
import { Input } from "./components/ui/input.tsx"
import type { Connection } from "./connection.ts"
import { X } from "./icons.tsx"
import type { Sessions } from "./sessions.ts"
import { addressOfWorkspace, fullFolder, suggestedFolder } from "./workspace.ts"
import { makeWorkspace, readHome } from "./workspace-actions.ts"

// The New workspace dialog, which makes a project with `new_project`: its
// name; its folder, suggested in the home folder of the computer evie runs
// on and named after it, or any other, such as a code repository already
// there, which is added as it is; and whether its .evie folder is backed up
// to a private repository. What the server refuses is said in its words.
// Once made, the page opens the new workspace.
export function NewWorkspace({ connection, sessions, children }: { connection: Connection; sessions: Sessions; children: ReactNode }) {
  const [open, setOpen] = useState(false)
  return (
    <Dialog open={open} onOpenChange={setOpen}>
      <DialogTrigger asChild>{children}</DialogTrigger>
      <DialogContent>
        <div className="flex items-center justify-between gap-4">
          <DialogTitle>New workspace</DialogTitle>
          {/* The button's own padding brings its icon to the window's edge. */}
          <DialogClose asChild>
            <Button variant="icon" aria-label="Close" className="-mr-2">
              <X />
            </Button>
          </DialogClose>
        </div>
        {/* Made afresh each time the dialog opens, since its content is. */}
        <Form
          connection={connection}
          sessions={sessions}
          made={(project) => {
            setOpen(false)
            location.hash = addressOfWorkspace({ workspace: project, tab: "sessions" })
          }}
        />
      </DialogContent>
    </Dialog>
  )
}

function Form({ connection, sessions, made }: { connection: Connection; sessions: Sessions; made(project: string): void }) {
  const [home, setHome] = useState<string>()
  const [name, setName] = useState("")
  // The folder the owner typed, or until then the one suggested.
  const [typed, setTyped] = useState<string>()
  const [backUp, setBackUp] = useState(true)
  const [problem, setProblem] = useState<string>()
  const [making, setMaking] = useState(false)
  const ids = { name: useId(), folder: useId(), about: useId() }
  useEffect(() => {
    void readHome(connection).then((folder) => folder !== undefined && setHome(folder))
  }, [connection])

  const folder = typed ?? (home === undefined || name.trim() === "" ? "" : suggestedFolder(home, name))
  async function submit() {
    if (making || folder.trim() === "") return
    setMaking(true)
    setProblem(undefined)
    const result = await makeWorkspace(connection, sessions, {
      folder: home === undefined ? folder.trim() : fullFolder(folder, home),
      ...(name.trim() !== "" && { name: name.trim() }),
      sync: backUp ? "private" : "none",
    })
    setMaking(false)
    if ("problem" in result) setProblem(result.problem)
    else made(result.project.id)
  }

  return (
    <form
      className="flex flex-col gap-4"
      onSubmit={(event) => {
        event.preventDefault()
        void submit()
      }}
    >
      <div className="flex flex-col gap-1">
        <label htmlFor={ids.name} className="text-label text-text">
          Name
        </label>
        <Input id={ids.name} autoFocus autoComplete="off" placeholder="Finances" value={name} onChange={(event) => setName(event.target.value)} />
      </div>
      <div className="flex flex-col gap-1">
        <label htmlFor={ids.folder} className="text-label text-text">
          Folder
        </label>
        <Input
          id={ids.folder}
          autoComplete="off"
          spellCheck={false}
          aria-describedby={ids.about}
          placeholder={home === undefined ? "/Users/me/finances" : suggestedFolder(home, "finances")}
          value={folder}
          onChange={(event) => setTyped(event.target.value)}
          className="font-mono text-code"
        />
        <p id={ids.about} className="text-caption text-text-muted">
          Made when it is missing. A folder already there, such as a code repository, is added as it is.
        </p>
      </div>
      <label className="flex min-h-control items-center gap-3 text-body text-text">
        <input type="checkbox" checked={backUp} onChange={(event) => setBackUp(event.target.checked)} className="size-icon-sm shrink-0 accent-accent" />
        Back up its .evie folder to a private repository on your GitHub account
      </label>
      {problem !== undefined && (
        <p role="status" className="text-caption break-words text-text-muted">
          {problem}
        </p>
      )}
      <div className="flex justify-end gap-2">
        <DialogClose asChild>
          <Button variant="quiet">Cancel</Button>
        </DialogClose>
        <Button type="submit" variant="primary" disabled={making || folder.trim() === ""}>
          {making ? "Making it" : "Make workspace"}
        </Button>
      </div>
    </form>
  )
}
