import type { WorkspaceCreation } from "../api/contextSessions";
import type { PresetInspection } from "../api/management";

export type WorkspaceDraft = {
  name: string;
  presetId: string;
  folderPath: string;
};

export function workspaceCreation(draft: WorkspaceDraft, presets: PresetInspection[]): WorkspaceCreation {
  const displayName = draft.name.trim();
  if (!displayName) throw new Error("Enter a workspace name.");
  if (!presets.some((preset) => preset.id === draft.presetId && preset.valid)) {
    throw new Error("Choose an available agent preset.");
  }
  const options: WorkspaceCreation = { displayName, presetId: draft.presetId };
  if (draft.folderPath) {
    const folderPath = draft.folderPath;
    if (!folderPath.startsWith("/") || folderPath.includes("\0")) {
      throw new Error("Enter the full local folder path, starting with /.");
    }
    options.folderPath = folderPath;
  }
  return options;
}

export function presetLabel(id: string): string {
  if (id === "standard") return "Standard";
  if (id === "research") return "Research";
  return id;
}

export function presetDescription(preset: PresetInspection): string {
  if (preset.id === "standard") return "Evie’s full default toolset, using enabled plugins and normal approval rules.";
  if (preset.id === "research") return "Web search and page reading only. No memory, local files, shell, or other plugins.";
  const count = (preset.requiredCapabilities?.length ?? 0) + (preset.optionalCapabilities?.length ?? 0);
  return `${count} ${count === 1 ? "capability" : "capabilities"} configured in this preset. Actions still follow their normal approval rules.`;
}
