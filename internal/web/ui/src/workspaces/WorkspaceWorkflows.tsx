import type { WorkflowCard, WorkflowDefinition } from "../api/workflows";

// Workflows bound to this Workspace (Stage 14 sub-stage 2). Definitions are
// proposed with `evie workflows propose`; here David reviews a proposal's
// approval card (what changes, the Standing Authority it requests, its dry
// run) and approves exactly that version, or deactivates one. Approval
// starts no run. Since the built-in decision (2026-10-04) the built-in
// workflows are listed here too, with their on/off switch; there is no
// "add from template" step.
//
// The Workflows tab (WorkspaceWorkflowsTab.tsx, workspace-activity.spec.md)
// replaced the container that lived here; the approval card is reused in
// its pop-up, and the list stays for the design system.

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
      {d.current?.schedule && <p className="text-muted-text mt-1">Schedule: {d.current.schedule.cron} {d.current.schedule.timezone}, catch-up {d.current.schedule.catch_up}{d.status === "active" ? "" : " (not running: the workflow is not active)"}</p>}
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
      <p className="text-muted-text">{schedule ? `Schedule: ${schedule.cron} ${schedule.timezone}, catch-up ${schedule.catch_up}${schedule.max_runs ? ` (at most ${schedule.max_runs})` : ""}. Approving also activates this schedule: its runs start at their times until you pause it (evie workflows schedule pause) or deactivate the workflow.` : "Schedule: none (manual starts only)."}</p>
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
