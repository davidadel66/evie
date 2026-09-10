import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";
import { reduce } from "../store/reducer";
import { splitBatch } from "../store/useSession";
import { Reasoning } from "./Reasoning";

describe("reasoning activity without a public summary", () => {
  it("keeps the thinking timer without offering an empty panel", () => {
    const [events, rest] = splitBatch([{ type: "reasoning", text: "" }], 0);
    expect(rest).toEqual([]);
    let items = reduce([], events[0], () => 1000);
    const live = items[0];
    if (live.kind !== "reasoning") throw new Error("missing thinking indicator");
    const liveHTML = renderToStaticMarkup(<Reasoning item={live} />);
    expect(liveHTML).toContain("Thinking…");
    expect(liveHTML).not.toContain("<button");
    expect(liveHTML).not.toContain("<svg");

    items = reduce(items, { type: "reasoning_done" }, () => 5000);
    const done = items[0];
    if (done.kind !== "reasoning") throw new Error("missing completed timer");
    const doneHTML = renderToStaticMarkup(<Reasoning item={done} />);
    expect(doneHTML).toContain("Thought for 4s");
    expect(doneHTML).not.toContain("<button");
    expect(doneHTML).not.toContain("<svg");
  });

  it("shows supplied public text during streaming and offers expansion afterwards", () => {
    let items = reduce([], { type: "reasoning", text: "" }, () => 1000);
    items = reduce(items, { type: "reasoning", text: "Checking the arithmetic." }, () => 2000);
    const live = items[0];
    if (live.kind !== "reasoning") throw new Error("missing reasoning summary");
    const liveHTML = renderToStaticMarkup(<Reasoning item={live} />);
    expect(liveHTML).toContain("Checking the arithmetic.");
    expect(liveHTML).toContain('aria-expanded="true"');

    items = reduce(items, { type: "reasoning_done" }, () => 5000);
    const done = items[0];
    if (done.kind !== "reasoning") throw new Error("missing reasoning summary");
    const doneHTML = renderToStaticMarkup(<Reasoning item={done} />);
    expect(doneHTML).toContain("Checking the arithmetic.");
    expect(doneHTML).toContain("- 4s");
    expect(doneHTML).not.toContain("Thought for");
    expect(doneHTML).toContain('aria-expanded="false"');
    // Only the header preview is rendered while the full body is collapsed.
    expect(doneHTML.split("Checking the arithmetic.")).toHaveLength(2);
  });

  it("normalizes multiline summaries into one preview without dropping their text", () => {
    const summary = "  Compare the options.\n\nCheck\t the constraints.  ";
    const html = renderToStaticMarkup(
      <Reasoning item={{ kind: "reasoning", key: "r", text: summary, streaming: false, startedAt: 1000, ms: 4000 }} />,
    );
    expect(html).toContain("Compare the options. Check the constraints.");
    expect(html).toContain("- 4s");
    expect(html).toContain('aria-expanded="false"');
  });

  it("keeps the timer fallback for whitespace-only summaries", () => {
    const html = renderToStaticMarkup(
      <Reasoning item={{ kind: "reasoning", key: "r", text: " \n\t ", streaming: false, startedAt: 1000, ms: 4000 }} />,
    );
    expect(html).toContain("Thought for 4s");
    expect(html).not.toContain("<button");
  });
});
