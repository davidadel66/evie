import { describe, expect, it } from "vitest";
import type { SemanticObjectSummary } from "../api/memory";
import { buildKnowledgeGraph, layoutKnowledgeGraph, parallelEdgeOffset } from "./knowledgeGraph";

const entity = (id: string, name: string, anchor_kind?: string): SemanticObjectSummary => ({
  object_kind: "entity", object_id: id, scope_key: "global", status: "active",
  entity: { entity_id: id, scope_key: "global", canonical_name: name, entity_type: "person", anchor_kind },
});

describe("Knowledge Graph", () => {
  it("keeps Claims first-class while resolving entity and literal endpoints", () => {
    const owner = entity("owner-1", "David", "owner");
    const city = entity("city-1", "Detroit");
    const claims: SemanticObjectSummary[] = [
      {
        object_kind: "claim", object_id: "claim-1", scope_key: "global", status: "active", subject: owner.entity, object_entity: city.entity,
        claim: {
          claim_id: "claim-1", scope_key: "global", subject_entity_id: "owner-1",
          predicate: { predicate_id: "p-1", token: "lives_in", version: 1, label: "lives in", object_constraint: "entity", cardinality: "one" },
          object: { entity_id: "city-1" }, polarity: "affirmed", valid_time: { from: null, to: null },
          created_operation_id: "op-1", transaction_time: "2026-09-04T12:00:00Z",
        },
      },
      {
        object_kind: "claim", object_id: "claim-2", scope_key: "global", status: "active", subject: owner.entity,
        claim: {
          claim_id: "claim-2", scope_key: "global", subject_entity_id: "owner-1",
          predicate: { predicate_id: "p-2", token: "timezone_name", version: 1, label: "time zone", object_constraint: "text", cardinality: "one" },
          object: { literal: { kind: "text", value: "America/Detroit" } }, polarity: "affirmed", valid_time: { from: null, to: null },
          created_operation_id: "op-2", transaction_time: "2026-09-04T12:00:00Z",
        },
      },
    ];

    const graph = buildKnowledgeGraph([owner, city], claims);
    expect(graph.nodes.map((node) => [node.kind, node.label])).toEqual([
      ["entity", "David"], ["entity", "Detroit"], ["literal", "America/Detroit"],
    ]);
    expect(graph.edges.map((edge) => [edge.label, edge.from, edge.to])).toEqual([
      ["lives in", "owner-1", "city-1"], ["time zone", "owner-1", "literal:claim-2"],
    ]);
    expect(graph.nodes.find((node) => node.id === "literal:claim-2")?.summary).toBe(claims[1]);
    const layout = layoutKnowledgeGraph(graph);
    expect(layout.nodes.find((node) => node.id === "owner-1")).toMatchObject({ x: 470, y: 278 });
    expect(graph.nodes.find((node) => node.id === "owner-1")?.summary).toMatchObject({ object_kind: "entity", object_id: "owner-1" });
  });

  it("grows the canvas so every node in the bounded result remains reachable", () => {
    const manyEntities = Array.from({ length: 49 }, (_, index) => entity(`entity-${index}`, `Entity ${index}`));

    const layout = layoutKnowledgeGraph(buildKnowledgeGraph(manyEntities, []));

    for (const node of layout.nodes) {
      expect(node.x).toBeGreaterThanOrEqual(0);
      expect(node.y).toBeGreaterThanOrEqual(0);
      expect(node.x + node.width).toBeLessThanOrEqual(layout.width);
      expect(node.y + node.height).toBeLessThanOrEqual(layout.height);
    }
  });

  it("assigns stable lanes to Claims sharing the same endpoints", () => {
    const owner = entity("owner-1", "David", "owner");
    const city = entity("city-1", "Detroit");
    const claim = (id: string, label: string): SemanticObjectSummary => ({
      object_kind: "claim", object_id: id, scope_key: "global", status: "active", subject: owner.entity, object_entity: city.entity,
      claim: {
        claim_id: id, scope_key: "global", subject_entity_id: "owner-1",
        predicate: { predicate_id: `predicate-${id}`, token: label.replaceAll(" ", "_"), version: 1, label, object_constraint: "entity", cardinality: "many" },
        object: { entity_id: "city-1" }, polarity: "affirmed", valid_time: { from: null, to: null },
        created_operation_id: `operation-${id}`, transaction_time: "2026-09-04T12:00:00Z",
      },
    });

    const graph = buildKnowledgeGraph([owner, city], [claim("claim-a", "lives in"), claim("claim-b", "works in")]);

    expect(graph.edges.map((edge) => [edge.id, edge.parallelIndex, edge.parallelCount, parallelEdgeOffset(edge)])).toEqual([
      ["claim-a", 0, 2, -16],
      ["claim-b", 1, 2, 16],
    ]);
  });
});
