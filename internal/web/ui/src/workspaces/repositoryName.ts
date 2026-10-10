/** A GitHub repository name from a folder's: letters, digits, dots, dashes and underscores. */
export function repositoryName(folder: string | undefined): string {
  const base = (folder ?? "").split("/").filter(Boolean).pop() ?? "";
  return base.replace(/[^A-Za-z0-9_.-]+/g, "-").replace(/^-+|-+$/g, "").slice(0, 100);
}
