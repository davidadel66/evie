import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";
import { FileCode, FileViewer } from "./FileViewer";
import { Panel } from "./Panel";
import { Diff } from "../chat/Diff";
import { Activity } from "../chat/Activity";
import { activityTurns } from "../chat/activityModel";
import type { FileInspection } from "./fileInspection";
import { inspectToolFile } from "./fileInspection";
import type { Item } from "../store/reducer";

describe("file side panel", () => {
  const file: FileInspection = {key: "t", path: "/src/main.go", status: "Edited", content: {kind: "change", before: "old\n", after: "new\n", coverage: "full", isNew: false}};

  it("shows the selected file, source code and changes control in the inspector", () => {
    const html = renderToStaticMarkup(<Panel target={{kind: "file", file}} focused={false} onClose={() => {}} />);
    for (const text of ["main.go", "/src/main.go", "Edited", "Recorded contents after this edit", "Changes", "new", 'aria-label="Inspector"', 'aria-label="Close inspector"']) expect(html).toContain(text);
    expect(html).toContain("Complete file change");
  });

  it("marks proposed and historical content honestly", () => {
    const proposed = renderToStaticMarkup(<FileViewer file={{...file, status: "Declined"}} />);
    expect(proposed).toContain("Declined");
    expect(proposed).toContain("Proposed file");
    expect(proposed).not.toContain("Recorded contents after this edit");
    const partial = renderToStaticMarkup(<FileViewer file={{...file, content: {kind: "change", before: "old", after: "new", coverage: "replacement", isNew: false}}} />);
    expect(partial).toContain("Recorded replacement only");
    expect(partial).toContain("Replacement");
    expect(partial).not.toContain("Removed lines"); // No invented source line numbers.
  });

  it("renders HTML-looking source as inert text", () => {
    const html = renderToStaticMarkup(<FileCode path="source.html" text={'<script>alert(1)</script>\n<img src=x onerror="bad()">\n'} />);
    expect(html).toContain("&lt;script&gt;");
    expect(html).not.toContain("<script>");
    expect(html).not.toContain("<img");
  });

  it("preserves the recorded approval after successful and failed edits", () => {
    for (const isErr of [false, true]) {
      const inspected = inspectToolFile({kind: "tool", key: "t", id: "c", name: "edit_file", args: '{"path":"/src/main.go","old_string":"old","new_string":"new"}', result: isErr ? "write failed" : "OK", isErr, startedAt: 0, approval: {reqId: "p", state: "approved"}});
      expect(inspected).not.toBeNull();
      const html = renderToStaticMarkup(<FileViewer file={inspected!} />);
      expect(html).toContain("Approval: Approved");
      expect(html).toContain(isErr ? "Failed" : "Edited");
    }
  });

  it("keeps before and after side by side within the side panel", () => {
    const html = renderToStaticMarkup(<Diff oldText="old\n" newText="new\n" fit />);
    expect(html).toContain("Before");
    expect(html).toContain("After");
    expect(html).not.toContain("min-w-[820px]");
    const partial = renderToStaticMarkup(<Diff oldText="old" newText="new" fit partial />);
    expect(partial).toContain("Recorded replacement preview");
    expect(partial).not.toContain("Complete file change preview");
    expect(partial).not.toContain("No newline at end of file");
  });

  it("renders a separate file action outside the tool disclosure", () => {
    const items: Item[] = [{kind: "user", key: "u", text: "read"}, {kind: "tool", key: "t", id: "c", name: "read_file", args: '{"path":"/src/main.go"}', result: "     1\tpackage main\n", startedAt: 0}];
    const html = renderToStaticMarkup(<Activity group={activityTurns(items, true)[0]} onAnswer={() => {}} onOpenFile={() => {}} />);
    expect(html).toContain('aria-label="Open /src/main.go"');
    expect(html).not.toContain("<details");
    expect(html).not.toContain("Arguments");
    expect(html).not.toContain("Result");
    expect(html.slice(html.indexOf("<summary"),html.indexOf("</summary>"))).not.toContain("<button");
  });

  it("keeps completed and pending file diffs out of chat", () => {
    for (const state of ["approved", "pending"] as const) {
      const items: Item[] = [{kind: "user", key: "u", text: "edit"}, {kind: "tool", key: "t", id: "c", name: "edit_file", args: '{"path":"/src/main.go","old_string":"before","new_string":"after"}', result: state === "approved" ? "OK" : undefined, startedAt: 0, approval: {reqId: "p", state, preview: {path: "/src/main.go", oldText: "before contents", newText: "after contents", isNew: false}}}];
      const html = renderToStaticMarkup(<Activity group={activityTurns(items, true)[0]} onAnswer={() => {}} onOpenFile={() => {}} />);
      expect(html).toContain('aria-label="Open /src/main.go"');
      expect(html).not.toContain("Complete file change");
      expect(html).not.toContain("before contents");
      expect(html).not.toContain("Arguments");
      if (state === "pending") {
        expect(html).toContain("Approve");
        expect(html).toContain("Decline");
      }
    }
  });
});
