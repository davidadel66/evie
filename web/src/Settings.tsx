import * as DialogPrimitive from "@radix-ui/react-dialog"
import { createContext, useContext, useEffect, useRef, useState, useSyncExternalStore, type KeyboardEvent, type ReactNode } from "react"
import type { SettingsChange, SettingsView } from "../../src/core/types.ts"
import { settingsIn } from "./address.ts"
import { Button } from "./components/ui/button.tsx"
import { DropdownMenu, DropdownMenuContent, DropdownMenuRadioGroup, DropdownMenuRadioItem, DropdownMenuTrigger } from "./components/ui/dropdown-menu.tsx"
import { Input } from "./components/ui/input.tsx"
import { Switch } from "./components/ui/switch.tsx"
import type { Connection } from "./connection.ts"
import { FolderPath } from "./FolderPath.tsx"
import { ArrowUUpLeft, Bell, CaretDown, CaretRight, FolderSimple, GearSix, Key, MagnifyingGlass, Palette, PuzzlePiece, Scroll, X } from "./icons.tsx"
import { cn } from "./lib/utils.ts"
import { keyState, onThisComputer, openServerSettings, pluginState, projectState, type ServerSettings, type Standing } from "./server-settings.ts"
import { Spinner } from "./Spinner.tsx"
import { keepNotices, NOTICE_DEFAULTS, noticeChoice, rememberedNotices, type NoticeMode, type NoticeSettings } from "./notices.ts"
import { closeSettings, openSettings, showSettingsPage, useAddress } from "./selection.ts"
import { PAGE_NAMES, searchSettings, SETTINGS_GROUPS, settingItems, type SettingItem, type SettingsPage as Page } from "./settings-page.ts"
import { onMac } from "./shortcuts.ts"
import { unlockSound } from "./sound.ts"
import { applyTheme, rememberedTheme, THEMES, type Theme } from "./theme.ts"
import { Words } from "./Words.tsx"

// The settings, as T3 Code's: a page of their own, with its own address
// (see address.ts), nearly as large as the window over the page dimmed
// under it, and the whole screen at phone width. Its pages are down the
// left, in groups, each with its icon, under a search over every setting,
// or at phone width a rail of the icons. Each setting has its title and
// what it is for on the left and its control on the right, which moves
// below them when the page is narrow; one with a default, once changed,
// has a reset after its title. The theme and the notifications are this
// browser's; the rest are kept on evie's server, in its data folder's
// files. A change that applies only once the server starts again says so,
// and Restart to apply starts it again; the page connects again by itself.
// Keys, plugins and trusted projects are changed only on the computer evie
// runs on, and shown elsewhere as they stand; a key is never shown back.
//
// The settings are followed for as long as the frame is there, so that
// instructions being written are kept when the page is closed, for the
// next time it is opened, and a restart is followed to its end.

// The gear that opens the settings, as T3 Code's settings button.
export function SettingsButton() {
  return (
    <Button variant="icon" aria-label="Settings" title="Settings" onClick={() => openSettings()}>
      <GearSix />
    </Button>
  )
}

export function SettingsPage({ connection }: { connection: Connection }) {
  const [settings, setSettings] = useState<ServerSettings>()
  useEffect(() => {
    const opened = openServerSettings(connection)
    setSettings(opened)
    return () => opened.close()
  }, [connection])
  const open = settingsIn(useAddress())
  const surface = useRef<HTMLDivElement>(null)
  return (
    <DialogPrimitive.Root open={open !== undefined} onOpenChange={(next) => !next && closeSettings()}>
      <DialogPrimitive.Portal>
        <DialogPrimitive.Overlay className="settings-backdrop fixed inset-0 z-50" />
        <DialogPrimitive.Content
          ref={surface}
          data-slot="dialog-content"
          aria-describedby={undefined}
          tabIndex={-1}
          // The page takes the focus, not its first control, so that no
          // ring shows and no phone's keyboard opens, as T3 Code's.
          onOpenAutoFocus={(event) => {
            event.preventDefault()
            surface.current?.focus()
          }}
          // Escape clears a search first, as T3 Code's.
          onEscapeKeyDown={(event) => {
            const focused = document.activeElement
            if (focused instanceof HTMLInputElement && focused.dataset.settingsSearch !== undefined && focused.value !== "") event.preventDefault()
          }}
          className="fixed inset-0 z-50 m-auto flex max-h-settings-page-height max-w-settings-page flex-col overflow-hidden bg-bg text-body text-text outline-none wide:inset-6 wide:rounded-lg wide:border wide:border-line wide:shadow-float"
        >
          {settings && open && <Window page={open.page} settings={settings} />}
        </DialogPrimitive.Content>
      </DialogPrimitive.Portal>
    </DialogPrimitive.Root>
  )
}

// Whether this page is on the computer evie runs on, from its own address.
const here = onThisComputer(location.origin)

// Each page's icon, as T3 Code's column has one for each.
const PAGE_ICONS: Record<Page, typeof Palette> = { appearance: Palette, notifications: Bell, instructions: Scroll, keys: Key, plugins: PuzzlePiece, projects: FolderSimple }

// The setting a search went to, which its row shows for a moment.
const Found = createContext<string | undefined>(undefined)

function Window({ page, settings }: { page: Page; settings: ServerSettings }) {
  const { view } = useSyncExternalStore(settings.subscribe, settings.state)
  const [found, setFound] = useState<string>()
  // A setting found is scrolled to and shown, once its page is drawn.
  useEffect(() => {
    if (found === undefined) return
    const row = document.querySelector<HTMLElement>(`[data-setting="${CSS.escape(found)}"]`)
    row?.scrollIntoView({ block: "center" })
    row?.focus({ preventScroll: true })
    const timer = setTimeout(() => setFound(undefined), 1500)
    return () => clearTimeout(timer)
  }, [found, page])
  return (
    <div className="relative flex min-h-0 flex-1">
      <Pages
        page={page}
        items={settingItems(view, here)}
        go={(item) => {
          showSettingsPage(item.page)
          setFound(item.id)
        }}
      />
      <div className="@container flex min-w-0 flex-1 flex-col">
        <header className="flex h-settings-header shrink-0 items-center justify-between gap-2 border-b border-line pr-2 pl-5">
          <DialogPrimitive.Title className="flex min-w-0 items-center gap-1.5 text-body font-normal">
            <span className="text-text-muted">Settings</span>
            <CaretRight className="size-icon-xs shrink-0 text-text-muted" />
            <span className="truncate font-medium text-text">{PAGE_NAMES[page]}</span>
          </DialogPrimitive.Title>
          <DialogPrimitive.Close asChild>
            <Button variant="icon" aria-label="Close" title="Close (Esc)" className="size-control-sm">
              <X />
            </Button>
          </DialogPrimitive.Close>
        </header>
        <div className="min-h-0 flex-1 overflow-y-auto">
          <div className="mx-auto flex w-full max-w-settings-content flex-col gap-4 px-6 pt-6 pb-8">
            <Found.Provider value={found}>
              {page === "appearance" ? <Appearance /> : page === "notifications" ? <Notifications /> : <ServerPage page={page} settings={settings} />}
            </Found.Provider>
          </div>
        </div>
        <RestartBar settings={settings} />
      </div>
    </div>
  )
}

// The column of pages, as T3 Code's: a search over every setting at its
// top, with / or mod+F to reach it, then the pages in their groups, each
// with its icon, the one shown filled. While something is typed, the
// settings found take the pages' place, each over the page it is on; Up
// and Down move among them, Enter goes to one, and Escape clears the
// search. At phone width it is a rail of the icons alone, whose search
// opens the column over the page.
function Pages({ page, items, go }: { page: Page; items: SettingItem[]; go(item: SettingItem): void }) {
  const [query, setQuery] = useState("")
  const [active, setActive] = useState(0)
  const [opened, setOpened] = useState(false)
  const search = useRef<HTMLInputElement>(null)
  const results = searchSettings(items, query)
  const searching = query.trim() !== ""
  const at = Math.min(active, Math.max(0, results.length - 1))
  useEffect(() => {
    const mac = onMac()
    const onKey = (event: globalThis.KeyboardEvent) => {
      if (event.defaultPrevented || event.isComposing) return
      const find = event.key.toLowerCase() === "f" && (mac ? event.metaKey : event.ctrlKey) && !event.altKey && !event.shiftKey
      const slash = event.key === "/" && !event.metaKey && !event.ctrlKey && !event.altKey
      if (!find && !slash) return
      const target = event.target
      const typing = target instanceof HTMLInputElement || target instanceof HTMLTextAreaElement || (target instanceof HTMLElement && target.isContentEditable)
      if (slash && typing) return
      event.preventDefault()
      setOpened(true)
      search.current?.focus()
      search.current?.select()
    }
    addEventListener("keydown", onKey)
    return () => removeEventListener("keydown", onKey)
  }, [])
  useEffect(() => {
    if (opened) search.current?.focus()
  }, [opened])
  const clear = () => {
    setQuery("")
    setActive(0)
  }
  const choose = (item: SettingItem) => {
    clear()
    setOpened(false)
    go(item)
  }
  function onKeyDown(event: KeyboardEvent<HTMLInputElement>) {
    if (event.nativeEvent.isComposing) return
    if (event.key === "Escape" && searching) {
      event.preventDefault()
      clear()
    } else if (results.length === 0) return
    else if (event.key === "ArrowDown" || event.key === "ArrowUp") {
      event.preventDefault()
      setActive((at + (event.key === "ArrowDown" ? 1 : results.length - 1)) % results.length)
    } else if (event.key === "Enter") {
      event.preventDefault()
      choose(results[at]!)
    }
  }
  const narrow = !opened && "max-wide:sr-only"
  return (
    <nav
      aria-label="Settings pages"
      className={cn(
        "flex w-settings-nav shrink-0 flex-col gap-2 overflow-y-auto border-r border-line bg-bg-side p-2",
        // Opened at phone width, under the header, whose Close stays in reach.
        opened ? "max-wide:absolute max-wide:inset-x-0 max-wide:top-settings-header max-wide:bottom-0 max-wide:z-10 max-wide:w-full max-wide:border-r-0" : "max-wide:w-settings-rail",
      )}
    >
      <label className={cn("flex h-row-height shrink-0 items-center gap-2 rounded-md px-2 text-text-muted hover:bg-surface", !opened && "max-wide:hidden")}>
        <MagnifyingGlass className="size-icon-sm shrink-0" />
        <input
          ref={search}
          data-settings-search=""
          type="text"
          value={query}
          onChange={(event) => {
            setQuery(event.target.value)
            setActive(0)
          }}
          onKeyDown={onKeyDown}
          onBlur={() => !searching && setOpened(false)}
          placeholder="Search"
          aria-label="Search settings"
          role="combobox"
          aria-autocomplete="list"
          aria-expanded={searching && results.length > 0}
          aria-controls={searching && results.length > 0 ? "settings-found" : undefined}
          aria-activedescendant={searching && results[at] !== undefined ? `settings-found-${at}` : undefined}
          className="min-w-0 flex-1 bg-transparent text-field text-text outline-none placeholder:text-text-muted"
        />
        {searching ? (
          <Button
            variant="icon"
            aria-label="Clear the search"
            onClick={() => {
              clear()
              search.current?.focus()
            }}
            className="size-reset rounded-sm [&>svg]:size-icon-xs"
          >
            <X />
          </Button>
        ) : (
          <kbd className="rounded-sm border border-line px-1 font-sans text-caption text-text-muted">/</kbd>
        )}
      </label>
      {searching ? (
        results.length === 0 ? (
          <p role="status" className="px-2 py-6 text-center text-caption text-text-muted">
            No settings found
          </p>
        ) : (
          <ul id="settings-found" role="listbox" aria-label="Settings found" className="flex flex-col gap-0.5">
            {results.map((item, index) => {
              const Icon = PAGE_ICONS[item.page]
              return (
                <li
                  key={item.id}
                  id={`settings-found-${index}`}
                  role="option"
                  aria-selected={index === at}
                  onMouseDown={(event) => event.preventDefault()}
                  onMouseMove={() => index !== at && setActive(index)}
                  onClick={() => choose(item)}
                  className={cn("flex cursor-default items-start gap-2 rounded-md px-2 py-1.5", index === at && "bg-surface-raised")}
                >
                  <Icon className="mt-0.5 size-icon-sm shrink-0 text-text-muted" />
                  <span className="flex min-w-0 flex-1 flex-col">
                    <span className="truncate text-body font-medium text-text">{item.title}</span>
                    <span className="truncate text-caption text-text-muted">{PAGE_NAMES[item.page]}</span>
                  </span>
                </li>
              )
            })}
          </ul>
        )
      ) : (
        <>
          {!opened && (
            <Button variant="icon" aria-label="Search settings" onClick={() => setOpened(true)} className="h-row-height w-full wide:hidden">
              <MagnifyingGlass />
            </Button>
          )}
          <div className="flex flex-col gap-4">
            {SETTINGS_GROUPS.map((group, index) => (
              <div key={group.heading} className="flex flex-col gap-1 max-wide:border-t max-wide:border-line max-wide:pt-4 max-wide:first:border-t-0 max-wide:first:pt-0">
                <h3 id={`settings-group-${index}`} className={cn("px-2 text-caption font-medium text-text-muted", group.headingHidden ? "sr-only" : narrow)}>
                  {group.heading}
                </h3>
                <ul aria-labelledby={`settings-group-${index}`} className="flex flex-col gap-0.5">
                  {group.pages.map((each) => {
                    const Icon = PAGE_ICONS[each]
                    return (
                      <li key={each}>
                        <button
                          type="button"
                          aria-current={each === page ? "page" : undefined}
                          title={PAGE_NAMES[each]}
                          onClick={() => {
                            setOpened(false)
                            showSettingsPage(each)
                          }}
                          className={cn(
                            "flex h-row-height w-full items-center gap-2 rounded-md px-2 text-left text-body font-medium text-text-muted hover:bg-surface hover:text-text aria-[current=page]:bg-surface-raised aria-[current=page]:text-text",
                            !opened && "max-wide:justify-center",
                          )}
                        >
                          <Icon className="size-icon-sm shrink-0" />
                          <span className={cn("truncate", narrow)}>{PAGE_NAMES[each]}</span>
                        </button>
                      </li>
                    )
                  })}
                </ul>
              </div>
            ))}
          </div>
        </>
      )}
    </nav>
  )
}

// What a page holds, above its settings.
function About({ children }: { children: ReactNode }) {
  return <div className="flex max-w-settings-about flex-col gap-1 px-3 text-caption text-text-muted wide:px-4">{children}</div>
}

// A page's settings, divided by lines, with nothing round them, as the
// design system has its content.
function Group({ children }: { children: ReactNode }) {
  return <div className="flex flex-col [&>*+*]:border-t [&>*+*]:border-line">{children}</div>
}

// Said on the pages whose settings only the computer evie runs on changes,
// when this page is elsewhere.
function OnlyThere({ what }: { what: string }) {
  return here ? null : <p className="text-text">{what} can be changed only on the computer evie runs on.</p>
}

// One setting, as T3 Code's row: its title, then, once changed from its
// default, a reset; what it is for and how it stands under them; and the
// control on the right, or below them when the page is narrow. A setting
// a search went to is shown for a moment.
function Row({
  id,
  title,
  about,
  standing,
  control,
  label,
  reset,
}: {
  id: string
  title: ReactNode
  about?: ReactNode
  standing?: Standing
  control?: ReactNode
  label: string
  reset?: { label: string; reset(): void } | undefined
}) {
  const found = useContext(Found) === id
  return (
    <section
      data-setting={id}
      tabIndex={-1}
      aria-label={label}
      className={cn("@container px-3 py-3 outline-none transition-colors motion-reduce:transition-none wide:px-4", found && "rounded-md bg-surface-raised")}
    >
      <div className="flex flex-col gap-3 @min-[512px]:flex-row @min-[512px]:items-center @min-[512px]:gap-8">
        <div className="flex min-w-0 flex-1 flex-col gap-1">
          <div className="flex min-h-5 items-center gap-1.5">
            <h4 className="text-body font-medium break-words text-text">{title}</h4>
            {reset !== undefined && (
              <Button variant="icon" aria-label={reset.label} title="Reset to default" onClick={reset.reset} className="size-reset rounded-sm [&>svg]:size-icon-xs">
                <ArrowUUpLeft />
              </Button>
            )}
          </div>
          {about !== undefined && <div className="max-w-settings-about min-w-0 text-caption text-text-muted">{about}</div>}
          {standing !== undefined && (
            <p className="text-caption">
              <span className="text-text">{standing.word}</span>
              {standing.note !== undefined && <span className="text-text-muted">. {standing.note}</span>}
            </p>
          )}
        </div>
        {control !== undefined && <div className="flex w-full min-w-0 shrink-0 flex-wrap items-center gap-2 @min-[512px]:w-auto @min-[512px]:justify-end">{control}</div>}
      </div>
    </section>
  )
}

// What the server said when it refused a change, under the row it was made in.
function Refused({ words }: { words: string | undefined }) {
  return words === undefined ? null : (
    <p role="alert" className="px-3 pb-3 text-body text-text wide:px-4">
      <Words text={words} />
    </p>
  )
}

// A choice of a few, as T3 Code's select: a menu as wide as it, at least
// so wide, with the one chosen marked.
function Choice<Value extends string>({ label, value, choices, choose, shown }: { label: string; value: Value; choices: readonly { value: Value; name: string }[]; choose(value: Value): void; shown?(value: Value): ReactNode }) {
  const name = choices.find((each) => each.value === value)?.name ?? value
  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button aria-label={`${label}: ${name}`} className="min-w-settings-select justify-between font-normal">
          <span className="flex min-w-0 items-center gap-2">
            {shown?.(value)}
            <span className="truncate">{name}</span>
          </span>
          <CaretDown className="size-icon-control text-text-muted" />
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end" className="min-w-settings-select">
        <DropdownMenuRadioGroup value={value} onValueChange={(chosen) => choose(chosen as Value)}>
          {choices.map((each) => (
            <DropdownMenuRadioItem key={each.value} value={each.value}>
              {shown?.(each.value)}
              {each.name}
            </DropdownMenuRadioItem>
          ))}
        </DropdownMenuRadioGroup>
      </DropdownMenuContent>
    </DropdownMenu>
  )
}

// ---- Appearance ---------------------------------------------------------------------------------

// Dusk, Moss or Oxblood, each with its swatch. Choosing one puts the page
// in it at once, and this browser remembers it. Dusk is the default, which
// the reset goes back to.
function Appearance() {
  const [theme, setTheme] = useState<Theme>(rememberedTheme)
  const choose = (chosen: Theme) => {
    applyTheme(chosen)
    setTheme(chosen)
  }
  return (
    <>
      <About>How the page looks, in this browser.</About>
      <Group>
        <Row
          id="theme"
          label="Theme"
          title="Theme"
          about="The page's colours, all dark. This browser remembers the one chosen."
          {...(theme !== "dusk" && { reset: { label: "Reset the theme to Dusk", reset: () => choose("dusk") } })}
          control={<Choice label="Theme" value={theme} choices={THEMES.map((each) => ({ value: each.id, name: each.name }))} choose={choose} shown={(each) => <Swatch theme={each} />} />}
        />
      </Group>
    </>
  )
}

// A theme in small: its ground and its accent, taken from the theme itself.
export function Swatch({ theme }: { theme: Theme }) {
  return (
    <span
      aria-hidden="true"
      data-theme={theme}
      className="size-icon-sm shrink-0 rounded-full border border-line"
      style={{ background: "linear-gradient(135deg, var(--bg) 50%, var(--accent) 50%)" }}
    />
  )
}

// ---- Notifications ----------------------------------------------------------------------------

// What evie tells the owner, and how, in this browser, as T3 Code's
// notifications: when a session needs them, or a turn finishes or fails.
// The system's notifications are off until the owner turns them on, and the
// browser is asked to allow them only then; a choice of them is kept only
// once it does, as T3 Code's, and otherwise the row says what to do.
// Notices in the page are a switch of their own, on until turned off. Each
// has its reset.
function Notifications() {
  const [settings, setSettings] = useState<NoticeSettings>(rememberedNotices)
  const [allowed, setAllowed] = useState(permission)
  const [refused, setRefused] = useState<string>()
  const keep = (next: NoticeSettings) => {
    keepNotices(next)
    setSettings(next)
  }
  async function choose(mode: NoticeMode) {
    // In the click that chose it, so that the browser lets it play.
    unlockSound(mode)
    if ((mode === "on" || mode === "both") && permission() === "default") await Notification.requestPermission().catch(() => {})
    const now = permission()
    setAllowed(now)
    const choice = noticeChoice(mode, now)
    setRefused(choice.keep ? undefined : choice.why)
    if (choice.keep) keep({ ...settings, mode })
  }
  const system = settings.mode === "on" || settings.mode === "both"
  return (
    <>
      <About>When a session needs you, or evie finishes or fails a turn. This browser remembers them.</About>
      <Group>
        <Row
          id="notifications"
          label="Notifications and sound"
          title="Notifications and sound"
          about={refused ?? "Told by the system while this window is behind others, and by a sound, if you choose one. Nothing is said of the session you are looking at."}
          {...(system && allowed !== "granted" && { standing: PERMISSION[allowed] })}
          {...(settings.mode !== NOTICE_DEFAULTS.mode && { reset: { label: `Reset notifications to ${MODE_NAMES[NOTICE_DEFAULTS.mode]}`, reset: () => void choose(NOTICE_DEFAULTS.mode) } })}
          control={<Choice label="Notifications" value={settings.mode} choices={(Object.keys(MODE_NAMES) as NoticeMode[]).map((mode) => ({ value: mode, name: MODE_NAMES[mode] }))} choose={(mode) => void choose(mode)} />}
        />
        <Row
          id="notices"
          label="Notices in the page"
          title="Notices in the page"
          about="A notice of another session, with Open, at the top of the page while this window is in front."
          {...(settings.inApp !== NOTICE_DEFAULTS.inApp && { reset: { label: "Reset notices in the page to on", reset: () => keep({ ...settings, inApp: NOTICE_DEFAULTS.inApp }) } })}
          control={<Switch aria-label="Notices in the page" checked={settings.inApp} onCheckedChange={(inApp) => keep({ ...settings, inApp })} />}
        />
      </Group>
    </>
  )
}

// Each choice of notifications, by T3 Code's names for them.
const MODE_NAMES: Record<NoticeMode, string> = { off: "Off", on: "Notifications", sound: "Sound only", both: "Notifications with sound" }

// Whether this browser lets the page show the system's notifications: it
// can, it has not been asked, it refuses, or it has none for this page, as
// at an address that is not secure, such as another computer's over plain http.
type Allowed = NotificationPermission | "none"
const permission = (): Allowed => (typeof Notification === "function" && isSecureContext ? Notification.permission : "none")
const PERMISSION: Record<Exclude<Allowed, "granted">, Standing> = {
  default: { word: "Not allowed yet", note: "Choose again for the browser to ask" },
  denied: { word: "Blocked by the browser", note: "Allow notifications for this page in the browser's settings" },
  none: { word: "Not in this browser", note: "It shows them only for a page at a secure address, such as this computer's own" },
}

// ---- The pages kept on the server -----------------------------------------------------------------

function ServerPage({ page, settings }: { page: Exclude<Page, "appearance" | "notifications">; settings: ServerSettings }) {
  const { view, problem, draft } = useSyncExternalStore(settings.subscribe, settings.state)
  if (view === undefined) {
    return (
      <p role="status" className="flex items-center gap-2 px-3 pt-1 text-caption text-text-muted wide:px-4">
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
  if (page === "keys") return <Keys view={view} change={change} />
  if (page === "plugins") return <Plugins view={view} change={change} />
  if (page === "instructions") return <Instructions view={view} draft={draft} settings={settings} />
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
      <About>
        <p>
          Kept in {env} in evie's data folder,{" "}
          {view.keysPrivate ? "which only you can read." : "which others on this computer can read too. Saving a key here makes it readable by you only."} A key is
          never shown once it is set, here or anywhere else.
        </p>
        <OnlyThere what="Keys" />
      </About>
      <Group>
        {view.keys.map((key) => (
          <KeyRow key={key.name} item={key} change={change} />
        ))}
      </Group>
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
    <div>
      <Row
        id={`key:${item.name}`}
        label={item.name}
        title={<code className="font-mono text-code font-normal">{item.name}</code>}
        {...(item.about !== undefined && { about: item.about })}
        standing={keyState(item)}
        {...(here && {
          control: (
            <div className="flex w-full flex-wrap items-center gap-2 @min-[512px]:w-auto">
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
                className="min-w-0 flex-1 font-mono [-webkit-text-security:disc] @min-[512px]:w-settings-field @min-[512px]:flex-none"
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
    </div>
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
      <About>
        <p>
          What evie's server loads, in order, from {json} in its data folder. A plugin runs with your full permissions, so add only one you trust. A plugin file
          added here can be taken off here; the built-in ones are changed in {json}.
        </p>
        <OnlyThere what="Plugins" />
      </About>
      <Group>
      {view.plugins.map((plugin) => (
        <div key={plugin.name}>
          <Row
            id={`plugin:${plugin.name}`}
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
        <div>
          <Row
            id="add-plugin"
            label="Add a plugin file"
            title="Add a plugin file"
            about="The full path to a plugin file. evie checks, in a process of its own, that its server would start with it."
            control={
              <div className="flex w-full items-center gap-2 @min-[512px]:w-auto">
                <Input
                  aria-label="A plugin file's path"
                  placeholder="/path/to/plugin.ts"
                  spellCheck={false}
                  value={adding}
                  onChange={(event) => setAdding(event.target.value)}
                  onKeyDown={(event: KeyboardEvent) => event.key === "Enter" && void add()}
                  className="min-w-0 flex-1 font-mono @min-[512px]:w-settings-field @min-[512px]:flex-none"
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
        </div>
      )}
      </Group>
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
      <About>
        <p>
          How you want evie to work, in every project. evie is told it before every message, from the next one. Kept in{" "}
          <code className="font-mono text-code">AGENTS.md</code> in evie's data folder.
        </p>
      </About>
      <div data-setting="instructions" tabIndex={-1} className="flex flex-col gap-3 px-3 pt-2 outline-none wide:px-4">
        <textarea
          aria-label="Your instructions"
          value={draft ?? view.instructions}
          onChange={(event) => {
            settings.editInstructions(event.target.value)
            setSaved(false)
          }}
          rows={12}
          className="w-full resize-y rounded-md border border-line bg-transparent p-3 text-field text-text placeholder:text-text-muted"
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
      <About>
        <p>
          A project's own settings file, <code className="font-mono text-code">.evie/evie.json</code>, can load plugins, which run with your full permissions,
          and switch protections off. It is used only while the project is trusted.
        </p>
        <OnlyThere what="Trusted projects" />
      </About>
      {view.projects.length === 0 && <p className="px-3 text-caption text-text-muted wide:px-4">No projects yet.</p>}
      <Group>
      {view.projects.map((project) => (
        <div key={project.id}>
          <Row
            id={`project:${project.id}`}
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
      </Group>
    </>
  )
}

// ---- Restarting ----------------------------------------------------------------------------------

// Along the page's foot while a change waits for the server to start
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
  // The status region is always on the page, and only what is in it comes
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
