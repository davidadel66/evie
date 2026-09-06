import { describe, expect, it } from "vitest";
import { metadataTimeFilter, pinMemoryRead, sameMemorySnapshot } from "./readSnapshot";

const metadata = {
  selected_scope: "global",
  allowed_scopes: ["global"],
  valid_at: "2026-09-04T19:00:00Z",
  as_known_at: "2026-09-04T19:00:00Z",
  scope_revisions: [{ scope_key: "global", revision: 4 }],
};

describe("Semantic Memory read snapshots", () => {
  it("pins missing time dimensions to one instant", () => {
    expect(pinMemoryRead({}, "2026-09-04T19:00:00Z")).toEqual({
      history: undefined,
      validAt: "2026-09-04T19:00:00Z",
      asKnownAt: "2026-09-04T19:00:00Z",
    });
    expect(pinMemoryRead({ validAt: "2026-09-03T12:00:00Z" }, "2026-09-04T19:00:00Z")).toMatchObject({
      validAt: "2026-09-03T12:00:00Z",
      asKnownAt: "2026-09-04T19:00:00Z",
    });
  });

  it("accepts only pages from the same scope, times, and revision", () => {
    expect(sameMemorySnapshot(metadata, { ...metadata, scope_revisions: [...metadata.scope_revisions].reverse() })).toBe(true);
    expect(sameMemorySnapshot(metadata, { ...metadata, scope_revisions: [{ scope_key: "global", revision: 5 }] })).toBe(false);
    expect(metadataTimeFilter(metadata, true)).toEqual({
      history: true,
      validAt: metadata.valid_at,
      asKnownAt: metadata.as_known_at,
    });
  });
});
