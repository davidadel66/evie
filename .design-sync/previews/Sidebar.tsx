import { Sidebar } from "ui";
import * as React from "react";
// The app root (App.tsx) sets these classes; every Evie screen sits on them.
const AppFrame = ({ children }: { children: React.ReactNode }) => (
  <div className="bg-app text-ink p-4 text-[13px]">{children}</div>
);
const noop = () => {};

const workspaces = [
  { id: "general", displayName: "General", state: "active", currentRevisionId: "rev-14", createdAt: "2026-06-01T09:00:00Z", updatedAt: "2026-10-01T09:00:00Z" },
  { id: "finance", displayName: "Finance", state: "active", currentRevisionId: "rev-31", createdAt: "2026-06-03T09:00:00Z", updatedAt: "2026-10-07T13:00:00Z" },
  { id: "intro-rl", displayName: "Intro to RL", state: "active", currentRevisionId: "rev-6", createdAt: "2026-09-12T09:00:00Z", updatedAt: "2026-10-05T18:00:00Z" },
];
const sessions = [
  { id: "s-general-1", workspaceId: "general", title: "Plan the week", status: "active", createdAt: "2026-10-06T08:00:00Z", updatedAt: "2026-10-06T08:40:00Z" },
  { id: "s-fin-1", workspaceId: "finance", title: "Credit card payment matching", status: "active", createdAt: "2026-10-07T13:10:00Z", updatedAt: "2026-10-07T13:42:00Z" },
  { id: "s-fin-2", workspaceId: "finance", title: "Why is Dining up 40% in September?", status: "active", createdAt: "2026-10-05T19:00:00Z", updatedAt: "2026-10-05T19:30:00Z" },
  { id: "s-fin-3", workspaceId: "finance", title: "", status: "active", createdAt: "2026-10-04T10:00:00Z", updatedAt: "2026-10-04T10:00:00Z" },
  { id: "s-fin-old", workspaceId: "finance", title: "Set up Chase sync", status: "closed", createdAt: "2026-07-01T10:00:00Z", updatedAt: "2026-07-01T11:00:00Z" },
  { id: "s-rl-1", workspaceId: "intro-rl", title: "Bellman equation walkthrough", status: "active", createdAt: "2026-10-05T17:00:00Z", updatedAt: "2026-10-05T18:00:00Z" },
];
const snapshot = { workspaces, projects: [], sessions, archivedSessions: [sessions[4]], activeSession: sessions[1] };

const callbacks = { onSettings: noop, onCreateWorkspace: noop, onArchive: noop, onCloseMobile: noop, onData: noop, onWorkspace: noop, onNewWorkspaceChat: noop, onSession: noop };

// The sidebar is an <aside> that fills the app's left column; the frame
// gives it the column height it has in the shell. mobileOpen keeps it
// visible below the md breakpoint as well.
const Shell = ({ children }: { children: React.ReactNode }) => (
  <div className="relative flex overflow-hidden" style={{ height: 560 }}>{children}</div>
);

export const FinanceSession = () => (
  <AppFrame><Shell>
    <Sidebar snapshot={snapshot as any} destination="workspace:finance" busy={false} mobileOpen {...callbacks} />
  </Shell></AppFrame>
);

export const DataWithReviewBadge = () => (
  <AppFrame><Shell>
    <Sidebar snapshot={{ ...snapshot, activeSession: undefined } as any} destination="data" busy={false} memoryReviewPending={3} mobileOpen {...callbacks} />
  </Shell></AppFrame>
);

export const Busy = () => (
  <AppFrame><Shell>
    <Sidebar snapshot={snapshot as any} destination="workspace:finance" busy mobileOpen {...callbacks} />
  </Shell></AppFrame>
);

export const ArchivedWorkspace = () => (
  <AppFrame><Shell>
    <Sidebar
      snapshot={{ ...snapshot, workspaces: [workspaces[0], workspaces[1], { ...workspaces[2], state: "archived" }], activeSession: sessions[0] } as any}
      destination="workspace:general" busy={false} mobileOpen {...callbacks}
      instructions={<p className="text-fainter px-3 pt-2 text-[11px]">Instructions enabled · revision 4</p>}
    />
  </Shell></AppFrame>
);

export const Empty = () => (
  <AppFrame><Shell>
    <Sidebar snapshot={{ workspaces: [], projects: [], sessions: [], archivedSessions: [] } as any} destination="workspaces" busy={false} mobileOpen {...callbacks} />
  </Shell></AppFrame>
);
