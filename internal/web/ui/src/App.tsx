// The shell: top bar, tabs, and whichever tab body is active. Chat is the only
// live tab; Whiteboard and Reports render a notice naming the feature that
// will fill them, so the navigation is honest rather than broken.

import { useEffect, useState } from "react";
import { Panel } from "./artifacts/Panel";
import { Chat } from "./chat/Chat";
import { Composer } from "./chat/Composer";
import { useSession } from "./store/useSession";
import { Banner } from "./ui/Banner";
import { TextSizeMenu } from "./ui/TextSizeMenu";
import {
  defaultChatTextSize,
  resolveChatTextSize,
  type ChatTextSize,
} from "./ui/textSize";

type Tab = "chat" | "board" | "reports";
const textSizeStorageKey = "evie.chatTextSize";

export default function App() {
  const { items, status, queue, problem, send, answer, dismissProblem } = useSession();
  const [tab, setTab] = useState<Tab>("chat");
  const [draft, setDraft] = useState("");
  const [textSize, setTextSize] = useState<ChatTextSize>(loadChatTextSize);
  // The artifacts rail starts collapsed — an empty 620px panel is wasted
  // space. The whiteboard feature owns the other half of this: when artifact
  // events exist, pinning one should setPanelOpen(true) from the stream.
  const [panelOpen, setPanelOpen] = useState(false);

  // Y/N resolve the pending approval, as the design specifies — but only from
  // the Chat tab and never while typing, or "yes" in the composer would
  // approve a file edit on its first keystroke.
  const pending = items.find(
    (it) => it.kind === "tool" && it.approval?.state === "pending",
  );
  const pendingId =
    pending?.kind === "tool" ? pending.approval?.reqId : undefined;

  useEffect(() => {
    if (!pendingId || tab !== "chat") return;
    const onKey = (e: KeyboardEvent) => {
      const el = e.target as HTMLElement | null;
      if (el?.tagName === "TEXTAREA" || el?.tagName === "INPUT") return;
      const k = e.key.toLowerCase();
      if (k === "y") answer(pendingId, true);
      else if (k === "n") answer(pendingId, false);
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [pendingId, tab, answer]);

  useEffect(() => {
    try {
      localStorage.setItem(textSizeStorageKey, textSize);
    } catch {
      // Storage can be unavailable in locked-down browser contexts; the
      // in-memory setting still works for this page load.
    }
  }, [textSize]);

  return (
    <div
      data-chat-size={textSize}
      className="bg-app text-ink flex h-screen flex-col overflow-hidden text-[13px]"
    >
      <TopBar
        tab={tab}
        onTab={setTab}
        textSize={textSize}
        onTextSize={setTextSize}
      />

      {problem && <Banner message={problem} onDismiss={dismissProblem} />}

      {tab === "chat" && (
        <div className="flex min-h-0 flex-1">
          <div className="border-hair flex min-w-0 flex-1 flex-col border-r">
            <Chat
              items={items}
              queued={queue}
              streaming={status === "streaming"}
              onAnswer={answer}
            />
            <Composer
              value={draft}
              onChange={setDraft}
              streaming={status === "streaming"}
              onSend={() => {
                send(draft);
                setDraft("");
              }}
            />
          </div>
          <Panel open={panelOpen} onToggle={() => setPanelOpen(!panelOpen)} />
        </div>
      )}

      {tab === "board" && (
        <Soon
          title="Whiteboard"
          detail="Evie draws here — free-stroke SVG, diagrams and markup, streamed as she writes. Landing with the whiteboard feature."
        />
      )}
      {tab === "reports" && (
        <Soon
          title="Reports"
          detail="Saved dashboards and queries Evie builds and edits. Not yet built."
        />
      )}
    </div>
  );
}

function TopBar({
  tab,
  onTab,
  textSize,
  onTextSize,
}: {
  tab: Tab;
  onTab: (t: Tab) => void;
  textSize: ChatTextSize;
  onTextSize: (value: ChatTextSize) => void;
}) {
  return (
    <div className="border-hair bg-topbar flex h-[46px] flex-none items-center gap-[22px] border-b px-5">
      <span className="text-ink font-sans text-[17px] font-bold tracking-[0.18em]">
        EVIE
      </span>
      <div className="flex h-full items-stretch gap-[2px]">
        <TabButton label="Chat" active={tab === "chat"} onClick={() => onTab("chat")} />
        <TabButton label="Whiteboard" active={tab === "board"} onClick={() => onTab("board")} />
        <TabButton label="Reports" active={tab === "reports"} onClick={() => onTab("reports")} />
      </div>
      <div className="flex-1" />
      <TextSizeMenu value={textSize} onChange={onTextSize} />
    </div>
  );
}

function TabButton({
  label,
  active,
  onClick,
}: {
  label: string;
  active: boolean;
  onClick: () => void;
}) {
  return (
    <div
      onClick={onClick}
      className="flex cursor-pointer items-center px-[14px] font-sans text-[12.5px] font-medium"
    >
      {/* The design underlines the active tab with an offset box-shadow rather
          than a border, so the line sits at the bar's bottom edge. */}
      <span
        className={active ? "text-ink" : "text-faint"}
        style={
          active
            ? { boxShadow: "0 15px 0 -13px var(--color-teal)", padding: "15px 0" }
            : { padding: "15px 0" }
        }
      >
        {label}
      </span>
    </div>
  );
}

function loadChatTextSize(): ChatTextSize {
  try {
    const stored = localStorage.getItem(textSizeStorageKey);
    return resolveChatTextSize(stored);
  } catch {
    return defaultChatTextSize;
  }
}

function Soon({ title, detail }: { title: string; detail: string }) {
  return (
    <div className="flex min-h-0 flex-1 flex-col items-center justify-center gap-3 px-8">
      <span className="text-body-dim font-sans text-[13px] font-semibold">
        {title}
      </span>
      <span className="text-fainter max-w-[420px] text-center font-sans text-xs leading-[1.6]">
        {detail}
      </span>
    </div>
  );
}
