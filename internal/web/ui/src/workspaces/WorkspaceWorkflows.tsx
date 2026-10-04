import { useCallback, useEffect, useState } from "react";
import type { Workspace } from "../api/contextSessions";
import { pendingProposal, workflowAPI, type WorkflowAPI, type WorkflowCard, type WorkflowDefinition } from "../api/workflows";

// Workflows bound to this Workspace (Stage 14 sub-stage 2). Definitions are
// proposed with `evie workflows propose`; here David reviews a proposal's
// approval card (what changes, the Standing Authority it requests, its dry
// run) and approves exactly that version, or deactivates one. Approval
// starts no run.

const statusLabel: Record<WorkflowDefinition["status"], string> = {
  proposed: "Proposed",
  active: "Active",
  blocked: "Blocked",
  inactive: "Inactive",
};

function shortHash(hash: string): string {
  return hash.startsWith("sha256:") ? hash.slice(0, 19) : hash;
}

export function WorkflowList({definitions, busy, onReview, onDeactivate}: {
  definitions: WorkflowDefinition[];
  busy: boolean;
  onReview: (definition: WorkflowDefinition) => void;
  onDeactivate: (definition: WorkflowDefinition) => void;
}) {
  if (definitions.length === 0) {
    return <p className="text-muted-text text-xs">No workflows have been proposed for this Workspace.</p>;
  }
  return <ul className="border-hair border-t">
    {definitions.map((d) => <li key={d.name} className="border-hair border-b py-3 text-xs">
      <div className="flex flex-wrap items-baseline gap-2">
        <span className="text-body font-mono">{d.name}</span>
        <span className={d.status === "blocked" ? "text-danger" : "text-muted-text"}>{statusLabel[d.status]}</span>
        {d.current && <span className="text-fainter">version {d.current.version} · {shortHash(d.current.executable_hash)}</span>}
        {d.pending > 0 && <span className="text-teal">{d.pending === 1 ? "1 proposal waiting" : `${d.pending} proposals waiting`}</span>}
      </div>
      {d.blocked?.map((reason) => <p key={reason} className="text-danger mt-1 leading-5">{reason}</p>)}
      <div className="mt-2 flex gap-3">
        {(d.pending > 0 || d.status === "blocked" || d.status === "inactive") && <button type="button" disabled={busy} onClick={() => onReview(d)} className="text-body underline underline-offset-2 disabled:opacity-40">Review proposal</button>}
        {d.current && !d.current.deactivated && <button type="button" disabled={busy} onClick={() => onDeactivate(d)} className="text-muted-text underline underline-offset-2 disabled:opacity-40">Deactivate version {d.current.version}</button>}
      </div>
    </li>)}
  </ul>;
}

export function WorkflowApprovalCard({card, busy, onApprove, onClose}: {
  card: WorkflowCard;
  busy: boolean;
  onApprove: () => void;
  onClose: () => void;
}) {
  const refusals = card.refusals ?? [];
  const simulation = card.proposal.simulation;
  const schedule = card.requirements.schedule;
  return <div role="dialog" aria-label={`Approve ${card.proposal.name}`} className="border-hair bg-app mt-3 rounded-[8px] border p-4 text-xs leading-5">
    <h3 className="text-body mb-2 text-sm font-medium">{card.proposal.name}: proposal {card.proposal.id}</h3>
    <p className="text-muted-text">Executable hash <code className="font-mono">{card.proposal.executable_hash}</code></p>
    <p className="text-muted-text">Authority hash <code className="font-mono">{card.proposal.authority_hash}</code></p>
    <p className="text-muted-text">{card.current ? `Compared with version ${card.current.version} (${card.current_status ?? "unknown"}).` : "No version has been approved yet."}</p>
    {card.diff.executable.length > 0 && <section className="mt-3">
      <h4 className="text-body font-medium">Executable changes</h4>
      {card.diff.executable.map((s) => <div key={s.section} className="mt-1">
        <p className="text-muted-text">{s.section}</p>
        <pre className="font-mono whitespace-pre-wrap">{[...(s.removed ?? []).map((l) => `- ${l}`), ...(s.added ?? []).map((l) => `+ ${l}`)].join("\n")}</pre>
      </div>)}
    </section>}
    {card.diff.documentation_changed && <p className="text-muted-text mt-2">Documentation changed (not part of the executable hash).</p>}
    <section className="mt-3">
      <h4 className="text-body font-medium">Authority changes</h4>
      {card.diff.authority.length === 0 ? <p className="text-muted-text">None.</p> : <pre className="font-mono whitespace-pre-wrap">{card.diff.authority.join("\n")}</pre>}
    </section>
    <section className="mt-3">
      <h4 className="text-body font-medium">Requested Standing Authority</h4>
      {card.requirements.authority.length === 0 ? <p className="text-muted-text">None.</p> : <ul>
        {card.requirements.authority.map((op) => <li key={`${op.capability}:${op.connection}`} className="font-mono">{op.capability} on {op.connection}: {op.resources.join(", ")}, at most {op.max_calls_per_run} calls per run</li>)}
      </ul>}
      {card.requirements.workers.length > 0 && <p className="text-muted-text">Workers: {card.requirements.workers.join(", ")}</p>}
      <p className="text-muted-text">{schedule ? `Schedule: ${schedule.cron} ${schedule.timezone}. Approving also activates this schedule; scheduled runs start once schedules ship.` : "Schedule: none (manual starts only)."}</p>
    </section>
    <p className="text-muted-text mt-3">{simulation ? `Dry run: ${simulation.status}, ${simulation.attempts} attempts, ${simulation.tokens} tokens, ${simulation.previews.length} effect previews (none executed).` : "Dry run: none."}{simulation?.error ? ` ${simulation.error}` : ""}</p>
    {refusals.length > 0 && <div role="alert" className="text-danger mt-3"><p>Approval would be refused now:</p><ul>{refusals.map((r) => <li key={r}>{r}</li>)}</ul></div>}
    <div className="mt-3 flex items-center gap-3">
      {card.needs_approval
        ? <button type="button" disabled={busy || refusals.length > 0} onClick={onApprove} className="bg-teal text-primary-foreground rounded-[6px] px-3 py-1 font-semibold disabled:opacity-40">Approve version {card.expected_version + 1}</button>
        : <p className="text-muted-text">{card.proposal.documents_version ? `No approval needed: this changes documentation of active version ${card.proposal.documents_version}.` : "No approval needed: this is the active version."}</p>}
      <button type="button" disabled={busy} onClick={onClose} className="text-body underline underline-offset-2">Close</button>
    </div>
  </div>;
}

export function WorkspaceWorkflows({workspace, api = workflowAPI}: {workspace: Workspace; api?: WorkflowAPI}) {
  const [definitions, setDefinitions] = useState<WorkflowDefinition[] | null>(null);
  const [card, setCard] = useState<WorkflowCard | null>(null);
  const [busy, setBusy] = useState(false);
  const [problem, setProblem] = useState("");
  const [message, setMessage] = useState("");
  const refresh = useCallback(async () => {
    const page = await api.list(workspace.id);
    setDefinitions(page.workflows);
  }, [api, workspace.id]);
  useEffect(() => {
    refresh().catch(() => setProblem("Workflows could not be loaded."));
  }, [refresh]);
  const run = async (action: () => Promise<string>) => {
    setBusy(true); setProblem(""); setMessage("");
    try { setMessage(await action()); }
    catch (error) { setProblem(error instanceof Error ? error.message : "The workflow request failed. Refresh before trying again."); }
    finally { setBusy(false); }
  };
  const review = (definition: WorkflowDefinition) => run(async () => {
    const detail = await api.show(workspace.id, definition.name);
    // A waiting proposal, or, for a blocked definition, the approved
    // version's own proposal, which can be approved again.
    const id = pendingProposal(detail)?.id ?? detail.current?.proposal_id ?? detail.proposals?.[0]?.id;
    if (!id) return "There is no proposal to review.";
    setCard(await api.diff(id));
    return "";
  });
  const approve = () => run(async () => {
    if (!card) return "";
    const activation = await api.approve(card);
    setCard(null);
    await refresh();
    return `Activated ${activation.name} version ${activation.version}. No run was started.`;
  });
  const deactivate = (definition: WorkflowDefinition) => run(async () => {
    if (!definition.current) return "";
    const ended = await api.deactivate(workspace.id, definition.name, definition.current.version);
    await refresh();
    return `Deactivated ${ended.name} version ${ended.version}. No new run will start.`;
  });
  return <section aria-label="Workflows" className="border-hair mt-7 border-b pb-7">
    <h2 className="text-body mb-3 text-sm font-medium">Workflows</h2>
    <p className="text-muted-text mb-3 max-w-[620px] text-xs leading-5">Reviewed procedures this Workspace can run. Propose one with <code className="font-mono">evie workflows propose {workspace.id} &lt;folder&gt; --fixtures &lt;file&gt;</code>, or a template bundled with Evie (<code className="font-mono">evie workflows templates</code>) with <code className="font-mono">evie workflows propose {workspace.id} --template deep-research</code>; approving a version grants exactly the Standing Authority it shows, within this Workspace&apos;s access, and starts no run. Any change to its behavior needs a new approval.</p>
    {definitions === null ? <p className="text-muted-text text-xs">Loading workflows…</p>
      : <WorkflowList definitions={definitions} busy={busy || workspace.state !== "active"} onReview={(d) => void review(d)} onDeactivate={(d) => void deactivate(d)} />}
    {card && <WorkflowApprovalCard card={card} busy={busy || workspace.state !== "active"} onApprove={() => void approve()} onClose={() => setCard(null)} />}
    <p role="status" className="text-muted-text mt-2 text-xs">{busy ? "Working…" : message}</p>
    {problem && <p role="alert" className="text-danger mt-2 text-xs">{problem}</p>}
  </section>;
}
