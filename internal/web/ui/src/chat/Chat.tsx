// The message column: user turns, grouped public activity and final answers.

import { Fragment, useEffect, useRef } from "react";
import type { Item } from "../store/reducer";
import { AssistantMessage, UserMessage } from "./Message";
import { Activity } from "./Activity";
import { activityTurns } from "./activityModel";

type Props = {
  items: Item[];
  queued: string[];
  streaming: boolean;
  onAnswer: (reqId: string, approve: boolean) => void;
  historyLoading?: boolean;
  historyProblem?: string | null;
  hasOlder?: boolean;
  onOlder?: () => void;
  onRetry?: () => void;
};

export function Chat({ items, queued, streaming, onAnswer, historyLoading, historyProblem, hasOlder, onOlder, onRetry }: Props) {
  const scroller = useRef<HTMLDivElement>(null);
  const pinned = useRef(true);
  const prependHeight = useRef<number | null>(null);

  // Follow the stream only while David is already at the bottom. Scrolling up
  // to read something must not be yanked back by the next token.
  useEffect(() => {
    const el = scroller.current;
    if (el && prependHeight.current !== null) {
      el.scrollTop += el.scrollHeight - prependHeight.current;
      prependHeight.current = null;
    } else if (el && pinned.current) el.scrollTop = el.scrollHeight;
  }, [items, queued, streaming]);

  return (
    <div
      ref={scroller}
      onScroll={(e) => {
        const el = e.currentTarget;
        pinned.current = el.scrollHeight - el.scrollTop - el.clientHeight < 40;
      }}
      className="flex flex-1 flex-col gap-4 overflow-y-auto px-4 pt-5 pb-2 sm:px-7"
    >
      {historyProblem && <div role="alert" className="text-amber-ink text-sm">{historyProblem} <button type="button" onClick={onRetry} className="underline">Retry</button></div>}
      {historyLoading && <p className="text-muted-text text-sm">Loading conversation…</p>}
      {hasOlder && <button type="button" disabled={historyLoading || streaming} onClick={() => { pinned.current = false; prependHeight.current = scroller.current?.scrollHeight ?? null; onOlder?.(); }} className="text-teal self-center py-2 text-sm">Earlier messages</button>}
      {items.length === 0 && !historyLoading && !historyProblem && <Empty />}
      {activityTurns(items, streaming).map((group) => <Fragment key={group.key}>
        {group.user && <UserMessage text={group.user.text} />}
        <Activity group={group} onAnswer={onAnswer} />
        {group.answer.map((item) => <AssistantMessage key={item.key} text={item.text} streaming={false} discarded={item.discarded} />)}
      </Fragment>)}
      {queued.map((text, i) => (
        // Dimmed + marked: the server hasn't seen these yet, and the
        // transcript shouldn't pretend otherwise.
        <div key={`q-${i}`} className="flex flex-none flex-col items-end gap-[2px] opacity-50">
          <UserMessage text={text} />
          <span className="text-ghost font-mono text-[10px]">queued</span>
        </div>
      ))}
      <div className="h-2 flex-none" />
    </div>
  );
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
