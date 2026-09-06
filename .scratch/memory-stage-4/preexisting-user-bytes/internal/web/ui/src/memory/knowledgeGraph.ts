import type { SemanticEntity, SemanticObjectSummary } from "../api/memory";

export type KnowledgeNode = {
  id: string;
  kind: "entity" | "literal";
  label: string;
  detail: string;
  entity?: SemanticEntity;
  summary?: SemanticObjectSummary;
  anchor?: boolean;
};

export type KnowledgeEdge = {
  id: string;
  from: string;
  to: string;
  label: string;
  polarity: string;
  summary: SemanticObjectSummary;
  parallelIndex: number;
  parallelCount: number;
};

export type KnowledgeGraph = { nodes: KnowledgeNode[]; edges: KnowledgeEdge[] };

export type KnowledgeLayoutNode = KnowledgeNode & {
  x: number;
  y: number;
  width: number;
  height: number;
};

export type KnowledgeLayout = {
  nodes: KnowledgeLayoutNode[];
  edges: KnowledgeEdge[];
  width: number;
  height: number;
};

const canvasWidth = 1100;
const canvasHeight = 620;
const nodeWidth = 160;
const nodeHeight = 64;

export function buildKnowledgeGraph(entityObjects: SemanticObjectSummary[], claimObjects: SemanticObjectSummary[]): KnowledgeGraph {
  const nodes = new Map<string, KnowledgeNode>();
  const addEntity = (entity: SemanticEntity, summary?: SemanticObjectSummary) => {
    const existing = nodes.get(entity.entity_id);
    nodes.set(entity.entity_id, {
      id: entity.entity_id,
      kind: "entity",
      label: entity.canonical_name,
      detail: entity.entity_type,
      entity,
      summary: summary ?? existing?.summary ?? entitySummary(entity),
      anchor: Boolean(entity.anchor_kind),
    });
  };

  for (const object of entityObjects) {
    if (object.entity) addEntity(object.entity, object);
  }

  const edges: KnowledgeEdge[] = [];
  for (const object of [...claimObjects].sort((left, right) => left.object_id.localeCompare(right.object_id))) {
    const claim = object.claim;
    if (!claim) continue;
    if (object.subject) addEntity(object.subject, nodes.get(object.subject.entity_id)?.summary);
    if (!nodes.has(claim.subject_entity_id)) {
      nodes.set(claim.subject_entity_id, {
        id: claim.subject_entity_id, kind: "entity", label: shortIdentity(claim.subject_entity_id), detail: "Referenced entity",
      });
    }

    let target: string;
    if (claim.object.entity_id) {
      target = claim.object.entity_id;
      if (object.object_entity) addEntity(object.object_entity, nodes.get(object.object_entity.entity_id)?.summary);
      if (!nodes.has(target)) {
        nodes.set(target, { id: target, kind: "entity", label: shortIdentity(target), detail: "Referenced entity" });
      }
    } else {
      target = `literal:${claim.claim_id}`;
      const literal = claim.object.literal;
      nodes.set(target, {
        id: target,
        kind: "literal",
        label: literal?.value ?? "Unknown value",
        detail: literal?.kind ?? "literal",
      });
    }
    edges.push({
      id: claim.claim_id,
      from: claim.subject_entity_id,
      to: target,
      label: claim.predicate.label || claim.predicate.token,
      polarity: claim.polarity,
      summary: object,
      parallelIndex: 0,
      parallelCount: 1,
    });
  }

  const parallelGroups = new Map<string, KnowledgeEdge[]>();
  for (const edge of edges) {
    const endpoints = [edge.from, edge.to].sort().join("\u0000");
    parallelGroups.set(endpoints, [...(parallelGroups.get(endpoints) ?? []), edge]);
  }
  for (const group of parallelGroups.values()) {
    group.forEach((edge, index) => {
      edge.parallelIndex = index;
      edge.parallelCount = group.length;
    });
  }

  const orderedNodes = [...nodes.values()].sort((left, right) => {
    const anchorDelta = Number(Boolean(right.anchor)) - Number(Boolean(left.anchor));
    const kindDelta = (left.kind === "entity" ? 0 : 1) - (right.kind === "entity" ? 0 : 1);
    return anchorDelta || kindDelta || left.label.localeCompare(right.label) || left.id.localeCompare(right.id);
  });
  return { nodes: orderedNodes, edges };
}

export function parallelEdgeOffset(edge: Pick<KnowledgeEdge, "parallelIndex" | "parallelCount">) {
  return (edge.parallelIndex - (edge.parallelCount - 1) / 2) * 32;
}

export function layoutKnowledgeGraph(graph: KnowledgeGraph): KnowledgeLayout {
  if (graph.nodes.length === 0) return { ...graph, width: canvasWidth, height: canvasHeight, nodes: [] };
  const centerIndex = graph.nodes.findIndex((node) => node.anchor);
  const centerNode = centerIndex >= 0 ? graph.nodes[centerIndex] : graph.nodes[0];
  const orbit = graph.nodes.filter((node) => node.id !== centerNode.id);
  const outerRing = Math.max(0, Math.ceil(orbit.length / 12) - 1);
  const outerRadiusX = 300 + outerRing * 118;
  const outerRadiusY = 205 + outerRing * 84;
  const parallelPadding = graph.edges.reduce((maximum, edge) => Math.max(maximum, Math.abs(parallelEdgeOffset(edge))), 0);
  const horizontalPadding = graph.edges.some((edge) => edge.from === edge.to) ? 140 : 36;
  const width = Math.max(canvasWidth, 2 * (outerRadiusX + nodeWidth / 2 + horizontalPadding));
  const height = Math.max(canvasHeight, 2 * (outerRadiusY + nodeHeight / 2 + 36 + parallelPadding));
  const centerX = width / 2;
  const centerY = height / 2;
  const nodes: KnowledgeLayoutNode[] = [{
    ...centerNode,
    x: centerX - nodeWidth / 2,
    y: centerY - nodeHeight / 2,
    width: nodeWidth,
    height: nodeHeight,
  }];
  for (let index = 0; index < orbit.length; index++) {
    const ring = Math.floor(index / 12);
    const ringStart = ring * 12;
    const ringCount = Math.min(12, orbit.length - ringStart);
    const angle = -Math.PI / 2 + ((index - ringStart) * Math.PI * 2) / Math.max(1, ringCount);
    const radiusX = 300 + ring * 118;
    const radiusY = 205 + ring * 84;
    const node = orbit[index];
    const itemWidth = node.kind === "literal" ? 148 : nodeWidth;
    const itemHeight = node.kind === "literal" ? 52 : nodeHeight;
    nodes.push({
      ...node,
      x: centerX + Math.cos(angle) * radiusX - itemWidth / 2,
      y: centerY + Math.sin(angle) * radiusY - itemHeight / 2,
      width: itemWidth,
      height: itemHeight,
    });
  }
  return { ...graph, nodes, width, height };
}

function shortIdentity(value: string) {
  return value.length > 12 ? `${value.slice(0, 8)}…` : value;
}

function entitySummary(entity: SemanticEntity): SemanticObjectSummary {
  return {
    object_kind: "entity",
    object_id: entity.entity_id,
    scope_key: entity.scope_key,
    status: "active",
    entity,
  };
}
