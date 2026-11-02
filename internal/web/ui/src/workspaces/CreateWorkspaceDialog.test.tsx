import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";
import { Dialog } from "../ui/Dialog";
import { CreateWorkspaceDialog, RepositoryChoiceFields } from "./CreateWorkspaceDialog";
import { defaultFolder, workspaceCreation, type WorkspaceDraft } from "./workspaceCreation";

const draft: WorkspaceDraft = { name: "  Interview prep  ", folderPath: "" };

describe("workspace creation", () => {
  // Sending neither lets the server create the Workspace on the Standard
  // preset with research delegation on (workspace-research.spec.md, amended
  // 2026-11-02).
  it("leaves the preset and research delegation to the server", () => {
    expect(workspaceCreation(draft)).toEqual({ displayName: "Interview prep" });
  });

  it("attaches the exact native selection without creating it again", () => {
    expect(workspaceCreation({ ...draft, folderPath: "/Users/david/research " })).toEqual({ displayName: "Interview prep", folderPath: "/Users/david/research " });
  });

  it("requires a name", () => {
    expect(() => workspaceCreation({ ...draft, name: "   " })).toThrow("Enter a workspace name");
  });

  it("rejects invalid native folder paths before registration", () => {
    for (const folderPath of ["   ", "Documents/research", "~research", "/Users/david/\0research"]) {
      expect(() => workspaceCreation({ ...draft, folderPath })).toThrow("full local folder path");
    }
    // A typed ~/ path is sent as typed; the server expands it to the home folder.
    expect(workspaceCreation({ ...draft, folderPath: "~/research" })).toMatchObject({ folderPath: "~/research" });
  });

  it("opens creation as a named modal with only a name, repository and folder to choose", () => {
    const html = renderToStaticMarkup(<CreateWorkspaceDialog onClose={() => undefined} onCreate={async () => undefined} />);
    for (const text of ["<dialog", "aria-labelledby", "Close create workspace", "Workspace name", "data-dialog-autofocus", 'aria-label="Folder path"', "Choose…", "Cancel"]) expect(html).toContain(text);
    for (const text of ["Agent preset", "<select", "research delegation", 'type="checkbox"']) expect(html).not.toContain(text);
    // Submission waits for a name, not for anything to load.
    expect(html).toMatch(/<button type="submit" disabled=""/);
    // The only choices are the Repository's (changes.spec.md story 2b), not folder modes.
    expect([...html.matchAll(/name="([a-z-]+)"/g)].every((m) => m[1] === "repository")).toBe(true);
    expect(html).not.toContain("Attach existing");
    expect(html).not.toContain("Create new");
    expect(html).not.toContain("Existing folder path");
  });

  it("prevents dismissal controls from implying a cancellable in-flight save", () => {
    const html = renderToStaticMarkup(<Dialog title="Create workspace" onClose={() => undefined} busy><p>Creating…</p></Dialog>);
    expect(html).toContain('aria-busy="true"');
    expect(html).toMatch(/aria-label="Close create workspace" disabled=""/);
  });
});

describe("repository at creation (changes.spec.md story 2b)", () => {
  const connected = {state: "connected" as const, login: "davidadel66"};
  const fields = (over: Partial<WorkspaceDraft>, account?: {state: "connected" | "no_token"; login?: string; message?: string}, repos: {name: string; full_name: string; private: boolean; owner: {login: string}}[] | null = null) =>
    renderToStaticMarkup(<RepositoryChoiceFields draft={{...draft, ...over}} account={account} repos={repos} disabled={false} onChange={() => {}} />);

  it("sends each repository choice with its folder", () => {
    expect(workspaceCreation({...draft, repository: "local", folderPath: "~/code/notes"})).toMatchObject({folderPath: "~/code/notes", repository: {kind: "local"}});
    expect(workspaceCreation({...draft, repository: "github_new", folderPath: "~/code/notes", repoName: "notes"}))
      .toMatchObject({repository: {kind: "github_new", name: "notes", private: true}});
    expect(workspaceCreation({...draft, repository: "github_new", folderPath: "~/code/notes", repoName: "notes", repoPublic: true}))
      .toMatchObject({repository: {kind: "github_new", name: "notes", private: false}});
    expect(workspaceCreation({...draft, repository: "github_existing", folderPath: "~/code/evie", repoFullName: "davidadel66/evie"}))
      .toMatchObject({repository: {kind: "github_existing", owner: "davidadel66", name: "evie"}});
    expect(workspaceCreation({...draft, repository: "none"})).not.toHaveProperty("repository");
  });

  it("refuses a repository without a folder, a bad name or no chosen repository", () => {
    expect(() => workspaceCreation({...draft, repository: "local"})).toThrow("folder for the repository");
    expect(() => workspaceCreation({...draft, repository: "github_new", folderPath: "~/code/x", repoName: "my notes"})).toThrow("GitHub repository name");
    expect(() => workspaceCreation({...draft, repository: "github_new", folderPath: "~/code/x", repoName: ".."})).toThrow("GitHub repository name");
    expect(() => workspaceCreation({...draft, repository: "github_existing", folderPath: "~/code/x"})).toThrow("Choose one of your GitHub repositories");
  });

  it("suggests ~/code/<name> for a repository's folder", () => {
    expect(defaultFolder("Interview prep")).toBe("~/code/Interview-prep");
    expect(defaultFolder("evie")).toBe("~/code/evie");
    expect(defaultFolder("  ")).toBe("");
  });

  it("offers the GitHub choices only with a working token, and says why not", () => {
    const off = fields({}, {state: "no_token", message: "GitHub is not connected: add GITHUB_TOKEN to ~/.evie/.env and restart evie serve."});
    expect(off.match(/<input type="radio"[^>]*disabled=""[^>]*name="repository"\/>/g)).toHaveLength(2);
    expect(off).toContain("GitHub is not connected");
    expect(fields({})).toContain("Checking GitHub…");
    const on = fields({}, connected);
    expect(on).not.toContain('disabled=""');
    expect(on).toContain("GitHub: connected as davidadel66.");
    expect([...on.matchAll(/name="repository"(?: checked="")?\/>([^<]+)</g)].map((m) => m[1]))
      .toEqual(["No repository", "New repository on this Mac", "New repository on GitHub", "Connect a GitHub repository"]);
  });

  it("asks a new GitHub repository's name, private unless chosen public", () => {
    const html = fields({repository: "github_new", repoName: "notes"}, connected);
    expect(html).toContain('value="notes"');
    expect(html).toMatch(/name="repo-visibility" checked=""\/>Private/);
    expect(html).not.toContain("Anyone on the internet");
    expect(fields({repository: "github_new", repoName: "notes", repoPublic: true}, connected)).toContain("Anyone on the internet will be able to read this repository.");
  });

  it("lists the owner's repositories to connect", () => {
    expect(fields({repository: "github_existing"}, connected)).toContain("Loading your repositories…");
    const html = fields({repository: "github_existing", repoFullName: "davidadel66/evie"}, connected, [
      {name: "evie", full_name: "davidadel66/evie", private: false, owner: {login: "davidadel66"}},
      {name: "notes", full_name: "davidadel66/notes", private: true, owner: {login: "davidadel66"}},
    ]);
    expect(html).toMatch(/<option value="davidadel66\/evie" selected="">davidadel66\/evie<\/option>/);
    expect(html).toContain(">davidadel66/notes (private)<");
  });
});
