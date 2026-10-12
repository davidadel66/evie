// Evie's design system as a library entry (design-sync, 2026-10-08). Only
// props-driven components belong here: each renders from its arguments
// alone, so Claude Design can build with it without Evie's server. The
// containers that fetch (Spending, Workspaces, WorkspaceWorkflows...) stay
// out. Adding a component here puts it in the next sync.
export { Banner } from "./ui/Banner";
export { Dialog } from "./ui/Dialog";
export { TextSizeMenu } from "./ui/TextSizeMenu";
export { Sidebar } from "./shell/Sidebar";
export { SettingsDialog, ArchivedSessions } from "./shell/SettingsDialog";
export { CreateWorkspaceDialog, PresetDiagnostics } from "./workspaces/CreateWorkspaceDialog";
export { SpendingView } from "./data/Spending";
export { CashFlowView } from "./data/CashFlow";
export { SpendingTransactionsView } from "./data/SpendingTransactions";
export { DayTransactionRows } from "./data/SpendingDayDialog";
export { SpendingAccountsView } from "./data/SpendingAccounts";
export { DataHubView } from "./data/DataHub";
export { DatabaseView } from "./data/Database";
export { UsageView } from "./data/Usage";
export { WorkflowList, WorkflowApprovalCard } from "./workspaces/WorkspaceWorkflows";
export { WorkflowRunList, WorkflowRunView, WorkflowRunStart } from "./workspaces/WorkspaceWorkflowRuns";
export { WorkflowInterruptionAnswer } from "./workspaces/WorkflowInterruptionAnswer";
export { WorkflowNotificationList, WorkflowCapLine } from "./workspaces/WorkspaceWorkflowNotifications";
export { BuiltinWorkflowList } from "./workspaces/WorkspaceBuiltinWorkflows";
