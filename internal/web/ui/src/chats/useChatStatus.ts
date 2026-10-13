import { useEffect, useState } from "react";
import { followChatStatus, type ChatStatus } from "../api/chatStatus";

/** How long to wait before following chat status again after it dropped. */
const RECONNECT_MS = 2000;

/** Every chat's live status by session ID, kept current from the server. */
export function useChatStatus(): Record<string, ChatStatus> {
  const [statuses, setStatuses] = useState<Record<string, ChatStatus>>({});
  useEffect(() => {
    const controller = new AbortController();
    let timer: number | undefined;
    const follow = () => {
      followChatStatus((chats) => setStatuses(Object.fromEntries(chats.map((chat) => [chat.sessionId, chat]))), controller.signal)
        .catch(() => undefined)
        .finally(() => {
          if (!controller.signal.aborted) timer = window.setTimeout(follow, RECONNECT_MS);
        });
    };
    follow();
    return () => {
      controller.abort();
      window.clearTimeout(timer);
    };
  }, []);
  return statuses;
}
