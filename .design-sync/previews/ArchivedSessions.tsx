import { ArchivedSessions } from "ui";
import * as React from "react";
// The app root (App.tsx) sets these classes; every Evie screen sits on them.
const AppFrame = ({ children }: { children: React.ReactNode }) => (
  <div className="bg-app text-ink p-4 text-[13px]">{children}</div>
);
const noop = () => {};

const workspaces = [
  { id: "general", displayName: "General", state: "active", currentRevisionId: "rev-14", createdAt: "", updatedAt: "" },
  { id: "finance", displayName: "Finance", state: "active", currentRevisionId: "rev-31", createdAt: "", updatedAt: "" },
  { id: "intro-rl", displayName: "Intro to RL", state: "active", currentRevisionId: "rev-6", createdAt: "", updatedAt: "" },
];
const projects = [
  { id: "p-evie", displayName: "evie", canonicalRoot: "/Users/david/code/evie", archived: false, createdAt: "", updatedAt: "" },
];
const archivedSessions = [
  { id: "a1", workspaceId: "finance", title: "Set up Chase sync", status: "closed", createdAt: "", updatedAt: "" },
  { id: "a2", workspaceId: "intro-rl", title: "Policy gradient notes", status: "closed", createdAt: "", updatedAt: "" },
  { id: "a3", projectId: "p-evie", title: "Migrate transactions table to allocations", status: "closed", createdAt: "", updatedAt: "" },
  { id: "a4", title: "", status: "closed", createdAt: "", updatedAt: "" },
];
const snapshot = { workspaces, projects, sessions: [], archivedSessions };

// The list lives in the Settings dialog body (520px wide, sidebar surface).
const DialogBody = ({ children }: { children: React.ReactNode }) => (
  <div className="border-hair-input bg-sidebar max-w-[520px] rounded-xl border px-6 py-5">
    <p className="text-muted-text mb-5 text-xs leading-5">Restore a conversation to bring it back to your workspace.</p>
    {children}
  </div>
);

export const List = () => (
  <AppFrame><DialogBody>
    <ArchivedSessions snapshot={snapshot as any} busy={false} onRefresh={noop} onRestore={noop} />
  </DialogBody></AppFrame>
);

export const Restoring = () => (
  <AppFrame><DialogBody>
    <ArchivedSessions snapshot={snapshot as any} busy restoring="a2" onRefresh={noop} onRestore={noop} />
  </DialogBody></AppFrame>
);

export const Empty = () => (
  <AppFrame><DialogBody>
    <ArchivedSessions snapshot={{ ...snapshot, archivedSessions: [] } as any} busy={false} onRestore={noop} />
  </DialogBody></AppFrame>
);

export const Loading = () => (
  <AppFrame><DialogBody>
    <ArchivedSessions busy={false} onRestore={noop} />
  </DialogBody></AppFrame>
);

export const Unavailable = () => (
  <AppFrame><DialogBody>
    <ArchivedSessions problem="Sessions could not be loaded: the local database is locked by another Evie process." onRefresh={noop} busy={false} onRestore={noop} />
  </DialogBody></AppFrame>
);

export const Retrying = () => (
  <AppFrame><DialogBody>
    <ArchivedSessions problem="Sessions could not be loaded: the local database is locked by another Evie process." refreshing onRefresh={noop} busy={false} onRestore={noop} />
  </DialogBody></AppFrame>
);
