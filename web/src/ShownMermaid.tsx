import { useEffect, useId, useRef, useState } from "react"
import { cn } from "./lib/utils.ts"

// Mermaid diagram viewer, loaded only on first use via dynamic import so
// mermaid never enters the page's initial bundle. Renders in evie's Dusk
// palette with strict security (no HTML scripts in labels).

let initialized = false
async function loadMermaid() {
  const { default: mermaid } = await import("mermaid")
  if (!initialized) {
    mermaid.initialize({
      startOnLoad: false,
      securityLevel: "strict",
      theme: "base",
      fontFamily: '"Geist", system-ui, sans-serif',
      themeVariables: {
        darkMode: true,
        background: "#131312",
        primaryColor: "#1a1a18",
        primaryTextColor: "#e8e4dc",
        primaryBorderColor: "#86965f",
        secondaryColor: "#242421",
        secondaryTextColor: "#e8e4dc",
        secondaryBorderColor: "#2a2a27",
        tertiaryColor: "#0e0e0d",
        tertiaryTextColor: "#e8e4dc",
        tertiaryBorderColor: "#2a2a27",
        lineColor: "#9b968c",
        textColor: "#e8e4dc",
        mainBkg: "#1a1a18",
        nodeBorder: "#86965f",
        clusterBkg: "#0e0e0d",
        clusterBorder: "#2a2a27",
        edgeLabelBackground: "#131312",
        noteBkgColor: "#242421",
        noteTextColor: "#e8e4dc",
        noteBorderColor: "#86965f",
        actorBkg: "#1a1a18",
        actorBorder: "#86965f",
        actorTextColor: "#e8e4dc",
        actorLineColor: "#9b968c",
        signalColor: "#e8e4dc",
        signalTextColor: "#e8e4dc",
        labelBoxBkgColor: "#1a1a18",
        labelBoxBorderColor: "#2a2a27",
        labelTextColor: "#e8e4dc",
        activationBkgColor: "#242421",
        activationBorderColor: "#86965f",
        cScale0: "#7d8f5a",
        cScale1: "#a8954f",
        cScale2: "#66849a",
        cScale3: "#ad6a45",
        cScale4: "#8e6888",
        cScale5: "#a2525a",
      },
    })
    initialized = true
  }
  return mermaid
}

export function MermaidView({ source, full = false }: { source: string; full?: boolean }) {
  const rawId = useId()
  const renderId = `evie-mmd-${rawId.replace(/[^a-zA-Z0-9_-]/g, "")}`
  const host = useRef<HTMLDivElement>(null)
  const [status, setStatus] = useState<"loading" | "ready" | "error">("loading")
  const [errorText, setErrorText] = useState<string>()

  useEffect(() => {
    let cancelled = false
    setStatus("loading")
    setErrorText(undefined)
    ;(async () => {
      try {
        const mermaid = await loadMermaid()
        const { svg } = await mermaid.render(renderId, source)
        if (cancelled || !host.current) return
        // Parse the SVG into an inert document and import the root <svg> element
        // rather than writing raw HTML strings directly.
        const parsed = new DOMParser().parseFromString(svg, "image/svg+xml")
        const svgEl = parsed.documentElement
        if (svgEl.tagName.toLowerCase() !== "svg") throw new Error("Mermaid did not produce an SVG diagram.")
        svgEl.removeAttribute("height")
        svgEl.style.maxWidth = "100%"
        svgEl.style.height = "auto"
        host.current.replaceChildren(document.importNode(svgEl, true))
        setStatus("ready")
      } catch (cause) {
        if (cancelled) return
        setErrorText(cause instanceof Error ? cause.message : String(cause))
        setStatus("error")
      }
    })()
    return () => {
      cancelled = true
    }
  }, [source, renderId])

  if (status === "error") {
    return (
      <div className="flex flex-col gap-2 rounded-chip bg-bg p-4 text-caption">
        <p className="text-status-failed">Could not draw this Mermaid diagram.</p>
        {errorText && <p className="font-mono text-text-muted">{errorText}</p>}
      </div>
    )
  }

  return (
    <div className={cn("overflow-auto rounded-chip bg-bg p-4", full ? "h-full w-full" : "max-h-shown")}>
      {status === "loading" && <p className="text-center text-caption text-text-muted">Drawing diagram…</p>}
      <div ref={host} className={cn("flex justify-center [&_svg]:max-w-full", status !== "ready" && "hidden")} />
    </div>
  )
}
