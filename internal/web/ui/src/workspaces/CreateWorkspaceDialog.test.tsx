import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";
import type { PresetInspection } from "../api/management";
import { Dialog } from "../ui/Dialog";
import { CreateWorkspaceDialog, PresetDiagnostics, RepositoryChoiceFields } from "./CreateWorkspaceDialog";
import { defaultFolder, presetDescription, workspaceCreation, type WorkspaceDraft } from "./workspaceCreation";

const standard: PresetInspection = {
  id: "standard", version: "1", valid: true, immutable: true,
  requiredCapabilities: [], optionalCapabilities: [], warnings: [], errors: [],
};
const research: PresetInspection = { ...standard, id: "research" };
const draft: WorkspaceDraft = { name: "  Interview prep  ", presetId: "standard", folderPath: "" };

describe("workspace creation", () => {
	 it("requires explicit research permission with the Standard preset", () => {
	   expect(workspaceCreation({...draft, allowResearchDelegation: true}, [standard, research])).toMatchObject({allowResearchDelegation: true, presetId: "standard"});
	   expect(() => workspaceCreation({...draft, presetId: "research", allowResearchDelegation: true}, [standard, research])).toThrow("Standard");
	 });
  it("uses the full standard preset by default without requiring a folder", () => {
    expect(workspaceCreation(draft, [standard, research])).toEqual({ displayName: "Interview prep", presetId: "standard" });
  });

  it("attaches the exact native selection without creating it again", () => {
    expect(workspaceCreation({ ...draft, presetId: "research", folderPath: "/Users/david/research " }, [standard, research])).toEqual({ displayName: "Interview prep", presetId: "research", folderPath: "/Users/david/research " });
    expect(workspaceCreation({ ...draft, folderPath: "/Users/david/new-work" }, [standard])).toEqual({ displayName: "Interview prep", presetId: "standard", folderPath: "/Users/david/new-work" });
  });

  it("requires a name and an available exact preset without silently widening access", () => {
    expect(() => workspaceCreation({ ...draft, name: "   " }, [standard])).toThrow("Enter a workspace name");
    expect(() => workspaceCreation({ ...draft, presetId: "research" }, [standard])).toThrow("Choose an available agent preset");
    expect(() => workspaceCreation(draft, [{ ...standard, valid: false }])).toThrow("Choose an available agent preset");
    expect(() => workspaceCreation(draft, [])).toThrow("Choose an available agent preset");
  });

  it("rejects invalid native folder paths before registration", () => {
    for (const folderPath of ["   ", "Documents/research", "~research", "/Users/david/\0research"]) {
      expect(() => workspaceCreation({ ...draft, folderPath }, [standard])).toThrow("full local folder path");
    }
    // A typed ~/ path is sent as typed; the server expands it to the home folder.
    expect(workspaceCreation({ ...draft, folderPath: "~/research" }, [standard])).toMatchObject({ folderPath: "~/research" });
  });

  it("explains the research preset's actual capability boundary", () => {
    expect(presetDescription(research)).toContain("Web search and page reading only");
    expect(presetDescription(research)).toContain("No memory, local files, shell");
    expect(presetDescription(standard)).toContain("enabled plugins");
    expect(presetDescription(standard)).toContain("full default toolset");
    expect(presetDescription(standard)).toContain("approval rules");
  });

  it("opens creation as a named modal with all requested choices and disables submission until presets load", () => {
    const html = renderToStaticMarkup(<CreateWorkspaceDialog onClose={() => undefined} onCreate={async () => undefined} />);
    for (const text of ["<dialog", "aria-labelledby", "Close create workspace", "Workspace name", "data-dialog-autofocus", "Agent preset", "Standard — loading…", 'aria-label="Folder path"', "Choose…", "Cancel"]) expect(html).toContain(text);
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

  it("keeps optional plugin diagnostics collapsed without hiding their availability warning", () => {
    const warnings = Array.from({ length: 19 }, (_, index) => `Optional capability ${index} is unavailable.`);
    const html = renderToStaticMarkup(<PresetDiagnostics preset={{ ...standard, warnings }} />);
    expect(html).toContain("Some optional capabilities are unavailable");
    expect(html).toContain("<details");
    expect(html).not.toMatch(/<details[^>]*\bopen(?:=|\s|>)/);
    expect(html).toContain('aria-label="Capability details"');
    expect(html).toContain("Optional capability 18 is unavailable.");
  });

  it("shows a concise unavailable preset error before optional diagnostic details", () => {
    const html = renderToStaticMarkup(<PresetDiagnostics preset={{ ...research, valid: false, errors: ["Required capability web.search is unavailable."] }} />);
    expect(html).toContain('role="alert"');
    expect(html).toContain("Choose another preset or check plugin settings.");
    expect(html).toContain("Show unavailable capabilities");
    expect(html).not.toMatch(/<details[^>]*\bopen(?:=|\s|>)/);
  });
});

describe("repository at creation (changes.spec.md story 2b)", () => {
  const connected = {state: "connected" as const, login: "davidadel66"};
  const fields = (over: Partial<WorkspaceDraft>, account?: {state: "connected" | "no_token"; login?: string; message?: string}, repos: {name: string; full_name: string; private: boolean; owner: {login: string}}[] | null = null) =>
    renderToStaticMarkup(<RepositoryChoiceFields draft={{...draft, ...over}} account={account} repos={repos} disabled={false} onChange={() => {}} />);

  it("sends each repository choice with its folder", () => {
    expect(workspaceCreation({...draft, repository: "local", folderPath: "~/code/notes"}, [standard])).toMatchObject({folderPath: "~/code/notes", repository: {kind: "local"}});
    expect(workspaceCreation({...draft, repository: "github_new", folderPath: "~/code/notes", repoName: "notes"}, [standard]))
      .toMatchObject({repository: {kind: "github_new", name: "notes", private: true}});
    expect(workspaceCreation({...draft, repository: "github_new", folderPath: "~/code/notes", repoName: "notes", repoPublic: true}, [standard]))
      .toMatchObject({repository: {kind: "github_new", name: "notes", private: false}});
    expect(workspaceCreation({...draft, repository: "github_existing", folderPath: "~/code/evie", repoFullName: "davidadel66/evie"}, [standard]))
      .toMatchObject({repository: {kind: "github_existing", owner: "davidadel66", name: "evie"}});
    expect(workspaceCreation({...draft, repository: "none"}, [standard])).not.toHaveProperty("repository");
  });

  it("refuses a repository without a folder, a bad name or no chosen repository", () => {
    expect(() => workspaceCreation({...draft, repository: "local"}, [standard])).toThrow("folder for the repository");
    expect(() => workspaceCreation({...draft, repository: "github_new", folderPath: "~/code/x", repoName: "my notes"}, [standard])).toThrow("GitHub repository name");
    expect(() => workspaceCreation({...draft, repository: "github_new", folderPath: "~/code/x", repoName: ".."}, [standard])).toThrow("GitHub repository name");
    expect(() => workspaceCreation({...draft, repository: "github_existing", folderPath: "~/code/x"}, [standard])).toThrow("Choose one of your GitHub repositories");
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
