// The open chat's skills, which David invokes with /name (skills.spec.md,
// Owner invocation): every skill the chat pinned, owner-only ones included.

export type ChatSkill = {
  name: string;
  description: string;
  argumentHint?: string;
  /** Hidden from Evie's catalog; only David invokes it. */
  ownerOnly: boolean;
};

export async function listSkills(sessionId: string, signal?: AbortSignal): Promise<ChatSkill[]> {
  const response = await fetch("/api/skills/list", {
    method: "POST", headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ sessionId }), signal,
  });
  const value = await response.json();
  if (!response.ok) throw new Error(value.error ?? "This chat's skills could not be loaded.");
  return Array.isArray(value.skills) ? value.skills : [];
}
