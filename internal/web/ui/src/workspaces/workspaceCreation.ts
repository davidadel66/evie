import type { RepositoryChoice, WorkspaceCreation } from "../api/contextSessions";
import type { PresetInspection } from "../api/management";

export type RepositoryKind = "none" | RepositoryChoice["kind"];

export type WorkspaceDraft = {
	allowResearchDelegation?: boolean;
  name: string;
  presetId: string;
  folderPath: string;
  /** The Repository the folder starts with (changes.spec.md story 2b). */
  repository?: RepositoryKind;
  /** github_new: the new repository's name. */
  repoName?: string;
  /** github_new: private unless the owner chose public. */
  repoPublic?: boolean;
  /** github_existing: "owner/name". */
  repoFullName?: string;
};

const githubName = /^[A-Za-z0-9_.-]{1,100}$/;

/** The folder a new repository goes in by default: ~/code/<name>. */
export function defaultFolder(name: string): string {
  const slug = name.trim().replace(/[^A-Za-z0-9_.-]+/g, "-").replace(/^-+|-+$/g, "");
  return slug ? `~/code/${slug}` : "";
}

export function workspaceCreation(draft: WorkspaceDraft, presets: PresetInspection[]): WorkspaceCreation {
  const displayName = draft.name.trim();
  if (!displayName) throw new Error("Enter a workspace name.");
  if (!presets.some((preset) => preset.id === draft.presetId && preset.valid)) {
    throw new Error("Choose an available agent preset.");
  }
  const options: WorkspaceCreation = { displayName, presetId: draft.presetId };
  if (draft.allowResearchDelegation) {
    if (draft.presetId !== "standard") throw new Error("Research delegation requires the Standard preset.");
    if (!presets.some(preset => preset.id === "research" && preset.valid)) throw new Error("The Research preset must be available to allow delegation.");
    options.allowResearchDelegation = true;
  }
  // The folder is sent exactly as chosen: a folder's name may end in a space.
  const folderPath = draft.folderPath;
  if (folderPath) {
    if (!(folderPath.startsWith("/") || folderPath.startsWith("~/")) || folderPath.includes("\0")) {
      throw new Error("Enter the full local folder path, starting with / or ~/.");
    }
    options.folderPath = folderPath;
  }
  const kind = draft.repository ?? "none";
  if (kind === "none") return options;
  if (!folderPath) throw new Error("Enter the folder for the repository.");
  if (kind === "local") {
    options.repository = { kind };
  } else if (kind === "github_new") {
    const name = (draft.repoName ?? "").trim();
    if (!githubName.test(name) || /^\.+$/.test(name)) throw new Error("Enter a GitHub repository name: letters, digits, dots, dashes or underscores.");
    options.repository = { kind, name, private: !draft.repoPublic };
  } else {
    const [owner, name, ...rest] = (draft.repoFullName ?? "").split("/");
    if (!owner || !name || rest.length > 0) throw new Error("Choose one of your GitHub repositories.");
    options.repository = { kind, owner, name };
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
