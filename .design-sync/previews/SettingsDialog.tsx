import { SettingsDialog } from "ui";
import * as React from "react";
// The app root (App.tsx) sets these classes; every Evie screen sits on them.
const AppFrame = ({ children }: { children: React.ReactNode }) => (
  <div className="bg-app text-ink p-4 text-[13px]">{children}</div>
);
const noop = () => {};
const resolve = async () => {};

const workspaces = [
  { id: "general", displayName: "General", state: "active", currentRevisionId: "rev-14", createdAt: "", updatedAt: "" },
  { id: "finance", displayName: "Finance", state: "active", currentRevisionId: "rev-31", createdAt: "", updatedAt: "" },
  { id: "intro-rl", displayName: "Intro to RL", state: "active", currentRevisionId: "rev-6", createdAt: "", updatedAt: "" },
];
const archivedSessions = [
  { id: "a1", workspaceId: "finance", title: "Set up Chase sync", status: "closed", createdAt: "", updatedAt: "" },
  { id: "a2", workspaceId: "intro-rl", title: "Policy gradient notes", status: "closed", createdAt: "", updatedAt: "" },
  { id: "a3", title: "", status: "closed", createdAt: "", updatedAt: "" },
];
const snapshot = { workspaces, projects: [], sessions: [], archivedSessions };

// The archived view is internal state behind the "Archived sessions" row.
// These stories press that row once after mount so the view renders
// deterministically.
const OpenArchived = ({ children }: { children: React.ReactNode }) => {
  React.useEffect(() => {
    const buttons = Array.from(document.querySelectorAll<HTMLButtonElement>("dialog button"));
    buttons.find((b) => b.textContent?.includes("Archived sessions"))?.click();
  }, []);
  return <>{children}</>;
};

export const Appearance = () => (
  <AppFrame>
    <SettingsDialog snapshot={snapshot as any} busy={false} textSize="default" onTextSize={noop} font="default" onFont={noop} onRestore={resolve} onClose={noop} />
  </AppFrame>
);

export const LargeSerif = () => (
  <AppFrame>
    <SettingsDialog snapshot={snapshot as any} busy={false} textSize="large" onTextSize={noop} font="serif" onFont={noop} onRestore={resolve} onClose={noop} />
  </AppFrame>
);

export const SessionsLoading = () => (
  <AppFrame>
    <SettingsDialog busy={false} textSize="compact" onTextSize={noop} font="system" onFont={noop} onRestore={resolve} onClose={noop} />
  </AppFrame>
);

export const Archived = () => (
  <AppFrame>
    <OpenArchived>
      <SettingsDialog snapshot={snapshot as any} busy={false} onRefresh={resolve} textSize="default" onTextSize={noop} font="default" onFont={noop} onRestore={resolve} onClose={noop} />
    </OpenArchived>
  </AppFrame>
);

export const ArchivedUnavailable = () => (
  <AppFrame>
    <OpenArchived>
      <SettingsDialog problem="Sessions could not be loaded: the local database is locked by another Evie process." onRefresh={resolve} busy={false} textSize="default" onTextSize={noop} font="default" onFont={noop} onRestore={resolve} onClose={noop} />
    </OpenArchived>
  </AppFrame>
);
