import { useEffect, useState } from "react";
import { listSkills, type ChatSkill } from "../api/skills";

/** The open chat's skills for the composer's "/" menu (skills.spec.md, Owner
 *  invocation). A chat pins its skills for life, so they load once per chat;
 *  skills is undefined until they have. */
export function useChatSkills(sessionId?: string) {
  const [state, setState] = useState<{ sessionId: string; skills: ChatSkill[] }>();
  const [problem, setProblem] = useState<string>();

  useEffect(() => {
    setProblem(undefined);
    if (!sessionId) return;
    const abort = new AbortController();
    listSkills(sessionId, abort.signal)
      .then((skills) => setState({ sessionId, skills }))
      .catch((error: unknown) => {
        if (!abort.signal.aborted) setProblem(error instanceof Error ? error.message : "This chat's skills could not be loaded.");
      });
    return () => abort.abort();
  }, [sessionId]);

  return { skills: state && state.sessionId === sessionId ? state.skills : undefined, problem };
}
