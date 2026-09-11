import { expect, it } from "vitest";
import { selectedToolInspection, selectedMemoryTools } from "./toolSelection";
import type { ToolItem } from "../chat/activityModel";
import { appendUser, reduce } from "../store/reducer";

it("associates recorded memory results with the selected snapshot's turn and session", () => {
  let items = appendUser([], "preferences");
  items = reduce(items, {type: "turn_started", id: "first", status: "working"});
  items = reduce(items, {type: "memory_activity", snapshotId: "snapshot", status: "empty", acceptedCount: 0, excerptCount: 0});
  items = reduce(items, {type: "tool_call", id: "list", name: "memory_list_objects", args: "{}"});
  const selection = {sessionId: "one", snapshotId: "snapshot"};
  expect(selectedMemoryTools(selection, "one", items)).toMatchObject([{id: "list"}]);
  expect(selectedMemoryTools(selection, "one", items)[0].result).toBeUndefined();
  items = reduce(items, {type: "tool_result", id: "list", content: "recorded first turn result", isError: false});
  items = reduce(items, {type: "assistant_done", content: "Done", terminal: true});
  items = appendUser(items, "another topic");
  items = reduce(items, {type: "turn_started", id: "second", status: "working"});
  items = reduce(items, {type: "tool_call", id: "other", name: "memory_list_objects", args: "{}"});
  expect(selectedMemoryTools(selection, "one", items)).toMatchObject([{id: "list", result: "recorded first turn result"}]);
  expect(selectedMemoryTools(selection, "two", items)).toEqual([]);
  expect(selectedMemoryTools({...selection, snapshotId: "missing"}, "one", items)).toEqual([]);
});

it("pins a tool to its session and follows its result without selecting later actions", () => {
  const tool: ToolItem = {kind: "tool", key: "search", id: "c", name: "memory_search", args: '{"query":"saved diet"}', startedAt: 0};
  const selection = {sessionId: "one", key: tool.key};
  expect(selectedToolInspection(selection, "one", [tool])).toEqual(tool);
  const completed = {...tool, result: "exact saved result"};
  expect(selectedToolInspection(selection, "one", [completed, {...tool, key: "later"}])).toEqual(completed);
  expect(selectedToolInspection(selection, "two", [completed])).toBeNull();
  expect(selectedToolInspection(selection, "one", [{...tool, key: "later"}])).toBeNull();
  expect(selectedToolInspection(undefined, "one", [tool])).toBeNull();
});
