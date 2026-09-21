import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";
import type { PresetInspection } from "../api/management";
import { Dialog } from "../ui/Dialog";
import { CreateWorkspaceDialog, PresetDiagnostics } from "./CreateWorkspaceDialog";
import { presetDescription, workspaceCreation, type WorkspaceDraft } from "./workspaceCreation";

const standard: PresetInspection = {
  id: "standard", version: "1", valid: true, immutable: true,
  requiredCapabilities: [], optionalCapabilities: [], warnings: [], errors: [],
};
const research: PresetInspection = { ...standard, id: "research" };
const draft: WorkspaceDraft = { name: "  Interview prep  ", presetId: "standard", folderPath: "" };

describe("workspace creation", () => {
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
    for (const folderPath of ["   ", "Documents/research", "~/research", "/Users/david/\0research"]) {
      expect(() => workspaceCreation({ ...draft, folderPath }, [standard])).toThrow("full local folder path");
    }
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
    for (const text of ["<dialog", "aria-labelledby", "Close create workspace", "Workspace name", "data-dialog-autofocus", "Agent preset", "Standard — loading…", "Choose folder…", "New Folder in the macOS picker", "Cancel"]) expect(html).toContain(text);
    expect(html).toMatch(/<button type="submit" disabled=""/);
    expect(html).not.toContain('type="radio"');
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
