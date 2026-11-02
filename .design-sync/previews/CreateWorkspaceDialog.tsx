import { CreateWorkspaceDialog } from "ui";
import * as React from "react";
// The app root (App.tsx) sets these classes; every Evie screen sits on them.
const AppFrame = ({ children }: { children: React.ReactNode }) => (
  <div className="bg-app text-ink p-4 text-[13px]">{children}</div>
);
const noop = () => {};
const create = async () => {};

// The dialog asks only for a name, a repository and a folder; the server
// chooses the preset and research setting (workspaces.decisions.md,
// 2026-11-02).
export const Ready = () => (
  <AppFrame>
    <CreateWorkspaceDialog onClose={noop} onCreate={create} />
  </AppFrame>
);
