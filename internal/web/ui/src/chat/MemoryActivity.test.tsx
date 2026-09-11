import { renderToStaticMarkup } from "react-dom/server";
import { expect, it } from "vitest";
import { MemoryActivity } from "./MemoryActivity";
import { appendUser, reduce } from "../store/reducer";
import { Chat } from "./Chat";

it("labels a saved request as recorded until its assistant response commits", () => {
  let items = appendUser([], "Check my earlier sources.");
  items = reduce(items, { type: "turn_started", id: "receipt-root", status: "working" });
  items = reduce(items, { type: "memory_activity", snapshotId: "pending-request", requestStatus: "prepared", iteration: 2, status: "success", acceptedCount: 1, excerptCount: 1 });
  const pending = items.find((item) => item.kind === "memory");
  expect(pending?.kind === "memory" && pending.memory.requestStatus).toBe("prepared");
  const pendingHtml = renderToStaticMarkup(<Chat items={items} queued={[]} streaming={false} onAnswer={() => undefined} onOpenMemory={() => undefined} />);
  expect(pendingHtml).toContain("2 recorded");
  expect(pendingHtml).not.toContain("2 supplied");
  items = reduce(items, { type: "assistant_done", content: "Evidence checked.", terminal: true });
  const completed = items.find((item) => item.kind === "memory");
  expect(completed?.kind === "memory" && completed.memory.requestStatus).toBe("completed");
  expect(renderToStaticMarkup(<Chat items={items} queued={[]} streaming={false} onAnswer={() => undefined} onOpenMemory={() => undefined} />)).toContain("2 supplied");
});

it("offers the supplied accepted memory for inspection without claiming an answer citation", () => {
  const html = renderToStaticMarkup(<MemoryActivity activity={{ snapshotId: "request-1", status: "success", acceptedCount: 2, excerptCount: 0 }} onOpen={() => undefined} />);
  expect(html).toContain("Accepted memory");
  expect(html).toContain("2 supplied");
  expect(html).toContain('aria-label="Inspect supplied memory"');
  expect(html).not.toContain("Cited");
  expect(html).not.toContain("Conversation excerpt");
});

it("keeps the supplied-memory activity attached to its completed turn", () => {
  let items = appendUser([], "What timezone did I save?");
  items = reduce(items, { type: "turn_started", id: "root-1", status: "working" });
  items = reduce(items, { type: "memory_activity", snapshotId: "request-1", status: "success", acceptedCount: 1, excerptCount: 0 });
  items = reduce(items, { type: "assistant_done", content: "Detroit.", terminal: true, parts: [{ text: "Detroit.", phase: "final_answer" }] });
  const html = renderToStaticMarkup(<Chat items={items} queued={[]} streaming={false} onAnswer={() => undefined} onOpenMemory={() => undefined} />);
  expect(html).toContain("Accepted memory");
  expect(html).toContain("Detroit.");
  expect(items.find((item) => item.kind === "memory")?.turn?.id).toBe("root-1");
});

it("distinguishes unavailable memory from a successful empty search", () => {
  const unavailable = renderToStaticMarkup(<MemoryActivity activity={{ snapshotId: "request-2", status: "failed", acceptedCount: 0, excerptCount: 0 }} onOpen={() => undefined} />);
  const empty = renderToStaticMarkup(<MemoryActivity activity={{ snapshotId: "request-3", status: "empty", acceptedCount: 0, excerptCount: 0 }} onOpen={() => undefined} />);
  expect(unavailable).toContain("Memory unavailable");
  expect(unavailable).not.toContain("No matches");
  expect(empty).toContain("No matches");
  expect(empty).not.toContain("Memory unavailable");
});

it("labels conversation excerpts separately and counts all supplied evidence", () => {
  const conversation = renderToStaticMarkup(<MemoryActivity activity={{ snapshotId: "conversation-request", status: "success", acceptedCount: 0, excerptCount: 2 }} onOpen={() => undefined} />);
  expect(conversation).toContain("Conversation excerpt");
  expect(conversation).toContain("2 supplied");
  expect(conversation).not.toContain("Accepted memory");
  const mixed = renderToStaticMarkup(<MemoryActivity activity={{ snapshotId: "mixed-request", status: "success", acceptedCount: 1, excerptCount: 2 }} onOpen={() => undefined} />);
  expect(mixed).toContain("Accepted memory");
  expect(mixed).toContain("Conversation excerpt");
  expect(mixed).toContain("3 supplied");
});

it("summarizes historical, retired and conflicting evidence as recorded for that request", () => {
  const activity = { snapshotId: "historical-request", status: "success", acceptedCount: 2, excerptCount: 1, historicalCount: 2, retiredCount: 1, conflictCount: 2 };
  const html = renderToStaticMarkup(<MemoryActivity activity={activity} onOpen={() => undefined} />);
  for (const value of ["3 supplied", "2 historical", "1 marked retired", "2 with conflicts"]) expect(html).toContain(value);
  expect(html).not.toContain("2 conflicts");
  expect(html).not.toContain("retired now");
});

it("retains original historical annotations through live activity reduction", () => {
  let items = appendUser([], "What did I say before the change?");
  items = reduce(items, { type: "turn_started", id: "historical-root", status: "working" });
  items = reduce(items, { type: "memory_activity", snapshotId: "historical-request", status: "success", acceptedCount: 1, excerptCount: 0, historicalCount: 1, retiredCount: 1, conflictCount: 1 });
  items = reduce(items, { type: "assistant_done", content: "You had recorded Boston.", terminal: true, parts: [{ text: "You had recorded Boston.", phase: "final_answer" }] });
  const html = renderToStaticMarkup(<Chat items={items} queued={[]} streaming={false} onAnswer={() => undefined} onOpenMemory={() => undefined} />);
  for (const value of ["1 historical", "1 marked retired", "1 with conflicts"]) expect(html).toContain(value);
});
