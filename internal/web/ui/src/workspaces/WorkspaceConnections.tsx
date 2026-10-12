import { useCallback, useEffect, useState } from "react";
import { connectionsAPI, type Connection, type ConnectionStatus, type ToolPolicy, type ToolReview } from "../api/connections";
import type { Workspace } from "../api/contextSessions";

// Settings → Connections (workspace-mcp-connections.spec.md story 3): the
// hosted MCP servers Evie is signed in to, and which of their tools this
// Workspace's chats may use, each set to Ask or Allow.

const statusLabel: Record<ConnectionStatus, string> = {
  connected: "Connected",
  not_connected: "Not connected",
  needs_sign_in: "Needs sign-in",
  unreachable: "Unreachable",
  keychain_unavailable: "Keychain unavailable",
};

export type Draft = { name: string; url: string; clientId: string; clientSecret: string };

export type ConnectionsViewProps = {
  active: boolean;
  connections: Connection[] | undefined;
  open: string | undefined;
  review: ToolReview | undefined;
  reviewProblem: string;
  busy: boolean;
  message: string;
  problem: string;
  draft: Draft;
  onToggle: (name: string) => void;
  onConnect: (name: string) => void;
  onDisconnect: (name: string) => void;
  onEnable: (name: string, tool: string, enabled: boolean) => void;
  onPolicy: (name: string, tool: string, policy: ToolPolicy) => void;
  onDraft: (draft: Draft) => void;
  onAdd: () => void;
};

export function ConnectionsView(p: ConnectionsViewProps) {
  const field = "border-hair bg-transparent text-body rounded-[6px] border px-2 py-1 text-xs";
  return <section aria-label="Connections" className="border-hair mt-7 border-b pb-7">
    <h2 className="text-body mb-1 text-sm font-medium">Connections</h2>
    <p className="text-muted-text mb-3 max-w-[620px] text-xs leading-5">Hosted MCP servers such as Square. Evie keeps each sign-in in your Mac's Keychain. Enable the tools this Workspace's chats may use; a chat started afterwards can use them. Every call asks you first unless you set the tool to Allow.</p>
    {p.connections === undefined ? <p className="text-muted-text text-xs">Reading connections…</p>
      : p.connections.length === 0 ? <p className="text-muted-text text-xs">No MCP servers yet. Add one below.</p>
      : <ul className="flex flex-col gap-3">{p.connections.map((c) => <li key={c.id} className="border-hair rounded-[8px] border p-3">
        <div className="flex flex-wrap items-center gap-2 text-xs">
          <span className="text-body font-medium">{c.name}</span>
          <span className={c.status === "connected" ? "text-ok" : "text-amber"}>{statusLabel[c.status] ?? c.status}</span>
          <span className="text-fainter min-w-0 flex-1 truncate">{c.url}</span>
          {c.status === "connected"
            ? <button type="button" disabled={p.busy} onClick={() => p.onDisconnect(c.name)} className="text-body underline underline-offset-2 disabled:opacity-40">Disconnect</button>
            : <button type="button" disabled={p.busy} onClick={() => p.onConnect(c.name)} className="text-teal-hover underline underline-offset-2 disabled:opacity-40">{c.status === "not_connected" ? "Connect" : "Reconnect"}</button>}
          <button type="button" aria-expanded={p.open === c.name} onClick={() => p.onToggle(c.name)} className="text-body underline underline-offset-2">{p.open === c.name ? "Hide tools" : "Tools"}</button>
        </div>
        {c.status_detail && c.status !== "connected" && <p className="text-muted-text mt-1 text-[11px]">{c.status_detail}</p>}
        {p.open === c.name && <ToolList {...p} name={c.name} />}
      </li>)}</ul>}
    <form className="mt-4 flex flex-wrap items-end gap-2" onSubmit={(e) => { e.preventDefault(); p.onAdd(); }}>
      <label className="text-muted-text flex flex-col gap-1 text-[11px]">Name<input className={field} value={p.draft.name} placeholder="square" disabled={!p.active || p.busy} onChange={(e) => p.onDraft({ ...p.draft, name: e.target.value })} /></label>
      <label className="text-muted-text flex flex-col gap-1 text-[11px]">Server URL<input className={field + " w-[260px]"} value={p.draft.url} placeholder="https://mcp.squareup.com/mcp" disabled={!p.active || p.busy} onChange={(e) => p.onDraft({ ...p.draft, url: e.target.value })} /></label>
      <label className="text-muted-text flex flex-col gap-1 text-[11px]">Client ID (optional)<input className={field} value={p.draft.clientId} disabled={!p.active || p.busy} onChange={(e) => p.onDraft({ ...p.draft, clientId: e.target.value })} /></label>
      <label className="text-muted-text flex flex-col gap-1 text-[11px]">Client secret (optional)<input type="password" autoComplete="off" className={field} value={p.draft.clientSecret} disabled={!p.active || p.busy} onChange={(e) => p.onDraft({ ...p.draft, clientSecret: e.target.value })} /></label>
      <button type="submit" disabled={!p.active || p.busy || !p.draft.name || !p.draft.url} className="border-teal-hair text-teal-hover rounded-[7px] border px-3 py-[5px] text-xs disabled:opacity-40">Add server</button>
    </form>
    <p role="status" className="text-muted-text mt-2 text-xs">{p.message}</p>
    {p.problem && <p role="alert" className="text-danger mt-2 text-xs">{p.problem}</p>}
  </section>;
}

function ToolList(p: ConnectionsViewProps & { name: string }) {
  if (p.reviewProblem) return <p role="alert" className="text-danger mt-2 text-xs">{p.reviewProblem}</p>;
  if (!p.review) return <p className="text-muted-text mt-2 text-xs">Listing tools…</p>;
  const editable = p.active && !p.busy;
  return <div className="mt-3">
    <p className="text-muted-text mb-2 text-[11px] leading-4">Allow runs the tool without asking you first, for this exact version only; a tool the server changes asks again.</p>
    <ul className="flex flex-col gap-2">{p.review.tools.map((tool) => <li key={tool.name} className="text-xs">
      <div className="flex flex-wrap items-center gap-2">
        <label className="text-body flex items-center gap-2">
          <input type="checkbox" className="accent-teal" aria-label={`Enable ${tool.name}`} checked={tool.state === "enabled"}
            disabled={!editable || tool.state === "unselectable"} onChange={(e) => p.onEnable(p.name, tool.name, e.target.checked)} />
          <code>{tool.name}</code>
        </label>
        {tool.state === "changed" && <span className="text-amber">Changed since review — check it, then enable it again</span>}
        {tool.state === "unselectable" && <span className="text-muted-text">Its name cannot be enabled</span>}
        {tool.state === "enabled" && <select aria-label={`Policy for ${tool.name}`} value={tool.policy ?? "ask"} disabled={!editable}
          onChange={(e) => p.onPolicy(p.name, tool.name, e.target.value as ToolPolicy)} className="border-hair bg-transparent text-body rounded-[6px] border px-1 py-[2px] text-[11px]">
          <option value="ask">Ask</option>
          <option value="allow">Allow</option>
        </select>}
      </div>
      {tool.description && <p className="text-muted-text mt-1 ml-6 max-w-[680px] whitespace-pre-wrap leading-5">{tool.description}</p>}
      {tool.input_schema !== undefined && <details className="ml-6 mt-1"><summary className="text-fainter cursor-pointer text-[11px]">Input schema</summary>
        <pre className="text-muted-text mt-1 max-h-60 overflow-auto text-[11px]">{JSON.stringify(tool.input_schema, null, 2)}</pre></details>}
    </li>)}</ul>
    {(p.review.no_longer_offered?.length ?? 0) > 0 && <p className="text-muted-text mt-2 text-[11px]">Enabled but no longer offered by the server: {p.review.no_longer_offered!.join(", ")}</p>}
  </div>;
}

export function WorkspaceConnections({ workspace }: { workspace: Workspace }) {
  const [connections, setConnections] = useState<Connection[]>();
  const [open, setOpen] = useState<string>();
  const [review, setReview] = useState<ToolReview>();
  const [reviewProblem, setReviewProblem] = useState("");
  const [busy, setBusy] = useState(false);
  const [message, setMessage] = useState("");
  const [problem, setProblem] = useState("");
  const [draft, setDraft] = useState<Draft>({ name: "", url: "", clientId: "", clientSecret: "" });

  const reload = useCallback(async () => {
    try { setConnections(await connectionsAPI.list()); }
    catch (error) { setConnections([]); setProblem(error instanceof Error ? error.message : "Connections could not be read."); }
  }, []);
  useEffect(() => { void reload(); }, [reload]);

  const loadReview = useCallback(async (name: string) => {
    setReview(undefined); setReviewProblem("");
    try { setReview(await connectionsAPI.tools(workspace.id, name)); }
    catch (error) { setReviewProblem(error instanceof Error ? error.message : "The tools could not be listed."); }
  }, [workspace.id]);

  const run = async (work: () => Promise<void>) => {
    setBusy(true); setProblem(""); setMessage("");
    try { await work(); }
    catch (error) { setProblem(error instanceof Error ? error.message : "That did not work. Try again."); }
    finally { setBusy(false); }
  };

  const props: ConnectionsViewProps = {
    active: workspace.state === "active", connections, open, review, reviewProblem, busy, message, problem, draft,
    onToggle: (name) => {
      if (open === name) { setOpen(undefined); return; }
      setOpen(name); void loadReview(name);
    },
    onConnect: (name) => void run(async () => {
      const attempt = await connectionsAPI.connect(name);
      window.open(attempt.authorization_url, "_blank", "noopener");
      setMessage(`Finish signing in to ${name} in the new tab…`);
      const connection = await connectionsAPI.wait(attempt.id);
      setMessage(`${connection.name} is ${statusLabel[connection.status] ?? connection.status}.`);
      await reload();
      if (open === name) await loadReview(name);
    }),
    onDisconnect: (name) => void run(async () => {
      await connectionsAPI.disconnect(name);
      setMessage(`${name} is disconnected. Its tools stay enabled here until you disable them.`);
      await reload();
    }),
    onEnable: (name, tool, enabled) => void run(async () => {
      setReview(await connectionsAPI.setEnabled(workspace.id, name, tool, enabled));
      setMessage(enabled ? `${tool} is enabled for new chats.` : `${tool} is disabled, including in open chats.`);
    }),
    onPolicy: (name, tool, policy) => void run(async () => {
      setReview(await connectionsAPI.setPolicy(workspace.id, name, tool, policy));
      setMessage(policy === "allow" ? `${tool} runs without asking.` : `${tool} asks before each call.`);
    }),
    onDraft: setDraft,
    onAdd: () => void run(async () => {
      const added = await connectionsAPI.add(draft.name.trim(), draft.url.trim(), draft.clientId.trim() || undefined, draft.clientSecret || undefined);
      setDraft({ name: "", url: "", clientId: "", clientSecret: "" });
      setMessage(`${added.name} added. Connect it to sign in.`);
      await reload();
    }),
  };
  return <ConnectionsView {...props} />;
}
