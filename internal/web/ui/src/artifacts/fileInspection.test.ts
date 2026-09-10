import { describe, expect, it } from "vitest";
import type { Item } from "../store/reducer";
import { decodeFileRead, inspectToolFile, selectedFileInspection } from "./fileInspection";

const read: Extract<Item, {kind: "tool"}> = {kind: "tool", key: "read", id: "c", name: "read_file", args: '{"path":"/src/main.go"}', startedAt: 0};
const edit: Extract<Item, {kind: "tool"}> = {...read, key: "edit", name: "edit_file", args: '{"path":"/src/main.go","old_string":"old","new_string":"new"}'};

describe("file inspection evidence", () => {
  it.each(["", "one", "one\n", "\tindented\r\n\n", "  42\tactual source\n", "</pre><script>alert(1)</script>"])("decodes exact numbered file bytes %j", (source) => {
    const numbered = source.split("\n").map((line, i) => `${String(i+1).padStart(6)}\t${line}\n`).join("");
    expect(decodeFileRead(numbered)).toEqual({text: source, coverage: "full"});
  });

  it.each(["[tool result bounded: original_bytes=100001 sha256=abc]\n     1\thead\n…", "     1\tfirst\n     3\tthird\n", "     1\tcut off"])("does not call partial or malformed output a full file", (result) => {
    expect(decodeFileRead(result)).toEqual({text: result, coverage: "excerpt"});
  });

  it("distinguishes a recorded read, waiting and failed reads", () => {
    expect(inspectToolFile({...read, result: "     1\tpackage main\n"})).toMatchObject({path: "/src/main.go", status: "Read", content: {kind: "code", coverage: "full", text: "package main"}});
    expect(inspectToolFile(read)).toMatchObject({status: "Requested", content: {kind: "unavailable"}});
    expect(inspectToolFile({...read, result: "Permission denied", isErr: true})).toMatchObject({status: "Failed", content: {kind: "unavailable", message: "Permission denied"}});
  });

  it("keeps complete live previews and labels historical replacements", () => {
    const preview = {path: "/resolved/main.go", oldText: "before\nold\nafter\n", newText: "before\nnew\nafter\n", isNew: false};
    expect(inspectToolFile({...edit, approval: {reqId: "p", state: "approved", preview}, result: "OK"})).toMatchObject({path: preview.path, status: "Edited", content: {kind: "change", coverage: "full", before: preview.oldText, after: preview.newText}});
    expect(inspectToolFile({...edit, result: "OK"})).toMatchObject({status: "Edited", content: {kind: "change", coverage: "replacement", before: "old", after: "new"}});
    expect(inspectToolFile({...edit, args: '{"path":"/src/main.go","old_string":"old","new_string":""}', result: "OK"})).toMatchObject({content: {after: ""}});
  });

  it("binds the selection to its session and follows the selected tool's outcome", () => {
    const selection = {sessionId: "session", key: "edit"};
    const pending = {...edit, approval: {reqId: "p", state: "pending" as const}};
    expect(selectedFileInspection(selection, "session", [pending])?.status).toBe("Awaiting approval");
    const failed = {...pending, approval: {reqId: "p", state: "approved" as const}, result: "File changed while approving", isErr: true};
    expect(selectedFileInspection(selection, "session", [failed, read])?.status).toBe("Failed");
    expect(selectedFileInspection(selection, "other", [failed])).toBeNull();
    expect(selectedFileInspection(selection, "session", [read])).toBeNull();
    expect(inspectToolFile({...pending, approval: {reqId: "p", state: "declined"}})?.status).toBe("Declined");
  });

  it("does not invent file links from shell commands or malformed arguments", () => {
    expect(inspectToolFile({...read, name: "bash", args: '{"command":"cat /tmp/file"}'})).toBeNull();
    expect(inspectToolFile({...read, args: "broken"})).toBeNull();
    expect(inspectToolFile({...edit, args: '{"path":"/tmp/file"}'})).toMatchObject({content: {kind: "unavailable"}});
  });
});
