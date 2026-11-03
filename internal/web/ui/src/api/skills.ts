// The open chat's skills, which David invokes with /name (skills.spec.md,
// Owner invocation): every skill the chat pinned, owner-only ones included.

export type ChatSkill = {
  name: string;
  description: string;
  argumentHint?: string;
  /** Hidden from Evie's catalog; only David invokes it. */
  ownerOnly: boolean;
};

/** The open chat's skills, and whether it can use skills at all: a chat
 *  without the skill capability (one from before skills) is not available,
 *  and its slash text is ordinary text. */
export type ChatSkills = { available: boolean; skills: readonly ChatSkill[] };

export async function listSkills(sessionId: string, signal?: AbortSignal): Promise<ChatSkills> {
  const response = await fetch("/api/skills/list", {
    method: "POST", headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ sessionId }), signal,
  });
  const value = await response.json();
  if (!response.ok) throw new Error(value.error ?? "This chat's skills could not be loaded.");
  return { available: value.available !== false, skills: Array.isArray(value.skills) ? value.skills : [] };
}
