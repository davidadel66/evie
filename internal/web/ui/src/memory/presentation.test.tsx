import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";
import { entityLabel, MemoryPresentationProvider, MemoryScopeSelect } from "./presentation";
import type { SemanticEntity } from "../api/memory";

const owner: SemanticEntity = { entity_id: "stable-owner", scope_key: "global", canonical_name: "owner", entity_type: "person", anchor_kind: "owner" };

describe("memory presentation", () => {
  it("labels only the canonical owner without rewriting its identity", () => {
    expect(entityLabel(owner, "David")).toBe("David");
    expect(entityLabel(owner)).toBe("You");
    expect(entityLabel({ ...owner, anchor_kind: undefined }, "David")).toBe("owner");
    expect(owner.canonical_name).toBe("owner");
    expect(owner.entity_id).toBe("stable-owner");
  });
  it("uses workspace names and keeps conversation scopes in a secondary control", () => {
    const html = renderToStaticMarkup(<MemoryPresentationProvider snapshot={{
      ownerDisplayName: "David", projects: [], sessions: [],
      workspaces: [{ id: "general", displayName: "General", state: "active", currentRevisionId: "v1", createdAt: "", updatedAt: "" }],
    }}><MemoryScopeSelect value="workspace:general" onChange={() => undefined} scopes={[{ scope_key: "global" }, { scope_key: "workspace:general" }, { scope_key: "session:one" }]} /></MemoryPresentationProvider>);
    expect(html).toContain('value="workspace:general" selected="">General');
    expect(html).toContain('<optgroup label="Other scopes">');
    const primary = html.split('<optgroup')[0];
    expect(primary).not.toContain('value="session:one"');
    expect(html).toContain('value="session:one"');
  });
});
