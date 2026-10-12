import { BuiltinWorkflowList } from "ui";
import * as React from "react";
// The app root (App.tsx) sets these classes; every Evie screen sits on them.
const AppFrame = ({ children }: { children: React.ReactNode }) => (
  <div className="bg-app text-ink p-4 text-[13px]">{children}</div>
);

const noop = () => {};
const deepResearch = {
  name: "deep-research", description: "Research one question from up to five search angles, then write a sourced answer.", inputs: ["question (string, required)"],
  workers: ["research"], max_tokens: 6_000_000, executable_hash: "sha256:" + "a".repeat(64), version: 1, enabled: true,
  workspace: { workspace_id: "finance", source: "builtin", version: 1 },
};
const blocked = { ...deepResearch, workspace: { workspace_id: "finance", source: "builtin", blocked: ["agent steps run research workers, which this Workspace does not allow"] } };
const off = { ...deepResearch, enabled: false, control: { seq: 3, name: "deep-research", enabled: false, reason: "trying my own copy first", surface: "web", recorded_at: "2026-10-06T15:30:00Z" } };
const overridden = { ...deepResearch, workspace: { workspace_id: "finance", source: "global", version: 2 } };
const ownCopy = { ...deepResearch, workspace: { workspace_id: "finance", source: "workspace", version: 1 } };

export const OnAndRunnable = () => (
  <AppFrame><BuiltinWorkflowList builtins={[deepResearch] as any} workspaceId="finance" busy={false} onToggle={noop} /></AppFrame>
);
export const BlockedHere = () => (
  <AppFrame><BuiltinWorkflowList builtins={[blocked] as any} workspaceId="finance" busy={false} onToggle={noop} /></AppFrame>
);
export const TurnedOff = () => (
  <AppFrame><BuiltinWorkflowList builtins={[off] as any} workspaceId="finance" busy={false} onToggle={noop} /></AppFrame>
);
export const Overridden = () => (
  <AppFrame><BuiltinWorkflowList builtins={[overridden, ownCopy] as any} workspaceId="finance" busy={false} onToggle={noop} /></AppFrame>
);
export const Busy = () => (
  <AppFrame><BuiltinWorkflowList builtins={[deepResearch] as any} workspaceId="finance" busy={true} onToggle={noop} /></AppFrame>
);
export const Empty = () => (
  <AppFrame><BuiltinWorkflowList builtins={[]} workspaceId="finance" busy={false} onToggle={noop} /></AppFrame>
);
