import { WorkflowCapLine } from "ui";
import * as React from "react";
// The app root (App.tsx) sets these classes; every Evie screen sits on them.
const AppFrame = ({ children }: { children: React.ReactNode }) => (
  <div className="bg-app text-ink p-4 text-[13px]">{children}</div>
);

const caps = {
  enforced: true, day: "2026-10-08", reset_at: "2026-10-09T00:00:00Z", cap: { tokens: 10_000_000, micro_usd: 5_000_000 },
  spent: { day: "2026-10-08", tokens: 2_345_678, micro_usd: 1_234_567, model_calls: 3, worker_calls: 12 }, reached: false, paused_runs: [],
};

export const UnderCap = () => (
  <AppFrame><WorkflowCapLine caps={caps as any} /></AppFrame>
);
export const RunsPaused = () => (
  <AppFrame><WorkflowCapLine caps={{ ...caps, spent: { ...caps.spent, tokens: 8_910_244, micro_usd: 4_402_118, worker_calls: 31 }, paused_runs: ["7f3e9c12-8b4d-4a6e-b2c5-1d0f6e8a9b74"] } as any} /></AppFrame>
);
export const Reached = () => (
  <AppFrame><WorkflowCapLine caps={{ ...caps, spent: { ...caps.spent, tokens: 10_004_912, micro_usd: 5_013_380, model_calls: 9, worker_calls: 44 }, reached: true, paused_runs: ["7f3e9c12-8b4d-4a6e-b2c5-1d0f6e8a9b74", "b1d4f0a7-3c2e-4d9b-8f16-7a5e9c2d0b31"] } as any} /></AppFrame>
);
export const NotEnforced = () => (
  <AppFrame><WorkflowCapLine caps={{ ...caps, enforced: false, spent: { ...caps.spent, tokens: 412_903, micro_usd: 218_450, model_calls: 2, worker_calls: 1 } } as any} /></AppFrame>
);
