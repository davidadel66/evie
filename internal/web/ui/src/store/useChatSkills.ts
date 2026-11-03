import { useEffect, useState } from "react";
import { listSkills, type ChatSkills } from "../api/skills";

// Retry delays after a failed load, the last repeated while it keeps failing.
const retryMs = [1_000, 2_000, 5_000, 10_000, 30_000];

/** The open chat's skills for the composer's "/" menu and /name sends
 *  (skills.spec.md, Owner invocation). A chat pins its skills for life, so
 *  they load once per chat; a failed load is retried with backoff, and
 *  problem says why it failed until one succeeds. skills is undefined until
 *  they have loaded. */
export function useChatSkills(sessionId?: string) {
  const [state, setState] = useState<{ sessionId: string; skills: ChatSkills }>();
  const [failure, setFailure] = useState<{ sessionId: string; problem: string }>();

  useEffect(() => {
    if (!sessionId) return;
    const abort = new AbortController();
    let timer: number | undefined;
    let attempt = 0;
    const load = () => {
      listSkills(sessionId, abort.signal)
        .then((skills) => {
          if (abort.signal.aborted) return;
          setState({ sessionId, skills });
          setFailure(undefined);
        })
        .catch((error: unknown) => {
          if (abort.signal.aborted) return;
          setFailure({ sessionId, problem: error instanceof Error ? error.message : "This chat's skills could not be loaded." });
          timer = window.setTimeout(load, retryMs[Math.min(attempt++, retryMs.length - 1)]);
        });
    };
    load();
    return () => {
      abort.abort();
      window.clearTimeout(timer);
    };
  }, [sessionId]);

  return {
    skills: state && state.sessionId === sessionId ? state.skills : undefined,
    problem: failure && failure.sessionId === sessionId ? failure.problem : undefined,
  };
}
