import { useEffect, useState, type ReactNode } from "react"
import type { DashboardEntry, ListedProject } from "../../src/core/types.ts"
import { Button } from "./components/ui/button.tsx"
import { afterOpening, openLive, type LiveShown } from "./dashboards.ts"
import { ArrowClockwise } from "./icons.tsx"
import { addressOf } from "./shown.ts"
import { usePreviewUrl } from "./Shown.tsx"

// One of a workspace's dashboards, as its tab: a static HTML file in
// `.evie/`, through a preview pass, a live app, through the dashboard
// proxy, or an entry that cannot be used, with why. The page runs in a
// sandbox with no `allow-same-origin`, so it has no origin of its own and
// can never read evie's page or the owner's sign-in.
const SANDBOX = "allow-scripts allow-forms allow-downloads allow-popups"
// How often a live dashboard's open tab asks for it again, which keeps its
// app from being stopped as idle and its address working.
const ASK_AGAIN_EVERY = 30_000

export function DashboardView({ project, dashboard }: { project: ListedProject; dashboard: DashboardEntry }) {
  if (dashboard.kind === "invalid") return <Problem title="This dashboard could not be opened" text={dashboard.error} note={`Check "${dashboard.name}" under "dashboards" in .evie/evie.json.`} />
  if (dashboard.kind === "static") return <StaticDashboard key={`${project.id}:${dashboard.id}`} project={project} dashboard={dashboard} />
  return <LiveDashboard key={`${project.id}:${dashboard.id}`} project={project} dashboard={dashboard} />
}

function StaticDashboard({ project, dashboard }: { project: ListedProject; dashboard: Extract<DashboardEntry, { kind: "static" }> }) {
  // Reload asks for its pass again, since one an hour old has stopped working.
  const preview = usePreviewUrl(addressOf({ project: project.id, path: dashboard.path }))
  return (
    <Framed status={preview.failed ? "Could not open" : preview.url === undefined ? "Opening…" : "Static"} tone={preview.failed ? "failed" : "quiet"} reload={preview.remint}>
      {preview.failed ? (
        <Problem title={`${dashboard.name} could not be opened`} text={`${dashboard.path} could not be opened from this workspace's .evie folder.`} retry={preview.remint} />
      ) : (
        preview.url === undefined ? <Waiting text={`Opening ${dashboard.name}…`} /> : <Frame url={preview.url} name={dashboard.name} />
      )}
    </Framed>
  )
}

// A live dashboard opens its app when its tab is shown, and asks for it
// again every 30 seconds while it is, and when the browser tab comes back
// into view. Reload opens it afresh, as Try again does after a failure.
function LiveDashboard({ project, dashboard }: { project: ListedProject; dashboard: Extract<DashboardEntry, { kind: "live" }> }) {
  const [shown, setShown] = useState<LiveShown>({ status: "opening" })
  const [opens, setOpens] = useState(0)
  const reopen = () => setOpens((n) => n + 1)

  useEffect(() => {
    let gone = false
    setShown({ status: "opening" })
    void openLive(project.id, dashboard.id).then((opened) => !gone && setShown(afterOpening({ status: "opening" }, opened)))
    return () => {
      gone = true
    }
  }, [project.id, dashboard.id, opens])

  const ready = shown.status === "ready"
  useEffect(() => {
    if (!ready) return
    let gone = false
    const ask = () => void openLive(project.id, dashboard.id).then((opened) => !gone && setShown((now) => afterOpening(now, opened)))
    const clock = setInterval(ask, ASK_AGAIN_EVERY)
    const backInView = () => document.visibilityState === "visible" && ask()
    document.addEventListener("visibilitychange", backInView)
    return () => {
      gone = true
      clearInterval(clock)
      document.removeEventListener("visibilitychange", backInView)
    }
  }, [project.id, dashboard.id, ready])

  const status = shown.status === "ready" ? "Live" : shown.status === "opening" ? `Starting ${dashboard.name}…` : shown.status === "not_running" ? "Not running" : "Could not start"
  return (
    <Framed status={status} tone={shown.status === "ready" ? "live" : shown.status === "opening" ? "starting" : "failed"} reload={reopen}>
      {shown.status === "ready" ? (
        <Frame key={`${shown.url}#${shown.load}#${opens}`} url={shown.url} name={dashboard.name} />
      ) : shown.status === "opening" ? (
        <Waiting text={`Starting ${dashboard.name}…`} />
      ) : (
        <Problem title={shown.status === "not_running" ? `${dashboard.name} is not running` : `${dashboard.name} could not start`} text={shown.message} log={shown.log} retry={reopen} />
      )}
    </Framed>
  )
}

// A dashboard with the slim bar above it: how it is on the left, and
// Reload on the right, leaving the rest of the column to the dashboard.
function Framed({ status, tone, reload, children }: { status: string; tone: "live" | "starting" | "failed" | "quiet"; reload: () => void; children: ReactNode }) {
  const dot = { live: "bg-status-done", starting: "bg-project-mustard", failed: "bg-status-failed", quiet: "bg-text-muted" }[tone]
  return (
    <div className="flex min-h-0 flex-1 flex-col">
      <div className="flex h-row-height shrink-0 items-center justify-between gap-2 border-b border-line px-gutter">
        <span className="flex min-w-0 items-center gap-2 text-caption text-text-muted">
          <span aria-hidden="true" className={`size-1.5 shrink-0 rounded-full ${dot}`} />
          <span className="truncate">{status}</span>
        </span>
        <button
          type="button"
          onClick={reload}
          title="Reload"
          className="flex h-control-xs shrink-0 items-center gap-1 rounded-md px-2 text-caption text-text-muted hover:bg-text/8 hover:text-text"
        >
          <ArrowClockwise className="size-icon-xs" />
          Reload
        </button>
      </div>
      {children}
    </div>
  )
}

function Waiting({ text }: { text: string }) {
  return <p className="flex flex-1 items-center justify-center px-gutter py-8 text-caption text-text-muted">{text}</p>
}

function Frame({ url, name }: { url: string; name: string }) {
  return <iframe src={url} title={name} sandbox={SANDBOX} className="min-h-0 w-full flex-1 border-0 bg-bg" />
}

// Why a dashboard cannot be shown, with the end of what its app printed,
// and a way to try again when trying again can help.
function Problem({ title, text, note, log, retry }: { title: string; text: string; note?: string; log?: string | undefined; retry?: () => void }) {
  return (
    <div className="flex flex-1 flex-col items-center justify-center px-gutter py-8">
      <div className="flex w-full max-w-dialog flex-col gap-3 rounded-md border border-line bg-surface p-5">
        <h2 className="text-title text-text">{title}</h2>
        <p className="text-body break-words text-text-muted">{text}</p>
        {note !== undefined && <p className="text-caption text-text-muted">{note}</p>}
        {log !== undefined && log !== "" && (
          <pre className="max-h-job-output overflow-auto rounded-sm border border-line bg-bg p-3 font-mono text-2xs whitespace-pre-wrap text-text-muted">{log}</pre>
        )}
        {retry !== undefined && (
          <div>
            <Button variant="secondary" onClick={retry}>
              Try again
            </Button>
          </div>
        )}
      </div>
    </div>
  )
}
