import { renderToStaticMarkup } from "react-dom/server";
import { expect, it } from "vitest";
import { MemoryActivity } from "./MemoryActivity";
import { appendUser, reduce } from "../store/reducer";
import { Chat } from "./Chat";

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
