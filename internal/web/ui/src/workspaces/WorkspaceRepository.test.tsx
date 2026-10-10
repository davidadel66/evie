import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";
import type { Workspace } from "../api/contextSessions";
import type { Repository } from "../api/repository";
import { WorkspaceHome } from "./Workspaces";
import { RepositoryView } from "./WorkspaceRepository";

const github: Repository = {kind: "github", folder: "/Users/me/code/evie", base_branch: "master", remote_url: "git@github.com:davidadel66/evie.git", owner: "davidadel66", name: "evie",
  github: {state: "connected", login: "davidadel66", private: false, url: "https://github.com/davidadel66/evie"}};
const view = (repository: Repository) => renderToStaticMarkup(<RepositoryView repository={repository} />);

describe("Settings → Repository", () => {
  it("shows a GitHub Repository with its link, visibility, base branch, folder and token", () => {
    const html = view(github);
    expect(html).toMatch(/<a href="https:\/\/github.com\/davidadel66\/evie"[^>]*>davidadel66\/evie<\/a>/);
    expect(html).toContain(" · public");
    expect(html).toMatch(/Base branch<\/dt><dd[^>]*>master</);
    expect(html).toContain("/Users/me/code/evie");
    expect(html).toMatch(/text-ok">Connected as davidadel66</);
    expect(view({...github, github: {...github.github!, private: true}})).toContain(" · private");
  });

  it("says why GitHub is not connected, and still shows what git knows", () => {
    const html = view({...github, github: {state: "no_token", message: "GitHub is not connected. Add GITHUB_TOKEN to ~/.evie/.env and restart evie serve."}});
    expect(html).toMatch(/text-amber">GitHub is not connected\. Add GITHUB_TOKEN/);
    expect(html).toContain(">davidadel66/evie<");
    expect(html).not.toContain(" · public");
    expect(html).toMatch(/Base branch<\/dt><dd[^>]*>master</);
  });

  it("shows a local Repository, with a non-GitHub origin when it has one", () => {
    const html = view({kind: "local", folder: "/Users/me/code/notes", base_branch: "main", remote_url: "https://gitlab.com/someone/notes.git"});
    expect(html).toContain("On this Mac only");
    expect(html).toContain("https://gitlab.com/someone/notes.git");
    expect(html).not.toContain(">GitHub<");
  });

  it("says why there is no Repository", () => {
    const html = view({kind: "none", folder: "/Users/me/Documents", problem: "The folder is not a git repository."});
    expect(html).toContain("No repository");
    expect(html).toContain("The folder is not a git repository.");
  });

  it("is the first section of the Settings tab", () => {
    const workspace: Workspace = {id: "w", displayName: "evie", state: "active", currentRevisionId: "r", createdAt: "", updatedAt: ""};
    const html = renderToStaticMarkup(<WorkspaceHome tab="settings" workspace={workspace} sessions={[]} busy={false} onNewChat={()=>{}} onResume={()=>{}} />);
    const sections = [...html.matchAll(/<section aria-label="([^"]+)"/g)].map((m) => m[1]);
    expect(sections[0]).toBe("Repository");
    expect(html).toContain("Reading the repository…");
    expect(renderToStaticMarkup(<WorkspaceHome workspace={workspace} sessions={[]} busy={false} onNewChat={()=>{}} onResume={()=>{}} />)).not.toContain('aria-label="Repository"');
  });
});
