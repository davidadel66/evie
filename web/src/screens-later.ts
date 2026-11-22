// The screens the page loads later, as one file: the bundler builds this
// one, and all they import that the page's first script does not, into a
// file of its own, which screens.ts fetches.
export { SettingsPage } from "./Settings.tsx"
export { Palette } from "./Palette.tsx"
export { DetailsCard } from "./ProjectCard.tsx"
export { RightPanel } from "./DiffPanel.tsx"
export { WorkspaceView } from "./Workspace.tsx"
export { ShownFrame } from "./Shown.tsx"
