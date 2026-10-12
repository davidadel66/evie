import { Dialog } from "ui";
import * as React from "react";
// The app root (App.tsx) sets these classes; every Evie screen sits on them.
const AppFrame = ({ children }: { children: React.ReactNode }) => (
  <div className="bg-app text-ink p-4 text-[13px]">{children}</div>
);
const noop = () => {};
const primary = "bg-teal text-primary-foreground hover:bg-teal-hover rounded-lg px-4 py-2.5 text-xs font-semibold disabled:opacity-40";
const secondary = "text-muted-text hover:bg-hover hover:text-ink rounded-lg px-4 py-2.5 text-xs font-medium disabled:opacity-40";

// A native <dialog> opened with showModal(): it paints in the browser's top
// layer, centred over a dimmed backdrop, regardless of where it is mounted.
export const Confirm = () => (
  <AppFrame>
    <Dialog title="Archive session" description="Archived sessions leave the sidebar. You can restore them from Settings." onClose={noop}>
      <p className="text-body text-[13px] leading-5">Archive <span className="text-ink font-medium">Credit card payment matching</span> from the Finance workspace?</p>
      <footer className="border-hair mt-5 flex items-center justify-end gap-2 border-t pt-5">
        <button type="button" className={secondary}>Cancel</button>
        <button type="button" className={primary}>Archive</button>
      </footer>
    </Dialog>
  </AppFrame>
);

export const Busy = () => (
  <AppFrame>
    <Dialog title="Rename workspace" description="The new name shows in the sidebar and in every session header." busy onClose={noop}>
      {/* Nothing inside is focusable while busy; park focus here so the browser does not outline the <dialog> itself. */}
      <div data-dialog-autofocus tabIndex={-1} className="outline-none">
      <label className="text-body mb-2 block text-xs font-medium">Workspace name</label>
      <input readOnly value="Intro to RL" disabled className="border-hair-input bg-card text-ink w-full rounded-lg border px-3 py-2.5 text-[13px] disabled:opacity-50" />
      </div>
      <footer className="border-hair mt-5 flex items-center justify-end gap-2 border-t pt-5">
        <button type="button" disabled className={secondary}>Cancel</button>
        <button type="button" disabled className={primary}>Saving…</button>
      </footer>
    </Dialog>
  </AppFrame>
);

export const TitleOnly = () => (
  <AppFrame>
    <Dialog title="Workflow run 9cbf8363" onClose={noop}>
      <dl className="grid grid-cols-[120px_1fr] gap-y-2 text-xs">
        <dt className="text-muted-text">Workflow</dt><dd className="text-body">finance-daily v1</dd>
        <dt className="text-muted-text">State</dt><dd className="text-body">succeeded</dd>
        <dt className="text-muted-text">Surface</dt><dd className="text-body">schedule</dd>
        <dt className="text-muted-text">Finished</dt><dd className="text-body font-mono">2026-10-07 13:00:06</dd>
        <dt className="text-muted-text">Outputs</dt><dd className="text-body font-mono">added 6 · removed 2 · categorized 6</dd>
      </dl>
    </Dialog>
  </AppFrame>
);
