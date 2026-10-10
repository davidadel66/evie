import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";
import type { Workspace } from "../api/contextSessions";
import type { Repository } from "../api/repository";
import { WorkspaceHome } from "./Workspaces";
import { PublishForm, RepositoryView } from "./WorkspaceRepository";
import { repositoryName } from "./repositoryName";

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

describe("Publish to GitHub", () => {
  const local: Repository = {kind: "local", folder: "/Users/me/code/My Notes!", base_branch: "main"};
  const connected = {state: "connected" as const, login: "davidadel66"};

  it("is offered for a local Repository without an origin, and only there", () => {
    const offer = (repository: Repository) => renderToStaticMarkup(<RepositoryView repository={repository} publish={{open: false, busy: false}} onOpenPublish={()=>{}} onPublish={()=>{}} onCancelPublish={()=>{}} />);
    expect(offer(local)).toMatch(/<button[^>]*>Publish to GitHub<\/button>/);
    expect(offer({...local, remote_url: "https://gitlab.com/someone/notes.git"})).not.toContain("Publish to GitHub");
    expect(offer(github)).not.toContain("Publish to GitHub");
    expect(offer({kind: "none", problem: "The folder is not a git repository."})).not.toContain("Publish to GitHub");
  });

  it("names the repository after the folder, as GitHub allows", () => {
    expect(repositoryName("/Users/me/code/My Notes!")).toBe("My-Notes");
    expect(repositoryName("/Users/me/code/evie/")).toBe("evie");
    expect(repositoryName(undefined)).toBe("");
  });

  it("starts private, names the account and says what it pushes", () => {
    const html = renderToStaticMarkup(<RepositoryView repository={local} publish={{open: true, busy: false, account: connected}} onOpenPublish={()=>{}} onPublish={()=>{}} onCancelPublish={()=>{}} />);
    expect(html).toMatch(/<form aria-label="Publish to GitHub"/);
    expect(html).toContain('value="My-Notes"');
    expect(html).toMatch(/<input type="radio"[^>]*name="visibility" checked=""\/>Private/);
    expect(html).toMatch(/<input type="radio"[^>]*name="visibility"\/>Public/);
    expect(html).toContain(">davidadel66<");
    expect(html).toContain("pushes the base branch. Other branches stay on this Mac.");
    expect(html).toMatch(/<button type="submit"[^>]*>Publish to GitHub<\/button>/);
    expect(html).not.toMatch(/<button type="submit" disabled=""/);
  });

  it("cannot publish until GitHub is connected", () => {
    const checking = renderToStaticMarkup(<PublishForm defaultName="notes" state={{open: true, busy: false}} onPublish={()=>{}} onCancel={()=>{}} />);
    expect(checking).toContain("Checking GitHub…");
    expect(checking).toMatch(/<button type="submit" disabled=""/);
    const missing = renderToStaticMarkup(<PublishForm defaultName="notes" state={{open: true, busy: false, account: {state: "no_token", message: "GitHub is not connected: add GITHUB_TOKEN to ~/.evie/.env and restart evie serve."}}} onPublish={()=>{}} onCancel={()=>{}} />);
    expect(missing).toMatch(/role="alert"[^>]*>GitHub is not connected/);
    expect(missing).toMatch(/<button type="submit" disabled=""/);
  });

  it("shows why a publish failed and offers to try again", () => {
    const html = renderToStaticMarkup(<PublishForm defaultName="notes" state={{open: true, busy: false, account: connected,
      problem: "the push to GitHub failed: created https://github.com/davidadel66/notes and set origin, but pushing main failed: timeout. Publish again with the same name to retry the push"}} onPublish={()=>{}} onCancel={()=>{}} />);
    expect(html).toContain("Publish again with the same name to retry the push");
    expect(html).toMatch(/<button type="submit"[^>]*>Try again<\/button>/);
  });
});
