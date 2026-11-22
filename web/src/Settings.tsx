import { useEffect, useRef, useState, useSyncExternalStore, type KeyboardEvent, type ReactNode } from "react"
import type { SettingsChange, SettingsView } from "../../src/core/types.ts"
import { Button } from "./components/ui/button.tsx"
import { Dialog, DialogClose, DialogContent, DialogTitle, DialogTrigger } from "./components/ui/dialog.tsx"
import { DropdownMenu, DropdownMenuContent, DropdownMenuRadioGroup, DropdownMenuRadioItem, DropdownMenuTrigger } from "./components/ui/dropdown-menu.tsx"
import { Input } from "./components/ui/input.tsx"
import type { Connection } from "./connection.ts"
import { FolderPath } from "./FolderPath.tsx"
import { ArrowCounterClockwise, CaretDown, GearSix, X } from "./icons.tsx"
import { cn } from "./lib/utils.ts"
import { keyState, onThisComputer, openServerSettings, pluginState, projectState, type ServerSettings, type Standing } from "./server-settings.ts"
import { Spinner } from "./Spinner.tsx"
import { applyTheme, rememberedTheme, THEMES, type Theme } from "./theme.ts"
import { Words } from "./Words.tsx"

// The settings: a gear that opens a window of them, with its pages in a
// column on the left, as T3 Code's are, or in a menu when the window is
// narrow. Each row has its title and what it is for on the left and its
// control on the right, which moves below them when the window is narrow.
// The theme is this browser's; the rest are kept on evie's server, in its
// data folder's files. A change that applies only once the server starts
// again says so, and Restart to apply starts it again; the page connects
// again by itself. Keys, plugins and trusted projects are changed only on
// the computer evie runs on, and shown elsewhere as they stand.
//
// The settings are followed for as long as the gear is there, so that
// instructions being written are kept when the window is closed, for the
// next time it is opened, and a restart is followed to its end.
export function Settings({ connection }: { connection: Connection }) {
  const [settings, setSettings] = useState<ServerSettings>()
  useEffect(() => {
    const opened = openServerSettings(connection)
    setSettings(opened)
    return () => opened.close()
  }, [connection])
  return (
    <Dialog>
      <DialogTrigger asChild>
        <Button variant="icon" aria-label="Settings">
          <GearSix />
        </Button>
      </DialogTrigger>
      {/* At phone width it fills the screen. */}
      <DialogContent className="@container h-settings-height max-w-settings gap-0 p-0 max-wide:inset-0 max-wide:top-0 max-wide:h-dvh max-wide:max-w-none max-wide:translate-y-0 max-wide:rounded-none max-wide:border-0">
        {settings && <Window settings={settings} />}
      </DialogContent>
    </Dialog>
  )
}

const PAGES = ["Appearance", "Keys", "Plugins", "Instructions", "Projects"] as const
type Page = (typeof PAGES)[number]

// Whether this page is on the computer evie runs on, from its own address.
const here = onThisComputer(location.origin)

function Window({ settings }: { settings: ServerSettings }) {
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
        <Pages page={page} choose={setPage} />
        <div className="@container min-h-0 flex-1 overflow-y-auto px-6 pb-6">
          {page === "Appearance" ? <Appearance /> : <ServerPage page={page} settings={settings} />}
        </div>
      </div>
      <RestartBar settings={settings} />
    </div>
  )
}

// The pages: a column on the left, or, in a narrow window, a menu that
// names the page shown, so that every page is in reach.
function Pages({ page, choose }: { page: Page; choose: (page: Page) => void }) {
  return (
    <nav aria-label="Settings pages" className="shrink-0 px-6 pb-2 @min-[600px]:w-settings-pages @min-[600px]:pr-0">
      <ul className="hidden flex-col gap-1 @min-[600px]:flex">
        {PAGES.map((each) => (
          <li key={each}>
            <button
              type="button"
              aria-current={each === page ? "page" : undefined}
              onClick={() => choose(each)}
              className="flex h-control w-full items-center rounded-md px-3 text-left text-body text-text-muted hover:text-text aria-[current=page]:bg-surface-raised aria-[current=page]:text-text"
            >
              {each}
            </button>
          </li>
        ))}
      </ul>
      <div className="@min-[600px]:hidden">
        <DropdownMenu>
          <DropdownMenuTrigger asChild>
            <Button aria-label={`Page: ${page}`} className="w-full justify-between">
              {page}
              <CaretDown className="size-icon-sm text-text-muted" />
            </Button>
          </DropdownMenuTrigger>
          <DropdownMenuContent align="start">
            <DropdownMenuRadioGroup value={page} onValueChange={(chosen) => choose(chosen as Page)}>
              {PAGES.map((each) => (
                <DropdownMenuRadioItem key={each} value={each}>
                  {each}
                </DropdownMenuRadioItem>
              ))}
            </DropdownMenuRadioGroup>
          </DropdownMenuContent>
        </DropdownMenu>
      </div>
    </nav>
  )
}

// A page's title and what it holds, above its rows.
function PageHead({ title, children }: { title: string; children: ReactNode }) {
  return (
    <div className="flex flex-col gap-1 pb-2">
      <h3 className="text-title text-text">{title}</h3>
      <div className="flex flex-col gap-1 text-caption text-text-muted">{children}</div>
    </div>
  )
}

// Said on the pages whose settings only the computer evie runs on changes,
// when this page is elsewhere.
function OnlyThere({ what }: { what: string }) {
  return here ? null : <p className="text-text">{what} can be changed only on the computer evie runs on.</p>
}

// One setting: its title, what it is for and how it stands on the left,
// the control on the right, or below them in a narrow window. Rows are
// divided by a line, with nothing around them.
function Row({ title, about, standing, control, label }: { title: ReactNode; about?: ReactNode; standing?: Standing; control?: ReactNode; label: string }) {
  return (
    <section aria-label={label} className="flex flex-col gap-3 border-t border-line py-4 @min-[440px]:flex-row @min-[440px]:items-center @min-[440px]:gap-6">
      <div className="flex min-w-0 flex-1 flex-col gap-1">
        <h4 className="text-label break-words text-text">{title}</h4>
        {about !== undefined && <div className="min-w-0 text-caption text-text-muted">{about}</div>}
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
  const { view, problem, draft } = useSyncExternalStore(settings.subscribe, settings.state)
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
  if (page === "Instructions") return <Instructions view={view} draft={draft} settings={settings} />
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
  const env = <code className="font-mono text-code">.env</code>
  return (
    <>
      <PageHead title="Keys">
        <p>
          Kept in {env} in evie's data folder,{" "}
          {view.keysPrivate ? "which only you can read." : "which others on this computer can read too. Saving a key here makes it readable by you only."} A key is
          never shown once it is set, here or anywhere else.
        </p>
        <OnlyThere what="Keys" />
      </PageHead>
      {view.keys.map((key) => (
        <KeyRow key={key.name} item={key} change={change} />
      ))}
    </>
  )
}

// A password manager offers to save, fill or sync what is typed in a
// password box, and in a form it can submit, so a key is typed in a plain
// text box, outside any form, that the managers are told to leave alone,
// and drawn as dots. A browser that cannot draw a text box as dots gets a
// password box instead.
const DOTS = typeof CSS !== "undefined" && CSS.supports("-webkit-text-security", "disc")
const LEAVE_ALONE = { "data-1p-ignore": "", "data-lpignore": "true", "data-bwignore": "", "data-form-type": "other" }

// A key's row: whether it is set, and on this computer a box for a new
// value, sent once and then emptied, so it is kept nowhere, and Clear,
// which takes the key out of the file.
function KeyRow({ item, change }: { item: SettingsView["keys"][number]; change: Change }) {
  const [value, setValue] = useState("")
  const { busy, send, refusedAt } = useSending(change)
  const sendKey = async (made: SettingsChange) => (await send(made)) && setValue("")
  const save = () => value.trim() !== "" && !busy && void sendKey({ key: item.name, value })
  return (
    <>
      <Row
        label={item.name}
        title={<code className="font-mono text-code font-normal">{item.name}</code>}
        {...(item.about !== undefined && { about: item.about })}
        standing={keyState(item)}
        {...(here && {
          control: (
            <div className="flex w-full flex-wrap items-center gap-2 @min-[440px]:w-auto">
              <Input
                type={DOTS ? "text" : "password"}
                autoComplete="off"
                autoCorrect="off"
                autoCapitalize="off"
                spellCheck={false}
                {...LEAVE_ALONE}
                aria-label={`New value for ${item.name}`}
                placeholder={item.inFile ? "A new value" : "Its value"}
                value={value}
                onChange={(event) => setValue(event.target.value)}
                onKeyDown={(event: KeyboardEvent) => event.key === "Enter" && save()}
                className="min-w-0 flex-1 font-mono text-code [-webkit-text-security:disc] @min-[440px]:w-settings-field @min-[440px]:flex-none"
              />
              {value.trim() !== "" && (
                <Button disabled={busy} onClick={save}>
                  Save
                </Button>
              )}
              {item.inFile && value === "" && (
                <Button variant="quiet" disabled={busy} onClick={() => void sendKey({ key: item.name, value: null })}>
                  Clear
                </Button>
              )}
            </div>
          ),
        })}
      />
      <Refused words={refusedAt()} />
    </>
  )
}

// ---- Plugins -------------------------------------------------------------------------------------

// The plugins listed, with whether each is loaded. Only plugin files added
// here can be taken off here; the built-in list is edited in evie.json.
function Plugins({ view, change }: { view: SettingsView; change: Change }) {
  const [adding, setAdding] = useState("")
  const { busy, send, refusedAt } = useSending(change)
  const add = async () => {
    if (adding.trim() !== "" && !busy && (await send({ addPlugin: adding }))) setAdding("")
  }
  const json = <code className="font-mono text-code">evie.json</code>
  return (
    <>
      <PageHead title="Plugins">
        <p>
          What evie's server loads, in order, from {json} in its data folder. A plugin runs with your full permissions, so add only one you trust. A plugin file
          added here can be taken off here; the built-in ones are changed in {json}.
        </p>
        <OnlyThere what="Plugins" />
      </PageHead>
      {view.plugins.map((plugin) => (
        <div key={plugin.name}>
          <Row
            label={plugin.name}
            title={looksLikePath(plugin.name) ? <FolderPath folder={plugin.name} className="font-normal" /> : plugin.name}
            standing={pluginState(plugin)}
            {...(here &&
              plugin.added &&
              plugin.listed && {
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
      {here && (
        <>
          <Row
            label="Add a plugin file"
            title="Add a plugin file"
            about="The full path to a plugin file. evie checks, in a process of its own, that its server would start with it."
            control={
              <div className="flex w-full items-center gap-2 @min-[440px]:w-auto">
                <Input
                  aria-label="A plugin file's path"
                  placeholder="/path/to/plugin.ts"
                  spellCheck={false}
                  value={adding}
                  onChange={(event) => setAdding(event.target.value)}
                  onKeyDown={(event: KeyboardEvent) => event.key === "Enter" && void add()}
                  className="min-w-0 flex-1 font-mono text-code @min-[440px]:w-settings-field @min-[440px]:flex-none"
                />
                {adding.trim() !== "" && (
                  <Button disabled={busy} onClick={() => void add()}>
                    Add
                  </Button>
                )}
              </div>
            }
          />
          <Refused words={refusedAt()} />
        </>
      )}
    </>
  )
}

// A plugin given by a file's path, rather than a built-in one's name.
const looksLikePath = (name: string) => /[/\\]/.test(name)

// ---- Instructions -----------------------------------------------------------------------------------

// The owner's instruction file, in a plain box. What is written and not yet
// saved is kept, whatever is read from the server meanwhile, and when the
// window is closed, until it is saved; without it, the box shows the file
// as the server last read it.
function Instructions({ view, draft, settings }: { view: SettingsView; draft: string | undefined; settings: ServerSettings }) {
  const [saved, setSaved] = useState(false)
  const [refused, setRefused] = useState<string>()
  const [busy, setBusy] = useState(false)
  async function save() {
    setBusy(true)
    const said = await settings.saveInstructions()
    setBusy(false)
    setRefused(said)
    setSaved(said === undefined)
  }
  return (
    <>
      <PageHead title="Instructions">
        <p>
          How you want evie to work, in every project. evie is told it before every message, from the next one. Kept in{" "}
          <code className="font-mono text-code">AGENTS.md</code> in evie's data folder.
        </p>
      </PageHead>
      <div className="flex flex-col gap-3 border-t border-line pt-4">
        <textarea
          aria-label="Your instructions"
          value={draft ?? view.instructions}
          onChange={(event) => {
            settings.editInstructions(event.target.value)
            setSaved(false)
          }}
          rows={12}
          className="w-full resize-y rounded-md border border-line bg-transparent p-3 text-body text-text placeholder:text-text-muted"
          placeholder="Such as: answer briefly, and ask before changing anything outside the project."
        />
        <div className="flex min-h-control items-center justify-end gap-3">
          {draft !== undefined && <span className="text-caption text-text-muted">Not saved yet</span>}
          {draft === undefined && saved && <span className="text-caption text-text-muted">Saved</span>}
          {draft !== undefined && (
            <Button disabled={busy} onClick={() => void save()}>
              Save
            </Button>
          )}
        </div>
        <Refused words={refused} />
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
        <p>
          A project's own settings file, <code className="font-mono text-code">.evie/evie.json</code>, can load plugins, which run with your full permissions,
          and switch protections off. It is used only while the project is trusted.
        </p>
        <OnlyThere what="Trusted projects" />
      </PageHead>
      {view.projects.length === 0 && <p className="border-t border-line pt-4 text-caption text-text-muted">No projects yet.</p>}
      {view.projects.map((project) => (
        <div key={project.id}>
          <Row
            label={project.name}
            title={project.name}
            about={<FolderPath folder={project.folder} />}
            standing={projectState(project)}
            {...(here && {
              control: project.trusted ? (
                <Button variant="quiet" disabled={busy} onClick={() => void trust(project.id, false)}>
                  Stop trusting
                </Button>
              ) : (
                <Button disabled={busy} onClick={() => void trust(project.id, true)}>
                  Trust
                </Button>
              ),
            })}
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
