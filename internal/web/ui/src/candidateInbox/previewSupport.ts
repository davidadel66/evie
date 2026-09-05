import type { BatchPreview, ReviewEffect, ReviewPreview } from "../api/candidateReview";

// This is a rendering-capability check, never an acceptance/authority policy.
// Unknown versions fail closed even if they resemble a supported payload.
export function supportedEffect(effect: ReviewEffect): boolean {
  if (effect.version === "owner-review-effect-v5") return Array.isArray(effect.members) && effect.members.length > 0 && effect.members.every((member) => member.version !== "owner-review-effect-v5" && supportedEffect(member)) && Array.isArray(effect.records) && (!effect.dependencies || Array.isArray(effect.dependencies)) && effect.claims.length === effect.members.length && effect.members.every((member, i) => JSON.stringify(member.claims[0]) === JSON.stringify(effect.claims[i]));
  if (!["owner-review-effect-v1", "owner-review-effect-v2", "owner-review-effect-v3", "owner-review-effect-v4"].includes(effect.version) || effect.claims.length !== 1 || effect.members?.length || effect.records?.length) return false;
  if (effect.version === "owner-review-effect-v1" && (effect.identity || effect.correction)) return false;
  if (effect.version === "owner-review-effect-v2" && effect.correction) return false;
  return true;
}
export function supportedPreview(preview: ReviewPreview, batch = false): boolean {
  const version = /^owner-review-preview-v([1-5])$/.exec(preview.version)?.[1];
  if (!version || (!batch && !!preview.batch_preview_id) || !preview.candidates.length) return false;
  if (preview.action === "reject") return !preview.effect;
  return preview.action === "accept" && !!preview.effect && preview.effect.version === `owner-review-effect-v${version}` && supportedEffect(preview.effect);
}
export function supportedBatch(preview: BatchPreview): boolean {
  return preview.version === "owner-review-batch-v1" && preview.failure_behavior === "atomic_groups_independent_failures; committed_failures_are_not_retried" && preview.groups.length > 0 && preview.groups.every((group) => group.preview.scope_key === preview.scope_key && group.preview.batch_preview_id === preview.preview_id && group.preview.version === "owner-review-preview-v5" && supportedPreview(group.preview, true));
}
