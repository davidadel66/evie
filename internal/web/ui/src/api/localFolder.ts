import type { Workspace } from "./contextSessions";

export type FolderRef = { workspaceId: string; revision: number };
export type FolderEntry = { path: string; name: string; directory: boolean };
export type FolderListing = { entries: FolderEntry[]; truncated: boolean };
export type DiskFile = { path: string; text: string; size: number };
export type GitChanges = { available: boolean; branch: string; files: { path: string; status: string }[] };
export function folderRef(workspace: Workspace): FolderRef {
  return { workspaceId: workspace.id, revision: workspace.folder?.revision ?? 0 };
}
export async function folderRequest<T>(action: string, body: object, signal?: AbortSignal): Promise<T> {
  const response = await fetch(action.startsWith("/") ? action : `/api/folder/${action}`, {
    method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify(body), signal,
  });
  const data = await response.json();
  if (!response.ok) throw new Error(data.error ?? "The folder could not be read.");
  return data;
}
