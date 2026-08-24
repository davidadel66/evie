// The message column: prose, tool cards, approval cards, and the waiting
// indicator that covers the silence before the first token.

import { useEffect, useRef } from "react";
import type { Item } from "../store/reducer";
import { ApprovalCard } from "./ApprovalCard";
import { AssistantMessage, DiscardWarning, UserMessage } from "./Message";
import { Reasoning } from "./Reasoning";
import { ToolCard } from "./ToolCard";
import { Waiting } from "./Waiting";

type Props = {
  items: Item[];
  queued: string[];
  streaming: boolean;
  onAnswer: (reqId: string, approve: boolean) => void;
};

export function Chat({ items, queued, streaming, onAnswer }: Props) {
  const scroller = useRef<HTMLDivElement>(null);
  const pinned = useRef(true);

  // Follow the stream only while David is already at the bottom. Scrolling up
  // to read something must not be yanked back by the next token.
  useEffect(() => {
    const el = scroller.current;
    if (el && pinned.current) el.scrollTop = el.scrollHeight;
  }, [items, queued, streaming]);

  return (
    <div
      ref={scroller}
      onScroll={(e) => {
        const el = e.currentTarget;
        pinned.current = el.scrollHeight - el.scrollTop - el.clientHeight < 40;
      }}
      className="flex flex-1 flex-col gap-4 overflow-y-auto px-7 pt-5 pb-2"
    >
      {items.length === 0 && <Empty />}
      {items.map((item) => {
        switch (item.kind) {
          case "user":
            return <UserMessage key={item.key} text={item.text} />;
          case "assistant":
            return (
              <AssistantMessage
                key={item.key}
                text={item.text}
                streaming={item.streaming}
                discarded={item.discarded}
              />
            );
          case "notice":
            return <DiscardWarning key={item.key} message={item.text} />;
          case "reasoning":
            return <Reasoning key={item.key} item={item} />;
          case "tool":
            return <ToolItem key={item.key} tool={item} onAnswer={onAnswer} />;
        }
      })}
      {queued.map((text, i) => (
        // Dimmed + marked: the server hasn't seen these yet, and the
        // transcript shouldn't pretend otherwise.
        <div key={`q-${i}`} className="flex flex-none flex-col items-end gap-[2px] opacity-50">
          <UserMessage text={text} />
          <span className="text-ghost font-mono text-[10px]">queued</span>
        </div>
      ))}
      {showWaiting(items, streaming) && <Waiting />}
      <div className="h-2 flex-none" />
    </div>
  );
}

/** ToolItem renders a tool call in whichever of its three states it's in: an
 *  ungated call is just the card; a pending gate is only the approval (there's
 *  no result to show yet); once answered, the approval record stays and the
 *  card joins it if the call actually ran. */
function ToolItem({
  tool,
  onAnswer,
}: {
  tool: Extract<Item, { kind: "tool" }>;
  onAnswer: (reqId: string, approve: boolean) => void;
}) {
  if (!tool.approval) return <ToolCard tool={tool} />;
  if (tool.approval.state === "pending") {
    return <ApprovalCard tool={tool} onAnswer={onAnswer} />;
  }
  return (
    <div className="flex flex-col gap-2">
      <ApprovalCard tool={tool} onAnswer={onAnswer} />
      {tool.result !== undefined && <ToolCard tool={tool} />}
    </div>
  );
}

/** showWaiting is true while the server owes us something but nothing is
 *  visibly in flight: after send with no reply yet, or between a tool result
 *  and the next token. A pending approval is not waiting on Evie — it's
 *  waiting on David, and the card already says so. */
function showWaiting(items: Item[], streaming: boolean): boolean {
  if (!streaming) return false;
  const last = items[items.length - 1];
  if (!last) return false;
  if (last.kind === "assistant") return !last.streaming;
  // A live reasoning block is already something to read during the wait.
  if (last.kind === "reasoning") return !last.streaming;
  if (last.kind === "tool") return last.approval?.state !== "pending";
  return true; // trailing user message: the turn hasn't produced anything yet
}

function Empty() {
  return (
    <div className="flex flex-1 flex-col items-center justify-center gap-2">
      <span className="text-ghost font-sans text-xs">
        Ask Evie anything — she can read files, run commands, and query your data.
      </span>
    </div>
  );
}
