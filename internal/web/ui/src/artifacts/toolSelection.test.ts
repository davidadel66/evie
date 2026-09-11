import { expect, it } from "vitest";
import { selectedToolInspection } from "./toolSelection";
import type { ToolItem } from "../chat/activityModel";

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
