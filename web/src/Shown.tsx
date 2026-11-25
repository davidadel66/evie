import * as DialogPrimitive from "@radix-ui/react-dialog"
import { lazy, Suspense, useContext, useEffect, useId, useImperativeHandle, useLayoutEffect, useMemo, useRef, useState, useSyncExternalStore, type ComponentType, type KeyboardEvent, type ReactNode, type Ref } from "react"
import { flushSync } from "react-dom"
import { ArrowClockwise, ArrowCounterClockwise, Back, CaretDown, DeviceDesktop, DevicePhone, Download, FullScreen, InStorage, KindHtml, KindImage, KindMarkdown, KindMermaid, KindModel, KindPdf, KindVector, Next, Previous } from "./icons.tsx"
import { cn } from "./lib/utils.ts"
import { renderMarkdown } from "./markdown.ts"
import { isImage, placeIn, previewMintOf, ShownGallery, stepFrom, viewerOf, type Shown, type Viewer } from "./shown.ts"
import type { ModelDimensions } from "./ShownModel.tsx"
import { addressOfWorkspace } from "./workspace.ts"

// Mermaid and three.js live in separate chunks fetched only on first use of a
// Mermaid diagram or a 3D model, so neither grows the page's initial load.
const LazyMermaid = lazy(() => import("./ShownMermaid.tsx").then((m) => ({ default: m.MermaidView })))
const LazyModel = lazy(() => import("./ShownModel.tsx").then((m) => ({ default: m.ModelView })))

// A file evie showed, drawn under its reply in a frame like the
// changed-files card: a 36px header with the kind's icon, the file's name
// and the folder it is kept in, then full screen, open in Storage and
// download; under it, the file, drawn by the viewer of its kind. Storage opens a file in the same frame. Full
// screen moves through every image shown in the session, with a count and
// zoom, after T3 Code's expanded image view (MIT licence, notice in
// LICENSES/t3code-MIT.txt).

// What a viewer is given: the file, the address it is fetched from, and a
// way to open it full screen.
type ViewerProps = { file: Shown; address: string; open(): void }

// Each kind's viewer, picked by the file's type (shown.ts): its icon, the
// verb its full-screen button says, how it is drawn in its frame, and how
// full screen draws it, when not as an image to zoom.
type Kind = { icon: ComponentType<{ className?: string }>; verb: string; Frame: ComponentType<ViewerProps>; Full?: ComponentType<{ file: Shown; address: string }> }
const KINDS: Record<Viewer, Kind> = {
  image: { icon: KindImage, verb: "View", Frame: ImageFrame },
  svg: { icon: KindVector, verb: "View", Frame: SvgFrame },
  pdf: { icon: KindPdf, verb: "View", Frame: PdfFrame, Full: PdfFull },
  markdown: { icon: KindMarkdown, verb: "Read", Frame: MarkdownFrame, Full: MarkdownFull },
  html: { icon: KindHtml, verb: "View", Frame: HtmlFrame, Full: HtmlFull },
  mermaid: { icon: KindMermaid, verb: "View", Frame: MermaidFrame, Full: MermaidFull },
  model: { icon: KindModel, verb: "Inspect", Frame: ModelFrame, Full: ModelFull },
}

// In Storage already, a file has no Open in Storage.
export function ShownFrame({ file: given, inStorage = false }: { file: Shown; inStorage?: boolean }) {
  const { storage: _, ...outside } = given
  const file: Shown = inStorage ? outside : given
  const gallery = useContext(ShownGallery)
  const [full, setFull] = useState(false)
  const viewer = viewerOf(file.type)
  const kind = viewer === undefined ? undefined : KINDS[viewer]
  const Icon = kind?.icon ?? KindImage
  const open = () => setFull(true)
  return (
    <figure aria-label={`Shown by evie: ${file.name}`} className="mt-1 rounded-md bg-text/4">
      <figcaption className="flex h-shown-header min-w-0 items-center gap-2 pr-1 pl-3 text-caption">
        <Icon className="size-icon-row shrink-0 text-text-muted" />
        {/* The name stays whole while it fits, and the folder gives way first. */}
        <span className="flex min-w-0 flex-1 items-center gap-2">
          <span className="max-w-full shrink-0 truncate font-medium text-text">{file.name}</span>
          <span title={file.path} className="min-w-0 truncate font-mono text-text-muted">
            {file.folder}
          </span>
        </span>
        {kind !== undefined && file.address !== undefined && (
          <FrameButton label={`${kind.verb} ${file.name} full screen`} title="Full screen" onClick={open}>
            <FullScreen className="size-icon-row" />
          </FrameButton>
        )}
        <FileActions file={file} look="frame" />
      </figcaption>
      <div className="mx-1 pb-1">{kind !== undefined && file.address !== undefined ? <kind.Frame file={file} address={file.address} open={open} /> : <Unavailable />}</div>
      {full && <FullScreenView file={file} gallery={inStorage ? [] : gallery} close={() => setFull(false)} />}
    </figure>
  )
}

const FRAME_BUTTON = "flex size-control-sm shrink-0 items-center justify-center rounded-md text-text-muted hover:bg-text/8 hover:text-text"
const FULL_BUTTON = "flex size-control shrink-0 items-center justify-center rounded-full bg-[rgba(255,255,255,0.1)] text-[#fff] hover:bg-[rgba(255,255,255,0.18)]"

function FrameButton({ label, title, onClick, children }: { label: string; title: string; onClick(): void; children: ReactNode }) {
  return (
    <button type="button" aria-label={label} title={title} onClick={onClick} className={FRAME_BUTTON}>
      {children}
    </button>
  )
}

// Open in Storage, for a file in a project's Storage, and Download, in the
// frame's header or over full screen.
function FileActions({ file, look, onLeave }: { file: Shown; look: "frame" | "full"; onLeave?: () => void }) {
  const style = look === "frame" ? FRAME_BUTTON : FULL_BUTTON
  return (
    <>
      {file.storage !== undefined && (
        <a href={addressOfWorkspace({ workspace: file.storage.project, tab: "storage", file: file.storage.path })} onClick={onLeave} aria-label={`Open ${file.name} in Storage`} title="Open in Storage" className={style}>
          <InStorage className="size-icon-row" />
        </a>
      )}
      {file.address !== undefined && (
        <a href={file.address} download={file.name} aria-label={`Download ${file.name}`} title="Download" className={style}>
          <Download className="size-icon-row" />
        </a>
      )}
    </>
  )
}

// A file that cannot be drawn, in quiet words.
function Unavailable({ dark = false }: { dark?: boolean }) {
  return (
    <p className={cn("flex min-h-control items-center justify-center rounded-chip px-3 py-2 text-center text-caption", dark ? "text-[rgba(255,255,255,0.8)]" : "text-text-muted")}>
      This file could not be shown. It may have been moved or deleted.
    </p>
  )
}

// A picture, fitted to the column, at most 480px tall, and never drawn
// larger than it is. It opens full screen.
function ImageFrame({ file, address, open }: ViewerProps) {
  const [failed, setFailed] = useState(false)
  if (failed) return <Unavailable />
  return (
    <button type="button" tabIndex={-1} aria-label={`View ${file.name} full screen`} onClick={open} className="block w-full cursor-zoom-in overflow-hidden rounded-chip">
      <img src={address} alt={file.name} onError={() => setFailed(true)} className="mx-auto block max-h-shown max-w-full" />
    </button>
  )
}

// An SVG, drawn as an image, so that no script in it runs, on a
// checkerboard, so that its own transparency shows, at its own width: the
// width it names, or with none, 300px, as a browser draws one. It opens
// full screen.
function SvgFrame({ file, address, open }: ViewerProps) {
  const [failed, setFailed] = useState(false)
  const [width, setWidth] = useState<number>()
  if (failed) return <Unavailable />
  return (
    <button
      type="button"
      tabIndex={-1}
      aria-label={`View ${file.name} full screen`}
      onClick={open}
      className="checkerboard flex max-h-shown min-h-shown-svg w-full cursor-zoom-in items-center justify-center overflow-hidden rounded-chip p-6"
    >
      <img
        src={address}
        alt={file.name}
        onLoad={(event) => setWidth(event.currentTarget.naturalWidth || undefined)}
        onError={() => setFailed(true)}
        style={width === undefined ? undefined : { width }}
        className="max-h-shown-drawing max-w-[80%] object-contain"
      />
    </button>
  )
}

// A PDF, in the browser's own PDF view, 480px tall, without its list of
// pages, which Chrome would open beside them in a frame this wide.
function PdfFrame({ file, address }: ViewerProps) {
  return <iframe src={`${address}#navpanes=0`} title={`${file.name}, in the browser's PDF view`} className="block h-shown w-full rounded-chip border-0" />
}
function PdfFull({ file, address }: { file: Shown; address: string }) {
  return <iframe src={address} title={`${file.name}, in the browser's PDF view`} className="block h-full-height w-full-width rounded-md border-0 shadow-float" />
}

// A Markdown document, fetched and drawn from its Markdown as a reply is,
// nothing in it as HTML, in a document's own look on the ground; cut at
// 320px with a fade and Show all when it is longer, which opens to the
// whole of it and Show less.
function MarkdownFrame({ file, address }: ViewerProps) {
  const read = useText(address)
  const [open, setOpen] = useState(false)
  const [long, setLong] = useState(false)
  const box = useRef<HTMLDivElement>(null)
  const id = useId()
  const drawn = useMemo(() => (read.text === undefined ? undefined : renderMarkdown(read.text)), [read.text])
  // Long is measured with the cut in place, so that it is known whether to offer the rest.
  useLayoutEffect(() => {
    const element = box.current
    if (element !== null && !open) setLong(element.scrollHeight > element.clientHeight + 1)
  }, [drawn, open])
  if (read.failed) return <Unavailable />
  return (
    <>
      <div className="relative rounded-chip bg-bg">
        <div id={id} ref={box} aria-label={file.name} className={cn("shown-document overflow-hidden px-5 pt-4 pb-1 break-words text-text", !open && "max-h-shown-cut")}>
          {drawn ?? <p className="text-caption text-text-muted">Reading {file.name}</p>}
        </div>
        {!open && long && <div aria-hidden="true" className="pointer-events-none absolute inset-x-0 bottom-0 h-shown-fade rounded-b-chip" style={{ background: "linear-gradient(to bottom, transparent, var(--bg))" }} />}
      </div>
      {long && (
        <div className="flex justify-center pt-1">
          <button
            type="button"
            aria-expanded={open}
            aria-controls={id}
            onClick={() => setOpen(!open)}
            className="flex h-control-sm items-center gap-1.5 rounded-md px-2.5 text-caption font-medium text-text-muted hover:bg-text/8 hover:text-text"
          >
            {open ? "Show less" : "Show all"}
            <CaretDown className={cn("size-icon-xs transition-transform motion-reduce:transition-none", open && "rotate-180")} />
          </button>
        </div>
      )}
    </>
  )
}
function MarkdownFull({ file, address }: { file: Shown; address: string }) {
  const read = useText(address)
  const drawn = useMemo(() => (read.text === undefined ? undefined : renderMarkdown(read.text)), [read.text])
  if (read.failed) return <Unavailable dark />
  return (
    <div aria-label={file.name} tabIndex={0} className="shown-document h-full-height w-full-width max-w-conversation overflow-y-auto overscroll-contain rounded-md bg-bg px-6 py-5 break-words text-text shadow-float">
      {drawn}
    </div>
  )
}

// ---- Live viewers: HTML mockups, Mermaid diagrams, 3D models --------------------

// Mints a scoped, short-lived preview pass for a file so its sandboxed iframe
// or 3D loader can fetch relative companion files without carrying evie's cookie.
export function usePreviewUrl(storageAddress: string): { url?: string; failed?: true; remint(): void } {
  const [tick, setTick] = useState(0)
  const [state, setState] = useState<{ key: string; url?: string; failed?: true }>({ key: `${storageAddress}#${tick}` })
  const key = `${storageAddress}#${tick}`
  useEffect(() => {
    let gone = false
    fetch(previewMintOf(storageAddress), { method: "POST" })
      .then((answer) => (answer.ok ? (answer.json() as Promise<{ url: string }>) : Promise.reject(new Error(String(answer.status)))))
      .then(
        ({ url }) => !gone && setState({ key, url }),
        () => !gone && setState({ key, failed: true }),
      )
    return () => {
      gone = true
    }
  }, [storageAddress, key])
  const current = state.key === key ? state : {}
  return { ...current, remint: () => setTick((n) => n + 1) }
}

// Desktop viewport width that a live HTML mockup scales down from when the
// conversation column is narrower than a desktop screen.
const DESKTOP_MOCKUP_WIDTH = 1280

// An HTML mockup, live in a sealed iframe (`sandbox="allow-scripts allow-forms"`,
// no `allow-same-origin`): scripts run, relative styles/scripts/images load from
// its own scoped preview pass, and it can never touch evie or the owner's sign-in.
// Offers a desktop/phone (390px) width switch, reload, and a quiet note saying it
// cannot reach evie. At most 480px tall, scrolling inside.
function HtmlFrame({ file, address }: ViewerProps) {
  const preview = usePreviewUrl(address)
  const [mode, setMode] = useState<"desktop" | "phone">("desktop")
  const [reloadKey, setReloadKey] = useState(0)
  const stage = useRef<HTMLDivElement>(null)
  const [stageWidth, setStageWidth] = useState(0)

  useEffect(() => {
    const el = stage.current
    if (!el) return
    const measure = () => setStageWidth(el.clientWidth)
    measure()
    const observer = new ResizeObserver(measure)
    observer.observe(el)
    return () => observer.disconnect()
  }, [])

  if (preview.failed) return <Unavailable />
  const scale = mode === "desktop" && stageWidth > 0 && stageWidth < DESKTOP_MOCKUP_WIDTH ? stageWidth / DESKTOP_MOCKUP_WIDTH : 1
  const scaledHeight = Math.round(480 / scale)

  return (
    <div className="flex flex-col gap-1">
      <div className="flex items-center justify-between gap-2 px-2 py-1 text-caption text-text-muted">
        <div role="group" aria-label="Mockup width" className="flex rounded-md bg-text/6 p-0.5">
          <button
            type="button"
            aria-pressed={mode === "desktop"}
            onClick={() => setMode("desktop")}
            className={cn("flex h-control-xs items-center gap-1 rounded-sm px-2 text-caption", mode === "desktop" ? "bg-surface-raised text-text shadow-toggle" : "text-text-muted hover:text-text")}
          >
            <DeviceDesktop className="size-icon-xs" />
            Desktop
          </button>
          <button
            type="button"
            aria-pressed={mode === "phone"}
            onClick={() => setMode("phone")}
            className={cn("flex h-control-xs items-center gap-1 rounded-sm px-2 text-caption", mode === "phone" ? "bg-surface-raised text-text shadow-toggle" : "text-text-muted hover:text-text")}
          >
            <DevicePhone className="size-icon-xs" />
            Phone
          </button>
        </div>
        <button
          type="button"
          onClick={() => setReloadKey((n) => n + 1)}
          aria-label={`Reload ${file.name}`}
          title="Reload"
          className="flex h-control-xs items-center gap-1 rounded-md px-2 text-caption text-text-muted hover:bg-text/8 hover:text-text"
        >
          <ArrowClockwise className="size-icon-xs" />
          Reload
        </button>
      </div>
      <div ref={stage} className="relative flex h-shown w-full justify-center overflow-hidden rounded-chip bg-bg">
        {preview.url === undefined ? (
          <p className="m-auto text-caption text-text-muted">Opening {file.name}…</p>
        ) : mode === "phone" ? (
          <iframe
            key={`${preview.url}#${reloadKey}`}
            src={preview.url}
            title={`${file.name}, live HTML mockup`}
            sandbox="allow-scripts allow-forms"
            className="h-full w-shown-phone max-w-full border-x border-line bg-white"
          />
        ) : (
          <div className="h-full w-full overflow-hidden">
            <iframe
              key={`${preview.url}#${reloadKey}`}
              src={preview.url}
              title={`${file.name}, live HTML mockup`}
              sandbox="allow-scripts allow-forms"
              style={scale < 1 ? { width: DESKTOP_MOCKUP_WIDTH, height: scaledHeight, transform: `scale(${scale})`, transformOrigin: "top left" } : undefined}
              className={cn("border-0 bg-white", scale === 1 && "h-full w-full")}
            />
          </div>
        )}
      </div>
      <p className="px-2 py-0.5 text-2xs text-text-muted">Live preview in a sealed frame — this page cannot reach evie or your sign-in.</p>
    </div>
  )
}

function HtmlFull({ file, address }: { file: Shown; address: string }) {
  const preview = usePreviewUrl(address)
  if (preview.failed) return <Unavailable dark />
  if (preview.url === undefined) return <p className="text-caption text-[rgba(255,255,255,0.8)]">Opening {file.name}…</p>
  return (
    <iframe
      src={preview.url}
      title={`${file.name}, live HTML mockup`}
      sandbox="allow-scripts allow-forms"
      className="block h-full-height w-full-width rounded-md border-0 bg-white shadow-float"
    />
  )
}

// A Mermaid diagram, drawn in Dusk colours on first use, with a "Show source"
// toggle and natural height up to 480px.
function MermaidFrame({ file, address }: ViewerProps) {
  const read = useText(address)
  const [showSource, setShowSource] = useState(false)
  if (read.failed) return <Unavailable />
  if (read.text === undefined) {
    return <p className="flex min-h-control items-center justify-center rounded-chip bg-bg px-3 py-4 text-caption text-text-muted">Reading {file.name}…</p>
  }
  return (
    <div className="flex flex-col gap-1">
      {showSource ? (
        <pre className="max-h-shown overflow-auto rounded-chip bg-bg p-4 font-mono text-code text-text">{read.text}</pre>
      ) : (
        <Suspense fallback={<p className="flex min-h-control items-center justify-center rounded-chip bg-bg px-3 py-4 text-caption text-text-muted">Loading Mermaid…</p>}>
          <LazyMermaid source={read.text} />
        </Suspense>
      )}
      <div className="flex justify-end px-2 py-0.5">
        <button
          type="button"
          aria-pressed={showSource}
          onClick={() => setShowSource((v) => !v)}
          className="flex h-control-xs items-center rounded-md px-2 text-caption text-text-muted hover:bg-text/8 hover:text-text"
        >
          {showSource ? "Show diagram" : "Show source"}
        </button>
      </div>
    </div>
  )
}

function MermaidFull({ file, address }: { file: Shown; address: string }) {
  const read = useText(address)
  if (read.failed) return <Unavailable dark />
  if (read.text === undefined) return <p className="text-caption text-[rgba(255,255,255,0.8)]">Reading {file.name}…</p>
  return (
    <div className="h-full-height w-full-width overflow-auto rounded-md bg-bg p-6 shadow-float">
      <Suspense fallback={<p className="text-caption text-text-muted">Loading Mermaid…</p>}>
        <LazyMermaid source={read.text} full />
      </Suspense>
    </div>
  )
}

// A 3D model (glTF, GLB, STL or OBJ), drawn with three.js loaded on first use.
// 420px tall in the conversation, with Reset view and its bounding dimensions.
function ModelFrame({ file, address }: ViewerProps) {
  const preview = usePreviewUrl(address)
  const [resetTick, setResetTick] = useState(0)
  const [dims, setDims] = useState<ModelDimensions>()
  if (preview.failed) return <Unavailable />
  return (
    <div className="flex flex-col gap-1">
      {preview.url === undefined ? (
        <div className="flex h-shown-model items-center justify-center rounded-chip bg-bg text-caption text-text-muted">Opening {file.name}…</div>
      ) : (
        <Suspense fallback={<div className="flex h-shown-model items-center justify-center rounded-chip bg-bg text-caption text-text-muted">Loading 3D viewer…</div>}>
          <LazyModel url={preview.url} type={file.type} resetTick={resetTick} onDimensions={setDims} />
        </Suspense>
      )}
      <div className="flex items-center justify-between gap-2 px-2 py-0.5 text-caption text-text-muted">
        <span className="font-mono text-2xs">{dims ? `Size: ${formatModelSize(dims)}` : file.type.replace("model/", "").toUpperCase()}</span>
        <button
          type="button"
          onClick={() => setResetTick((n) => n + 1)}
          className="flex h-control-xs items-center gap-1 rounded-md px-2 text-caption text-text-muted hover:bg-text/8 hover:text-text"
        >
          <ArrowCounterClockwise className="size-icon-xs" />
          Reset view
        </button>
      </div>
    </div>
  )
}

function ModelFull({ file, address }: { file: Shown; address: string }) {
  const preview = usePreviewUrl(address)
  const [resetTick, setResetTick] = useState(0)
  if (preview.failed) return <Unavailable dark />
  if (preview.url === undefined) return <p className="text-caption text-[rgba(255,255,255,0.8)]">Opening {file.name}…</p>
  return (
    <div className="relative">
      <Suspense fallback={<div className="flex h-full-height w-full-width items-center justify-center rounded-md bg-bg text-caption text-text-muted">Loading 3D viewer…</div>}>
        <LazyModel url={preview.url} type={file.type} full resetTick={resetTick} />
      </Suspense>
      <button
        type="button"
        onClick={() => setResetTick((n) => n + 1)}
        className={cn(FULL_BUTTON, "absolute right-4 bottom-4 w-auto gap-1.5 px-3 text-detail font-medium")}
      >
        <ArrowCounterClockwise className="size-icon-xs" />
        Reset view
      </button>
    </div>
  )
}

function formatModelSize(dims: ModelDimensions): string {
  const fmt = (n: number) => (n >= 100 ? n.toFixed(0) : n >= 10 ? n.toFixed(1) : n.toFixed(2)).replace(/\.0+$/, "")
  return `${fmt(dims.x)} × ${fmt(dims.y)} × ${fmt(dims.z)}`
}

// A text file fetched from its address, signed in as the page is, with
// its cookie: its text once read, or that it could not be.
function useText(address: string): { text?: string; failed?: true } {
  const [read, setRead] = useState<{ address: string; text?: string; failed?: true }>({ address })
  useEffect(() => {
    let gone = false
    fetch(address)
      .then((answer) => (answer.ok ? answer.text() : Promise.reject(new Error(String(answer.status)))))
      .then(
        (text) => !gone && setRead({ address, text }),
        () => !gone && setRead({ address, failed: true }),
      )
    return () => {
      gone = true
    }
  }, [address])
  return read.address === address ? read : {}
}

// ---- Full screen ----------------------------------------------------------------

// A file full screen, over a dark veil: Back at the top left;
// Open in Storage and Download at the top right; and under the file its
// name. An image is one of the session's, which previous and next move
// through, with its place among them, and it zooms: a click zooms in where
// it was clicked and back to fit, and while zoomed in, dragging or
// scrolling moves around it. Escape, Back, or a click on the veil closes it.
function FullScreenView({ file, gallery, close }: { file: Shown; gallery: readonly Shown[]; close(): void }) {
  // The image shown is held by its key, so that it stays shown as the
  // gallery grows, as when the page before is read while it is open.
  const [key, setKey] = useState(file.key)
  const images = isImage(file) && gallery.some((image) => image.key === file.key) ? gallery : [file]
  const current = images.find((image) => image.key === key) ?? file
  const place = placeIn(images, current.key)
  const [zoomed, setZoomed] = useState(false)
  const zoomable = useRef<ZoomHandle>(null)
  const viewer = viewerOf(current.type)
  const Full = viewer === undefined ? undefined : KINDS[viewer].Full
  const many = place.count > 1
  const move = (by: number) => {
    setKey(stepFrom(images, current.key, by))
    setZoomed(false)
  }
  function onKeyDown(event: KeyboardEvent<HTMLDivElement>) {
    if (event.defaultPrevented) return
    if (zoomable.current?.pan(event.key)) return event.preventDefault()
    if (!many || (event.key !== "ArrowLeft" && event.key !== "ArrowRight")) return
    event.preventDefault()
    move(event.key === "ArrowLeft" ? -1 : 1)
  }
  return (
    <DialogPrimitive.Root open onOpenChange={(open) => !open && close()}>
      <DialogPrimitive.Portal>
        <DialogPrimitive.Overlay className="fixed inset-0 z-50 bg-[rgba(0,0,0,0.86)]" />
        <DialogPrimitive.Content
          aria-describedby={undefined}
          onKeyDown={onKeyDown}
          onClick={(event) => event.target === event.currentTarget && close()}
          className="fixed inset-0 z-50 flex items-center justify-center font-sans text-[#fff] outline-none"
        >
          <DialogPrimitive.Title className="sr-only">{isImage(current) ? "Expanded image preview" : `${current.name}, full screen`}</DialogPrimitive.Title>
          <button type="button" onClick={close} aria-label={isImage(current) ? "Back, close image preview" : "Back, close full screen"} className={cn(FULL_BUTTON, "absolute top-4 left-4 z-10 w-auto gap-1.5 pr-3 pl-2.5 text-detail font-medium")}>
            <Back className="size-icon-row" />
            Back
          </button>
          <div className="absolute top-4 right-4 z-10 flex gap-1">
            <FileActions file={current} look="full" onLeave={close} />
          </div>
          {many && (
            <>
              <button type="button" aria-label="Previous image" onClick={() => move(-1)} className={cn(FULL_BUTTON, "absolute top-1/2 left-4 z-10 size-full-nav -translate-y-1/2")}>
                <Previous className="size-4.5" />
              </button>
              <button type="button" aria-label="Next image" onClick={() => move(1)} className={cn(FULL_BUTTON, "absolute top-1/2 right-4 z-10 size-full-nav -translate-y-1/2")}>
                <Next className="size-4.5" />
              </button>
            </>
          )}
          <div className="relative flex min-w-0 flex-col items-center">
            {current.address === undefined ? (
              <Unavailable dark />
            ) : Full !== undefined ? (
              <Full file={current} address={current.address} />
            ) : (
              <ZoomableImage key={current.key} ref={zoomable} file={current} address={current.address} onZoom={setZoomed} />
            )}
            <p className="mt-2 flex max-w-full-width items-center gap-1.5 text-caption text-[rgba(255,255,255,0.8)]">
              <span aria-live="polite" aria-atomic="true" className="truncate">
                {current.name}
                {many ? ` (${place.at}/${place.count})` : ""}
              </span>
              {isImage(current) && (
                <>
                  <span aria-hidden="true" className="text-[rgba(255,255,255,0.55)]">
                    ·
                  </span>
                  <span className="shrink-0 text-[rgba(255,255,255,0.7)]">{zoomed ? "Drag or scroll to move around. Click to fit." : "Click to zoom"}</span>
                </>
              )}
            </p>
          </div>
        </DialogPrimitive.Content>
      </DialogPrimitive.Portal>
    </DialogPrimitive.Root>
  )
}

// ---- Zoom -----------------------------------------------------------------------

// The most an image is zoomed in, as T3 Code's.
const MOST_ZOOM = 8

type ZoomHandle = { pan(key: string): boolean }

// The window's size, the same object while it stays the same size, so
// that only a change of size fits the image again.
let windowSize = { width: 0, height: 0 }
function readWindow() {
  if (windowSize.width !== innerWidth || windowSize.height !== innerHeight) windowSize = { width: innerWidth, height: innerHeight }
  return windowSize
}
function onResize(changed: () => void) {
  addEventListener("resize", changed)
  return () => removeEventListener("resize", changed)
}

// An image full screen, adapted from T3 Code's ZoomableImage: fitted
// within the room full screen has, a picture never larger than it is and
// an SVG to fill it; a click zooms in two times where it was clicked, and
// out again to fit; Ctrl and the wheel, as a trackpad's pinch sends,
// zooms by as much; while zoomed in, dragging and scrolling move around
// it, and so do the arrow keys. Enter or Space zooms too, plus and minus
// zoom by steps, and 0 fits it. An SVG is drawn on a larger checkerboard.
function ZoomableImage({ file, address, onZoom, ref }: { file: Shown; address: string; onZoom(zoomed: boolean): void; ref: Ref<ZoomHandle> }) {
  const viewport = useRef<HTMLDivElement | null>(null)
  const room = useSyncExternalStore(onResize, readWindow)
  const [natural, setNatural] = useState({ width: 0, height: 0 })
  const [failed, setFailed] = useState(false)
  const [zoom, setZoomNow] = useState(1)
  const [sizedFor, setSizedFor] = useState(room)
  if (sizedFor !== room) {
    setSizedFor(room)
    setZoomNow(1)
  }
  const drag = useRef<{ pointer: number; x: number; y: number; left: number; top: number } | undefined>(undefined)
  const dragged = useRef(false)
  const [dragging, setDragging] = useState(false)
  const svg = viewerOf(file.type) === "svg"
  const tallest = Math.max(1, Math.min(room.height * 0.86, room.height - 160))
  const widest = Math.max(1, room.width * 0.92 - 96)
  const fit = Math.min(svg ? Infinity : 1, widest / (natural.width || 1), tallest / (natural.height || 1))
  const width = natural.width * fit * zoom
  const height = natural.height * fit * zoom

  const setZoom = (next: number, point?: { x: number; y: number }) => {
    const box = viewport.current
    const clamped = Math.min(MOST_ZOOM, Math.max(1, next))
    if (!box || clamped === zoom) return
    const bounds = box.getBoundingClientRect()
    const x = point ? point.x - bounds.left : box.clientWidth / 2
    const y = point ? point.y - bounds.top : box.clientHeight / 2
    const anchor = { x: (box.scrollLeft + x) / zoom, y: (box.scrollTop + y) / zoom }
    // Laid out at the new size first, so that the point clicked stays under the pointer.
    flushSync(() => setZoomNow(clamped))
    onZoom(clamped > 1)
    const resized = box.getBoundingClientRect()
    box.scrollLeft = anchor.x * clamped - (bounds.left + x - resized.left)
    box.scrollTop = anchor.y * clamped - (bounds.top + y - resized.top)
  }

  useImperativeHandle(ref, () => ({
    pan(key) {
      const box = viewport.current
      if (!box || zoom <= 1) return false
      // Left and right move through the gallery while there is nothing to move to sideways.
      if ((key === "ArrowLeft" || key === "ArrowRight") && box.scrollWidth <= box.offsetWidth) return false
      const by = { ArrowLeft: [-40, 0], ArrowRight: [40, 0], ArrowUp: [0, -40], ArrowDown: [0, 40] }[key]
      if (by === undefined) return false
      box.scrollLeft += by[0]!
      box.scrollTop += by[1]!
      return true
    },
  }))

  // A plain wheel scrolls; only Ctrl and the wheel zooms, which must stop
  // the browser zooming the page, and React's own wheel listener cannot.
  useEffect(() => {
    const box = viewport.current
    if (!box) return
    const wheel = (event: WheelEvent) => {
      if (!event.ctrlKey || event.deltaY === 0) return
      event.preventDefault()
      setZoom(zoom * Math.exp(-event.deltaY * (event.deltaMode === 1 ? 16 : 1) * 0.01), { x: event.clientX, y: event.clientY })
    }
    box.addEventListener("wheel", wheel, { passive: false })
    return () => box.removeEventListener("wheel", wheel)
  })

  if (failed) return <Unavailable dark />
  const inset = svg ? 0.76 : 1
  return (
    <div
      ref={viewport}
      role="region"
      aria-label={`${file.name}, zoomable image`}
      aria-description="Click to zoom in or back to fit. Scroll or drag to move around when zoomed in."
      tabIndex={0}
      className={cn("max-w-full-width overflow-auto overscroll-contain rounded-md shadow-[0_0_0_1px_rgba(255,255,255,0.12),0_25px_50px_-12px_rgba(0,0,0,0.6)] outline-none focus-visible:outline-2 focus-visible:outline-focus-ring", svg ? "checkerboard-large" : "bg-bg")}
      style={{ width: width || undefined, height: height || undefined, maxHeight: tallest, cursor: zoom > 1 ? (dragging ? "grabbing" : "grab") : "zoom-in" }}
      onClick={(event) => {
        // A drag ends in a click, which must not zoom.
        if (dragged.current || event.detail > 1) return
        setZoom(zoom > 1 ? 1 : 2, { x: event.clientX, y: event.clientY })
      }}
      onKeyDown={(event) => {
        if (event.ctrlKey || event.metaKey || event.altKey) return
        const by: Record<string, () => void> = { Enter: () => setZoom(zoom > 1 ? 1 : 2), " ": () => setZoom(zoom > 1 ? 1 : 2), "+": () => setZoom(zoom * 1.5), "=": () => setZoom(zoom * 1.5), "-": () => setZoom(zoom / 1.5), "0": () => setZoom(1) }
        const action = by[event.key]
        if (action === undefined) return
        event.preventDefault()
        if (!event.repeat) action()
      }}
      onPointerDown={(event) => {
        dragged.current = false
        if (drag.current || event.pointerType !== "mouse" || event.button !== 0 || zoom <= 1) return
        const box = event.currentTarget
        drag.current = { pointer: event.pointerId, x: event.clientX, y: event.clientY, left: box.scrollLeft, top: box.scrollTop }
        box.setPointerCapture(event.pointerId)
        setDragging(true)
      }}
      onPointerMove={(event) => {
        const was = drag.current
        if (was?.pointer !== event.pointerId) return
        if (Math.hypot(event.clientX - was.x, event.clientY - was.y) > 4) dragged.current = true
        event.currentTarget.scrollLeft = was.left - (event.clientX - was.x)
        event.currentTarget.scrollTop = was.top - (event.clientY - was.y)
      }}
      onPointerUp={(event) => {
        if (drag.current?.pointer !== event.pointerId) return
        if (event.currentTarget.hasPointerCapture(event.pointerId)) event.currentTarget.releasePointerCapture(event.pointerId)
        drag.current = undefined
        setDragging(false)
      }}
      onLostPointerCapture={(event) => {
        if (drag.current?.pointer !== event.pointerId) return
        drag.current = undefined
        setDragging(false)
      }}
    >
      <div className="flex items-center justify-center" style={natural.width ? { width, height } : undefined}>
        <img
          src={address}
          alt={file.name}
          draggable={false}
          onLoad={(event) => setNatural({ width: event.currentTarget.naturalWidth || 300, height: event.currentTarget.naturalHeight || 150 })}
          onError={() => setFailed(true)}
          className="block max-w-none select-none"
          style={natural.width ? { width: width * inset, height: height * inset } : { maxWidth: widest, maxHeight: tallest }}
        />
      </div>
      <span className="sr-only" aria-live="polite">{`${Math.round(zoom * 100)}% zoom`}</span>
    </div>
  )
}
