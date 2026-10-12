import { WorkflowRunStart } from "ui";
import * as React from "react";
// The app root (App.tsx) sets these classes; every Evie screen sits on them.
const AppFrame = ({ children }: { children: React.ReactNode }) => (
  <div className="bg-app text-ink p-4 text-[13px]">{children}</div>
);

const noop = () => {};

export const TwoWorkflows = () => (
  <AppFrame><WorkflowRunStart workflows={["finance-daily", "finance-review"]} busy={false} onStart={noop} /></AppFrame>
);
export const Busy = () => (
  <AppFrame><WorkflowRunStart workflows={["finance-daily", "finance-review"]} busy={true} onStart={noop} /></AppFrame>
);
export const NoneActive = () => (
  <AppFrame><WorkflowRunStart workflows={[]} busy={false} onStart={noop} /></AppFrame>
);
