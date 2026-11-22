import { useEffect, useRef, useState, useSyncExternalStore, type FormEvent, type ReactNode } from "react"
import type { SettingsChange, SettingsView } from "../../src/core/types.ts"
import { Button } from "./components/ui/button.tsx"
import { Dialog, DialogClose, DialogContent, DialogTitle, DialogTrigger } from "./components/ui/dialog.tsx"
import { DropdownMenu, DropdownMenuContent, DropdownMenuRadioGroup, DropdownMenuRadioItem, DropdownMenuTrigger } from "./components/ui/dropdown-menu.tsx"
import { Input } from "./components/ui/input.tsx"
import type { Connection } from "./connection.ts"
import { ArrowCounterClockwise, CaretDown, GearSix, X } from "./icons.tsx"
import { cn } from "./lib/utils.ts"
import { keyState, openServerSettings, pluginState, projectState, type ServerSettings, type Standing } from "./server-settings.ts"
import { Spinner } from "./Spinner.tsx"
import { applyTheme, rememberedTheme, THEMES, type Theme } from "./theme.ts"
import { Words } from "./Words.tsx"

// The settings: a gear that opens a window of them, with its pages in a
// column on the left, as T3 Code's are. Each row has its title and what it
// is for on the left and its control on the right, which moves below them
// when the window is narrow. The theme is this browser's; the rest are kept
// on evie's server, in its data folder's files. A change that applies only
// once the server starts again says so, and Restart to apply starts it
// again; the page connects again by itself.
export function Settings({ connection }: { connection: Connection }) {
  return (
    <Dialog>
      <DialogTrigger asChild>
        <Button variant="icon" aria-label="Settings">
          <GearSix />
        </Button>
      </DialogTrigger>
      <DialogContent className="@container h-settings-height max-w-settings gap-0 p-0">
        <Window connection={connection} />
      </DialogContent>
    </Dialog>
  )
}

const PAGES = ["Appearance", "Keys", "Plugins", "Instructions", "Projects"] as const
type Page = (typeof PAGES)[number]

// The window's contents, made each time it opens, when the settings are
// read from the server.
function Window({ connection }: { connection: Connection }) {
  const [settings, setSettings] = useState<ServerSettings>()
  useEffect(() => {
    const opened = openServerSettings(connection)
    setSettings(opened)
    return () => opened.close()
  }, [connection])
  const [page, setPage] = useState<Page>("Appearance")
  return (
    <div className="flex min-h-0 flex-1 flex-col">
      <div className="flex shrink-0 items-center justify-between gap-4 px-6 pt-6 pb-4">
        <DialogTitle>Settings</DialogTitle>
        {/* The button's own padding brings its icon to the window's edge. */}
        <DialogClose asChild>
          <Button variant="icon" aria-label="Close" className="-mr-2">
            <X />
          </Button>
        </DialogClose>
      </div>
      <div className="flex min-h-0 flex-1 flex-col @min-[600px]:flex-row">
        <nav aria-label="Settings pages" className="shrink-0 overflow-x-auto px-6 pb-2 @min-[600px]:w-settings-pages @min-[600px]:pr-0">
          <ul className="flex gap-1 @min-[600px]:flex-col">
            {PAGES.map((each) => (
              <li key={each} className="shrink-0">
                <button
                  type="button"
                  aria-current={each === page ? "page" : undefined}
                  onClick={() => setPage(each)}
                  className="flex h-control w-full items-center rounded-md px-3 text-left text-body text-text-muted hover:text-text aria-[current=page]:bg-surface-raised aria-[current=page]:text-text"
                >
                  {each}
                </button>
              </li>
            ))}
          </ul>
        </nav>
        <div className="@container min-h-0 flex-1 overflow-y-auto px-6 pb-6">
          {page === "Appearance" ? <Appearance /> : settings && <ServerPage page={page} settings={settings} />}
        </div>
      </div>
      {settings && <RestartBar settings={settings} />}
    </div>
  )
}

// A page's title and what it holds, above its rows.
function PageHead({ title, children }: { title: string; children: ReactNode }) {
  return (
    <div className="flex flex-col gap-1 pb-2">
      <h3 className="text-title text-text">{title}</h3>
      <p className="text-caption text-text-muted">{children}</p>
    </div>
  )
}

// One setting: its title, what it is for and how it stands on the left,
// the control on the right, or below them in a narrow window. Rows are
// divided by a line, with nothing around them.
function Row({ title, about, standing, control, label }: { title: ReactNode; about?: ReactNode; standing?: Standing; control?: ReactNode; label: string }) {
  return (
    <section aria-label={label} className="flex flex-col gap-3 border-t border-line py-4 @min-[440px]:flex-row @min-[440px]:items-center @min-[440px]:gap-6">
      <div className="flex min-w-0 flex-1 flex-col gap-1">
        <h4 className="text-label break-words text-text">{title}</h4>
        {about !== undefined && <p className="text-caption text-text-muted">{about}</p>}
        {standing !== undefined && (
          <p className="text-caption">
            <span className="text-text">{standing.word}</span>
            {standing.note !== undefined && <span className="text-text-muted">. {standing.note}</span>}
          </p>
        )}
      </div>
      {control !== undefined && <div className="flex shrink-0 flex-wrap items-center gap-2">{control}</div>}
    </section>
  )
}

// What the server said when it refused a change, under the row it was made in.
function Refused({ words }: { words: string | undefined }) {
  return words === undefined ? null : (
    <p role="alert" className="pb-4 text-body text-text">
      <Words text={words} />
    </p>
  )
}

// ---- Appearance ---------------------------------------------------------------------------------

// Dusk, Moss or Oxblood, from a menu, each with its swatch. Choosing one
// puts the page in it at once, and this browser remembers it. Dusk is the
// default, and the reset goes back to it.
function Appearance() {
  const [theme, setTheme] = useState<Theme>(rememberedTheme)
  const choose = (chosen: Theme) => {
    applyTheme(chosen)
    setTheme(chosen)
  }
  const name = THEMES.find((each) => each.id === theme)!.name
  return (
    <>
      <PageHead title="Appearance">How the page looks, in this browser.</PageHead>
      <Row
        label="Theme"
        title="Theme"
        about="The page's colours, all dark. This browser remembers the one chosen."
        control={
          <>
            {theme !== "dusk" && (
              <Button variant="icon" aria-label="Reset to Dusk" onClick={() => choose("dusk")}>
                <ArrowCounterClockwise />
              </Button>
            )}
            <DropdownMenu>
              <DropdownMenuTrigger asChild>
                <Button aria-label={`Theme: ${name}`}>
                  <Swatch theme={theme} />
                  {name}
                  <CaretDown className="size-icon-sm text-text-muted" />
                </Button>
              </DropdownMenuTrigger>
              <DropdownMenuContent align="end">
                <DropdownMenuRadioGroup value={theme} onValueChange={(chosen) => choose(chosen as Theme)}>
                  {THEMES.map((each) => (
                    <DropdownMenuRadioItem key={each.id} value={each.id}>
                      <Swatch theme={each.id} />
                      {each.name}
                    </DropdownMenuRadioItem>
                  ))}
                </DropdownMenuRadioGroup>
              </DropdownMenuContent>
            </DropdownMenu>
          </>
        }
      />
    </>
  )
}

// A theme in small: its ground and its accent, taken from the theme itself.
function Swatch({ theme }: { theme: Theme }) {
  return (
    <span
      aria-hidden="true"
      data-theme={theme}
      className="size-icon-sm shrink-0 rounded-full border border-line"
      style={{ background: "linear-gradient(135deg, var(--bg) 50%, var(--accent) 50%)" }}
    />
  )
}

// ---- The pages kept on the server -----------------------------------------------------------------

function ServerPage({ page, settings }: { page: Exclude<Page, "Appearance">; settings: ServerSettings }) {
  const { view, problem } = useSyncExternalStore(settings.subscribe, settings.state)
  if (view === undefined) {
    return (
      <p role="status" className="flex items-center gap-2 pt-1 text-caption text-text-muted">
        {problem === undefined ? (
          <>
            <Spinner />
            Reading the settings from evie's server
          </>
        ) : (
          <Words text={problem} />
        )}
      </p>
    )
  }
  const change = (made: SettingsChange) => settings.change(made)
  if (page === "Keys") return <Keys view={view} change={change} />
  if (page === "Plugins") return <Plugins view={view} change={change} />
  if (page === "Instructions") return <Instructions view={view} change={change} />
  return <Projects view={view} change={change} />
}

type Change = (change: SettingsChange) => Promise<string | undefined>

// Sends changes one at a time, and keeps what the server said when it
// refused the last, with where on the page it was made. `send` says
// whether the change was made.
function useSending(change: Change) {
  const [busy, setBusy] = useState(false)
  const [refused, setRefused] = useState<{ at: string; words: string }>()
  async function send(made: SettingsChange, at = ""): Promise<boolean> {
    setBusy(true)
    const said = await change(made)
    setBusy(false)
    setRefused(said === undefined ? undefined : { at, words: said })
    return said === undefined
  }
  return { busy, send, refusedAt: (at = "") => (refused?.at === at ? refused.words : undefined) }
}

// ---- Keys ----------------------------------------------------------------------------------------

function Keys({ view, change }: { view: SettingsView; change: Change }) {
  return (
    <>
      <PageHead title="Keys">
        Kept in <code className="font-mono text-code">.env</code> in evie's data folder, which only you can read. A key is never shown once it is set, here or
        anywhere else.
      </PageHead>
      {view.keys.map((key) => (
        <KeyRow key={key.name} item={key} change={change} />
      ))}
    </>
  )
}

// A key's row: whether it is set, a box for a new value, which is sent once
// and then forgotten, and Clear, which takes it out of the file.
function KeyRow({ item, change }: { item: SettingsView["keys"][number]; change: Change }) {
  const [value, setValue] = useState("")
  const { busy, send, refusedAt } = useSending(change)
  // The box is emptied once the value has gone, so it is kept nowhere.
  const sendKey = async (made: SettingsChange) => (await send(made)) && setValue("")
  function save(event: FormEvent) {
    event.preventDefault()
    if (value.trim() !== "" && !busy) void sendKey({ key: item.name, value })
  }
  return (
    <>
      <Row
        label={item.name}
        title={<code className="font-mono text-code">{item.name}</code>}
        {...(item.about !== undefined && { about: item.about })}
        standing={keyState(item)}
        control={
          <form onSubmit={save} className="flex w-full flex-wrap items-center gap-2 @min-[440px]:w-auto">
            <Input
              type="password"
              autoComplete="off"
              spellCheck={false}
              aria-label={`New value for ${item.name}`}
              placeholder={item.inFile ? "A new value" : "Its value"}
              value={value}
              onChange={(event) => setValue(event.target.value)}
              className="min-w-0 flex-1 font-mono text-code @min-[440px]:w-settings-field @min-[440px]:flex-none"
            />
            {value.trim() !== "" && (
              <Button type="submit" disabled={busy}>
                Save
              </Button>
            )}
            {item.inFile && value === "" && (
              <Button variant="quiet" disabled={busy} onClick={() => void sendKey({ key: item.name, value: null })}>
                Clear
              </Button>
            )}
          </form>
        }
      />
      <Refused words={refusedAt()} />
    </>
  )
}

// ---- Plugins -------------------------------------------------------------------------------------

function Plugins({ view, change }: { view: SettingsView; change: Change }) {
  const [adding, setAdding] = useState("")
  const { busy, send, refusedAt } = useSending(change)
  async function add(event: FormEvent) {
    event.preventDefault()
    if (adding.trim() !== "" && !busy && (await send({ addPlugin: adding }))) setAdding("")
  }
  return (
    <>
      <PageHead title="Plugins">
        What evie's server loads, in order, from <code className="font-mono text-code">evie.json</code> in its data folder. A plugin runs with your full
        permissions, so add only one you trust.
      </PageHead>
      {view.plugins.map((plugin) => (
        <div key={plugin.name}>
          <Row
            label={plugin.name}
            title={looksLikePath(plugin.name) ? <code className="font-mono text-code">{plugin.name}</code> : plugin.name}
            standing={pluginState(plugin)}
            {...(plugin.listed && {
              control: (
                <Button variant="quiet" disabled={busy} onClick={() => void send({ removePlugin: plugin.name }, plugin.name)}>
                  Remove
                </Button>
              ),
            })}
          />
          <Refused words={refusedAt(plugin.name)} />
        </div>
      ))}
      <Row
        label="Add a plugin"
        title="Add a plugin"
        about="A built-in plugin's name, such as web, or the full path to a plugin file."
        control={
          <form onSubmit={(event) => void add(event)} className="flex w-full items-center gap-2 @min-[440px]:w-auto">
            <Input
              aria-label="A plugin's name or path"
              spellCheck={false}
              value={adding}
              onChange={(event) => setAdding(event.target.value)}
              className="min-w-0 flex-1 font-mono text-code @min-[440px]:w-settings-field @min-[440px]:flex-none"
            />
            {adding.trim() !== "" && (
              <Button type="submit" disabled={busy}>
                Add
              </Button>
            )}
          </form>
        }
      />
      <Refused words={refusedAt()} />
    </>
  )
}

// A plugin given by a file's path, rather than a built-in one's name.
const looksLikePath = (name: string) => /[/\\]/.test(name)

// ---- Instructions -----------------------------------------------------------------------------------

// The owner's instruction file, in a plain box, with Save once it differs
// from what is saved. The box starts from the file as the window read it.
function Instructions({ view, change }: { view: SettingsView; change: Change }) {
  const [text, setText] = useState(view.instructions)
  const [saved, setSaved] = useState(false)
  const { busy, send, refusedAt } = useSending(change)
  const changed = text !== view.instructions
  return (
    <>
      <PageHead title="Instructions">
        How you want evie to work, in every project. evie is told it before every message, from the next one. Kept in{" "}
        <code className="font-mono text-code">AGENTS.md</code> in evie's data folder.
      </PageHead>
      <div className="flex flex-col gap-3 border-t border-line pt-4">
        <textarea
          aria-label="Your instructions"
          value={text}
          onChange={(event) => {
            setText(event.target.value)
            setSaved(false)
          }}
          rows={12}
          className="w-full resize-y rounded-md border border-line bg-transparent p-3 text-body text-text placeholder:text-text-muted"
          placeholder="Such as: answer briefly, and ask before changing anything outside the project."
        />
        <div className="flex min-h-control items-center justify-end gap-3">
          {saved && !changed && <span className="text-caption text-text-muted">Saved</span>}
          {changed && (
            <Button disabled={busy} onClick={async () => setSaved(await send({ instructions: text }))}>
              Save
            </Button>
          )}
        </div>
        <Refused words={refusedAt()} />
      </div>
    </>
  )
}

// ---- Projects ----------------------------------------------------------------------------------------

function Projects({ view, change }: { view: SettingsView; change: Change }) {
  const { busy, send, refusedAt } = useSending(change)
  const trust = (project: string, trusted: boolean) => send({ project, trusted }, project)
  return (
    <>
      <PageHead title="Projects">
        A project's own settings file, <code className="font-mono text-code">.evie/evie.json</code>, can load plugins, which run with your full permissions,
        and switch protections off. It is used only while the project is trusted.
      </PageHead>
      {view.projects.length === 0 && <p className="border-t border-line pt-4 text-caption text-text-muted">No projects yet.</p>}
      {view.projects.map((project) => (
        <div key={project.id}>
          <Row
            label={project.name}
            title={project.name}
            about={<code className="font-mono text-code break-all">{project.folder}</code>}
            standing={projectState(project)}
            control={
              project.trusted ? (
                <Button variant="quiet" disabled={busy} onClick={() => void trust(project.id, false)}>
                  Stop trusting
                </Button>
              ) : (
                <Button disabled={busy} onClick={() => void trust(project.id, true)}>
                  Trust
                </Button>
              )
            }
          />
          <Refused words={refusedAt(project.id)} />
        </div>
      ))}
    </>
  )
}

// ---- Restarting ----------------------------------------------------------------------------------

// Along the window's foot while a change waits for the server to start
// again: what that is, and Restart to apply. While it restarts, it says so,
// until the server started in its place answers; then that it has.
function RestartBar({ settings }: { settings: ServerSettings }) {
  const { view, restarting } = useSyncExternalStore(settings.subscribe, settings.state)
  const [refused, setRefused] = useState<string>()
  // Whether a restart asked for here has finished, to say so.
  const [restarted, setRestarted] = useState(false)
  const was = useRef(restarting)
  useEffect(() => {
    if (was.current && !restarting && refused === undefined) setRestarted(true)
    was.current = restarting
  }, [restarting, refused])
  const waiting = view?.restart === true
  async function restart() {
    setRestarted(false)
    setRefused(await settings.restart())
  }
  // The status region is always in the window, and only what is in it comes
  // and goes, so that a screen reader, which listens to a region already
  // there, says it.
  return (
    <div role="status" className="shrink-0">
      {(waiting || restarting || restarted || refused !== undefined) && (
        <div className="flex flex-col gap-3 border-t border-line px-6 py-4 @min-[600px]:flex-row @min-[600px]:items-center @min-[600px]:justify-between">
          <p className={cn("flex items-center gap-2 text-caption", restarting ? "text-text-muted" : "text-text")}>
            {restarting ? (
              <>
                <Spinner />
                Restarting evie's server. This page connects again by itself.
              </>
            ) : refused !== undefined ? (
              <Words text={refused} />
            ) : waiting ? (
              "Some changes apply once evie's server starts again. Work in progress carries on where it was."
            ) : (
              "Restarted. Every change applies now."
            )}
          </p>
          {waiting && !restarting && (
            <Button variant="primary" onClick={() => void restart()}>
              Restart to apply
            </Button>
          )}
        </div>
      )}
    </div>
  )
}
