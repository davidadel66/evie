import { useEffect, useRef, useState } from "react";
import type { Terminal } from "@xterm/xterm";
import "@xterm/xterm/css/xterm.css";
import type { FolderRef } from "../api/localFolder";
import { openTerminal, terminalControls } from "../api/terminal";

export function TerminalView({ root, active }: { root: FolderRef; active: boolean }) {
  const { workspaceId, revision } = root;
  const host = useRef<HTMLDivElement>(null);
  const runtime = useRef<{ terminal: Terminal; fit: () => void } | null>(null);
  const [attempt, setAttempt] = useState(0);
  const [status, setStatus] = useState("Starting terminal…");
  const [ended, setEnded] = useState(false);
  const activeRef = useRef(active);
  useEffect(() => {
    activeRef.current = active;
    if (!active) return;
    const frame = requestAnimationFrame(() => { runtime.current?.fit(); runtime.current?.terminal.focus(); });
    return () => cancelAnimationFrame(frame);
  }, [active]);
  useEffect(() => {
    const folder = { workspaceId, revision };
    const controller = new AbortController();
    let dispose = () => {};
    setEnded(false); setStatus("Starting terminal…");
    const fail = (error: unknown) => {
      if (controller.signal.aborted) return;
      setStatus(error instanceof Error ? error.message : "Terminal disconnected."); setEnded(true);
      if (runtime.current) runtime.current.terminal.options.disableStdin = true;
      controller.abort();
    };
    void (async () => {
      const [{ Terminal }, { FitAddon }] = await Promise.all([import("@xterm/xterm"), import("@xterm/addon-fit")]);
      if (controller.signal.aborted || !host.current) return;
      const terminal = new Terminal({ cols: 80, rows: 24, scrollback: 2000, disableStdin: true, cursorBlink: true, fontSize: 13, fontFamily: '"SFMono-Regular", Consolas, "Liberation Mono", monospace', theme: { background: "#161616", foreground: "#deded9", cursor: "#72c9b9", selectionBackground: "#35534e" } });
      const fit = new FitAddon(); terminal.loadAddon(fit); terminal.open(host.current);
      terminal.textarea?.setAttribute("aria-label", "Terminal input");
      let controls: ReturnType<typeof terminalControls> | undefined;
      let frame = 0;
      const fitVisible = () => {
        if (!host.current?.clientWidth || !host.current.clientHeight) return;
        const dimensions = fit.proposeDimensions();
        if (dimensions) terminal.resize(Math.max(2, Math.min(500, dimensions.cols)), Math.max(1, Math.min(200, dimensions.rows)));
      };
      runtime.current = { terminal, fit: fitVisible };
      const observer = new ResizeObserver(() => { cancelAnimationFrame(frame); frame = requestAnimationFrame(fitVisible); });
      observer.observe(host.current);
      const input = terminal.onData(data => controls?.input(data));
      const resize = terminal.onResize(({ cols, rows }) => controls?.resize(cols, rows));
      terminal.attachCustomKeyEventHandler(event => {
        if (event.key === "Escape" && event.shiftKey) {
          event.preventDefault(); event.stopPropagation();
          document.getElementById("pane-tab-terminal")?.focus(); return false;
        }
        return true;
      });
      dispose = () => { observer.disconnect(); cancelAnimationFrame(frame); input.dispose(); resize.dispose(); runtime.current = null; terminal.dispose(); };
      fitVisible(); if (activeRef.current) terminal.focus();
      await openTerminal(folder, { columns: terminal.cols, rows: terminal.rows }, controller.signal, async event => {
        if (controller.signal.aborted) return;
        if (event.type === "ready") { controls = terminalControls(folder, event.id, controller.signal, fail); controls.resize(terminal.cols, terminal.rows); terminal.options.disableStdin = false; setStatus("Connected"); }
        if (event.type === "output") {
          const bytes = Uint8Array.from(atob(event.data), char => char.charCodeAt(0));
          await new Promise<void>(resolve => {
            const done = () => { controller.signal.removeEventListener("abort", done); resolve(); };
            controller.signal.addEventListener("abort", done, { once: true });
            terminal.write(bytes, done);
          });
        }
        if (event.type === "exit") { controls = undefined; terminal.options.disableStdin = true; setStatus(`Shell exited (${event.code ?? 0})`); setEnded(true); }
      });
    })().catch(fail);
    return () => { controller.abort(); dispose(); };
  }, [workspaceId, revision, attempt]);
  return <div data-terminal className="flex min-h-0 min-w-0 flex-1 flex-col bg-[#161616]">
    <div className="border-editor-hair text-editor-muted flex flex-none items-center gap-3 border-b px-3 py-2 text-xs"><span role="status" className="min-w-0 flex-1 truncate" title={status}>{status}</span>{ended ? <button type="button" onClick={() => setAttempt(value => value + 1)} className="text-teal">New session</button> : <span className="text-[10px]">Shift+Esc leaves terminal</span>}</div>
    <div ref={host} className="min-h-0 min-w-0 flex-1 overflow-hidden p-2" />
  </div>;
}
