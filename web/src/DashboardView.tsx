import { useCallback, useEffect, useState } from "react"
import type { DashboardEntry, DashboardOpenResult, ListedProject } from "../../src/core/types.ts"
import { Button } from "./components/ui/button.tsx"
import type { Connection } from "./connection.ts"
import { dashboardPreviewMintUrl, openDashboard, startDashboardHeartbeat } from "./dashboards.ts"
import { ArrowClockwise } from "./icons.tsx"

// One workspace dashboard tab: either a static HTML file served from a scoped
// `/preview` pass, a live local web app proxied through `/dashboard/<pass>/`,
// or an invalid entry shown with why it could not be opened.
//
// Both static and live iframes omit `allow-same-origin` so the framed page
// runs in an opaque origin and can never read evie's sign-in cookie or touch
// the parent page.
const IFRAME_SANDBOX = "allow-scripts allow-forms allow-downloads allow-popups"

type StaticState =
  | { status: "loading" }
  | { status: "ready"; url: string }
  | { status: "failed"; message: string }

type LiveState =
  | { status: "starting" }
  | { status: "ready"; url: string; passId?: string }
  | { status: "not_running" | "failed"; message: string; log?: string }
  | { status: "invalid"; error: string }

export function DashboardView({ connection, project, dashboard }: { connection: Connection; project: ListedProject; dashboard: DashboardEntry }) {
  if (dashboard.kind === "invalid") {
    return <InvalidDashboard name={dashboard.name} error={dashboard.error} />
  }
  if (dashboard.kind === "static") {
    return <StaticDashboard key={`${project.id}:${dashboard.id}:${dashboard.path}`} connection={connection} project={project} dashboard={dashboard} />
  }
  return <LiveDashboard key={`${project.id}:${dashboard.id}:${dashboard.url}`} connection={connection} project={project} dashboard={dashboard} />
}

// Slim utility bar above the iframe: quiet status indicator on the left,
// Reload action on the right, leaving the rest of the workspace column to the
// dashboard itself.
function DashboardBar({ tone, status, onReload }: { tone: "ready" | "starting" | "error" | "muted"; status: string; onReload: () => void }) {
  const dot =
    tone === "ready"
      ? "bg-status-done"
      : tone === "starting"
        ? "bg-project-mustard"
        : tone === "error"
          ? "bg-status-failed"
          : "bg-text-muted"
  return (
    <div className="flex h-row-height shrink-0 items-center justify-between gap-2 border-b border-line px-gutter">
      <span className="flex min-w-0 items-center gap-2 text-caption text-text-muted">
        <span aria-hidden="true" className={`size-1.5 shrink-0 rounded-full ${dot}`} />
        <span className="truncate">{status}</span>
      </span>
      <button
        type="button"
        onClick={onReload}
        aria-label="Reload dashboard"
        title="Reload"
        className="flex h-control-xs shrink-0 items-center gap-1 rounded-md px-2 text-caption text-text-muted hover:bg-text/8 hover:text-text"
      >
        <ArrowClockwise className="size-icon-xs" />
        Reload
      </button>
    </div>
  )
}

function InvalidDashboard({ name, error }: { name: string; error: string }) {
  return (
    <div className="flex flex-1 flex-col items-center justify-center px-gutter py-8">
      <div className="flex w-full max-w-dialog flex-col gap-2 rounded-md border border-line bg-surface p-5">
        <h2 className="text-title text-text">This dashboard could not be opened</h2>
        <p className="text-body break-words text-text-muted">{error}</p>
        <p className="text-caption text-text-muted">Check &quot;{name}&quot; under &quot;dashboards&quot; in `.evie/evie.json`.</p>
      </div>
    </div>
  )
}

function StaticDashboard({
  connection,
  project,
  dashboard,
}: {
  connection: Connection
  project: ListedProject
  dashboard: Extract<DashboardEntry, { kind: "static" }>
}) {
  const [tick, setTick] = useState(0)
  const [state, setState] = useState<StaticState>({ status: "loading" })
  const reload = useCallback(() => setTick((n) => n + 1), [])

  useEffect(() => {
    let gone = false
    setState({ status: "loading" })

    async function load() {
      const opened = await openDashboard(connection, project.id, dashboard.id)
      if (gone) return
      if (opened === undefined) {
        setState({ status: "failed", message: "Not connected to evie's server." })
        return
      }
      if (opened.kind === "invalid") {
        setState({ status: "failed", message: opened.error })
        return
      }
      if (opened.kind !== "static") {
        setState({ status: "failed", message: "This dashboard is no longer a static file. Reload the page to refresh tabs." })
        return
      }
      try {
        const answer = await fetch(dashboardPreviewMintUrl(project.id, opened.path), { method: "POST" })
        if (gone) return
        if (!answer.ok) {
          setState({ status: "failed", message: `Could not open ${opened.path} (server answered ${answer.status}).` })
          return
        }
        const { url } = (await answer.json()) as { url: string }
        if (!gone) setState({ status: "ready", url })
      } catch (cause) {
        if (!gone) setState({ status: "failed", message: wordsOf(cause) })
      }
    }

    void load()
    return () => {
      gone = true
    }
  }, [connection, project.id, dashboard.id, tick])

  return (
    <div className="flex min-h-0 flex-1 flex-col">
      <DashboardBar
        tone={state.status === "ready" ? "muted" : state.status === "loading" ? "starting" : "error"}
        status={state.status === "loading" ? `Opening ${dashboard.name}…` : state.status === "failed" ? "Static · unavailable" : "Static"}
        onReload={reload}
      />
      {state.status === "loading" ? (
        <div className="flex flex-1 items-center justify-center px-gutter py-8">
          <p className="text-caption text-text-muted">Opening {dashboard.name}…</p>
        </div>
      ) : state.status === "failed" ? (
        <div className="flex flex-1 flex-col items-center justify-center px-gutter py-8">
          <div className="flex w-full max-w-dialog flex-col gap-3 rounded-md border border-line bg-surface p-5">
            <h2 className="text-title text-text">{dashboard.name} could not be opened</h2>
            <p className="text-body break-words text-text-muted">{state.message}</p>
            <div>
              <Button variant="secondary" onClick={reload}>
                Retry
              </Button>
            </div>
          </div>
        </div>
      ) : (
        <iframe
          key={`${state.url}#${tick}`}
          src={state.url}
          title={dashboard.name}
          sandbox={IFRAME_SANDBOX}
          className="min-h-0 w-full flex-1 border-0 bg-bg"
        />
      )}
    </div>
  )
}

function LiveDashboard({
  connection,
  project,
  dashboard,
}: {
  connection: Connection
  project: ListedProject
  dashboard: Extract<DashboardEntry, { kind: "live" }>
}) {
  const [openTick, setOpenTick] = useState(0)
  const [reloadKey, setReloadKey] = useState(0)
  const [state, setState] = useState<LiveState>({ status: "starting" })

  const reopen = useCallback(() => setOpenTick((n) => n + 1), [])
  const handleReload = useCallback(() => {
    if (state.status === "ready") setReloadKey((n) => n + 1)
    else setOpenTick((n) => n + 1)
  }, [state.status])

  useEffect(() => {
    let gone = false
    setState({ status: "starting" })

    async function load() {
      let opened: DashboardOpenResult | undefined
      try {
        opened = await openDashboard(connection, project.id, dashboard.id)
      } catch (cause) {
        if (!gone) setState({ status: "failed", message: wordsOf(cause) })
        return
      }
      if (gone) return
      if (opened === undefined) {
        setState({ status: "failed", message: "Not connected to evie's server." })
        return
      }
      if (opened.kind === "invalid") {
        setState({ status: "invalid", error: opened.error })
        return
      }
      if (opened.kind !== "live") {
        setState({ status: "failed", message: "This dashboard is no longer a live app. Reload the page to refresh tabs." })
        return
      }
      if (opened.status === "ready") {
        setState({ status: "ready", url: opened.url, ...(opened.passId !== undefined && { passId: opened.passId }) })
      } else {
        setState({
          status: opened.status,
          message: opened.message,
          ...(opened.log !== undefined && { log: opened.log }),
        })
      }
    }

    void load()
    return () => {
      gone = true
    }
  }, [connection, project.id, dashboard.id, openTick])

  // Keep the live app and its scoped pass alive while this tab is mounted,
  // and check immediately when returning from a background tab or laptop sleep.
  const readyPassId = state.status === "ready" ? state.passId : undefined
  const isReady = state.status === "ready"
  useEffect(() => {
    if (!isReady) return
    const heartbeat = startDashboardHeartbeat(
      connection,
      { project: project.id, id: dashboard.id, ...(readyPassId !== undefined && { passId: readyPassId }) },
      reopen,
    )
    function onVisible() {
      if (document.visibilityState === "visible") heartbeat.touchNow()
    }
    document.addEventListener("visibilitychange", onVisible)
    return () => {
      document.removeEventListener("visibilitychange", onVisible)
      heartbeat.stop()
    }
  }, [connection, project.id, dashboard.id, isReady, readyPassId, reopen])

  if (state.status === "invalid") {
    return <InvalidDashboard name={dashboard.name} error={state.error} />
  }

  return (
    <div className="flex min-h-0 flex-1 flex-col">
      <DashboardBar
        tone={state.status === "ready" ? "ready" : state.status === "starting" ? "starting" : "error"}
        status={state.status === "ready" ? "Live" : state.status === "starting" ? `Starting ${dashboard.name}…` : state.status === "not_running" ? "Not running" : "Could not start"}
        onReload={handleReload}
      />
      {state.status === "starting" ? (
        <div className="flex flex-1 flex-col items-center justify-center gap-2 px-gutter py-8">
          <span aria-hidden="true" className="size-2 rounded-full bg-project-mustard" />
          <p className="text-caption text-text-muted">Starting {dashboard.name}…</p>
        </div>
      ) : state.status === "ready" ? (
        <iframe
          key={`${state.url}#${reloadKey}`}
          src={state.url}
          title={dashboard.name}
          sandbox={IFRAME_SANDBOX}
          className="min-h-0 w-full flex-1 border-0 bg-bg"
        />
      ) : (
        <div className="flex flex-1 flex-col items-center justify-center px-gutter py-8">
          <div className="flex w-full max-w-dialog flex-col gap-3 rounded-md border border-line bg-surface p-5">
            <h2 className="text-title text-text">
              {state.status === "not_running" ? `${dashboard.name} is not running` : `${dashboard.name} could not start`}
            </h2>
            <p className="text-body break-words text-text-muted">{state.message}</p>
            {state.log !== undefined && state.log !== "" && (
              <pre className="max-h-job-output overflow-auto rounded-sm border border-line bg-bg p-3 font-mono text-2xs whitespace-pre-wrap text-text-muted">
                {state.log}
              </pre>
            )}
            <div>
              <Button variant="secondary" onClick={reopen}>
                Try again
              </Button>
            </div>
          </div>
        </div>
      )}
    </div>
  )
}

const wordsOf = (cause: unknown) => (cause instanceof Error ? cause.message : String(cause))
