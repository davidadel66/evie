import type { ToolItem } from "../chat/activityModel";

// Builds in a chat (changes.spec.md story 6b): a delegate_build,
// continue_research or propose_build_merge row names the branch Evie built,
// and opening it shows that Change in the Inspector with its files and
// diffs, and a way to open it in the Changes tab.

export type BuildFile = { path: string; added: number; deleted: number; binary?: boolean };

/** One build a tool row names: its Change, branch and what the worker reported. */
export type BuildRef = { changeId: string; branch: string; status?: string; files: BuildFile[]; insertions: number; deletions: number };

const buildTools = new Set(["delegate_build", "continue_research", "propose_build_merge"]);

type Reported = { status?: string; branch?: string; files?: BuildFile[]; insertions?: number; deletions?: number; build?: Reported };

/** The builds a tool row's result names; none for any other row. */
export function buildsOfTool(tool: ToolItem): BuildRef[] {
  if (!buildTools.has(tool.name) || !tool.result) return [];
  let value: unknown;
  try {
    value = JSON.parse(tool.result.replace(/^tool call came back with error /, ""));
  } catch {
    return [];
  }
  const builds: BuildRef[] = [];
  for (const result of Array.isArray(value) ? value : [value]) {
    if (!result || typeof result !== "object") continue;
    const reported = result as Reported;
    const build = reported.build ?? (tool.name === "propose_build_merge" ? reported : undefined);
    if (!build || typeof build.branch !== "string" || !build.branch) continue;
    builds.push({ changeId: `build:${build.branch}`, branch: build.branch, status: reported.status, files: Array.isArray(build.files) ? build.files : [],
      insertions: build.insertions ?? 0, deletions: build.deletions ?? 0 });
  }
  return builds;
}
