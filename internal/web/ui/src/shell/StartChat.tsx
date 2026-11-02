import { Layers, MessageSquare } from "../ui/Icon";

/** What Chat shows with no chat open: start a chat with no Workspace, or a
 * new Workspace (ui-chat.spec.md, plain chats amendment, 2026-11-02). A
 * Workspace's own chats start from its row in the sidebar. */
export function StartChat({ busy, onNewChat, onNewWorkspace }: { busy: boolean; onNewChat: () => void; onNewWorkspace: () => void }) {
  return (
    <main className="flex min-h-0 flex-1 items-center justify-center overflow-y-auto p-7">
      <div className="w-full max-w-[560px]">
        <h1 className="text-ink text-[22px] font-semibold tracking-[-0.02em]">Start a conversation</h1>
        <p className="text-muted-text mt-2 max-w-[520px] text-[13px] leading-6">A chat uses your global memory. A workspace keeps its own memory, folder and chats.</p>
        <div className="mt-7 flex flex-wrap gap-2">
          <button type="button" disabled={busy} onClick={onNewChat} className="bg-teal text-primary-foreground hover:bg-teal-hover focus-visible:ring-teal flex items-center gap-2 rounded-lg px-4 py-2.5 text-xs font-semibold focus-visible:ring-2 focus-visible:outline-none disabled:opacity-40">
            <MessageSquare size={14} /> New chat
          </button>
          <button type="button" disabled={busy} onClick={onNewWorkspace} className="border-hair-input text-body hover:bg-hover hover:text-ink focus-visible:ring-teal flex items-center gap-2 rounded-lg border px-4 py-2.5 text-xs font-medium focus-visible:ring-2 focus-visible:outline-none disabled:opacity-40">
            <Layers size={14} /> New workspace
          </button>
        </div>
      </div>
    </main>
  );
}
