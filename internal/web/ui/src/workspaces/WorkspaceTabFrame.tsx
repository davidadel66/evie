import { useEffect, useRef, useState } from "react";
import { customTabURL, workspaceTabsAPI, type CustomTab, type TabDataRequest, type WorkspaceTabsAPI } from "../api/workspaceTabs";

/** maxInFlight bounds a tab's unanswered data requests, and maxProblems the problems one page load records. */
const maxInFlight = 4;
const maxProblems = 20;

function newKey(): string {
  const bytes = new Uint8Array(16);
  crypto.getRandomValues(bytes);
  return [...bytes].map((byte) => byte.toString(16).padStart(2, "0")).join("");
}

/**
 * WorkspaceTabFrame shows one of the Workspace's own tabs sealed
 * (workspace-tabs.spec.md): scripts run, but without same-origin, popups,
 * forms or top navigation, and the server's policy adds no network. Its
 * parent remounts it to reload the page, which also makes a new key.
 *
 * Its data bridge (story 2) answers only the page Evie loaded: the page's
 * address carries a one-time key, the helper greets with it once, and every
 * request then travels over a private channel Evie hands back. A page the
 * frame is navigated to later has neither, so it can read nothing. The
 * server still checks every request against the approved data list. The
 * same channel carries the page's problems, which Evie records (story 3).
 */
export function WorkspaceTabFrame({workspaceId, tab, api = workspaceTabsAPI}: {workspaceId: string; tab: CustomTab; api?: WorkspaceTabsAPI}) {
  const frame = useRef<HTMLIFrameElement>(null);
  const [key] = useState(newKey);
  useEffect(() => {
    let channel: MessageChannel | undefined;
    let greeted = false;
    let inFlight = 0;
    let problems = 0;
    const answer = async (message: unknown) => {
      if (!channel || typeof message !== "object" || message === null) return;
      const {id, request, problem} = message as {id?: unknown; request?: unknown; problem?: {kind?: unknown; message?: unknown}};
      if (problem && typeof problem.kind === "string" && typeof problem.message === "string") {
        if (problems++ < maxProblems) api.problem(workspaceId, tab.id, {kind: problem.kind, message: problem.message}).catch(() => undefined);
        return;
      }
      if (typeof id !== "number" || typeof request !== "object" || request === null) return;
      const port = channel.port1;
      if (inFlight >= maxInFlight) {
        port.postMessage({id, ok: false, error: `at most ${maxInFlight} requests at once`});
        return;
      }
      inFlight++;
      try {
        const {result} = await api.data(workspaceId, tab.id, request as TabDataRequest);
        port.postMessage({id, ok: true, result});
      } catch (error) {
        port.postMessage({id, ok: false, error: error instanceof Error ? error.message : "Evie refused the request."});
      } finally {
        inFlight--;
      }
    };
    const greet = (event: MessageEvent) => {
      const data = event.data as {evie?: unknown; key?: unknown} | null;
      if (greeted || event.source !== frame.current?.contentWindow || !data || data.evie !== "tab-hello" || data.key !== key) return;
      greeted = true;
      channel = new MessageChannel();
      channel.port1.onmessage = (reply) => void answer(reply.data);
      (event.source as Window).postMessage({evie: "tab-port"}, "*", [channel.port2]);
    };
    window.addEventListener("message", greet);
    return () => {
      window.removeEventListener("message", greet);
      channel?.port1.close();
    };
  }, [api, workspaceId, tab.id, key]);
  return <iframe ref={frame} title={tab.title} src={`${customTabURL(workspaceId, tab.id)}#evie-key=${key}`}
    sandbox="allow-scripts" referrerPolicy="no-referrer" className="bg-app min-h-0 w-full flex-1 border-0" />;
}
