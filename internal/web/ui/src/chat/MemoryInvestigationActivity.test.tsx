import { expect, test } from "vitest";
import { renderToStaticMarkup } from "react-dom/server";
import { MemoryActivity } from "./MemoryActivity";

test("investigation activity preserves each resource and failure outcome", () => {
  for (const [status, label] of [
    ["partial", "Memory partially available"],
    ["failed", "Memory search failed"],
    ["unavailable", "Memory unavailable"],
    ["empty", "No matches"],
    ["cancelled", "Memory search cancelled"],
    ["exhausted", "Memory budget exhausted"],
  ]) {
    const html = renderToStaticMarkup(<MemoryActivity activity={{ snapshotId: "original", status, acceptedCount: status === "partial" || status === "exhausted" ? 1 : 0, excerptCount: 0 }} />);
    expect(html).toContain(label);
    if (status !== "empty") expect(html).not.toContain("No matches");
    if (status === "partial" || status === "exhausted") expect(html).toContain("1 supplied");
  }
});
