import { useId, useState, type KeyboardEvent } from "react";
import { systemTabAfterKey, systemTabs, type SystemTab } from "./systemTabs";

// The System view (system-page.spec.md): the system map and the system
// explorer, built into the binary and served at /explorer/, full height
// inside Evie. Every tab is a frame with scripts and popups but not
// same-origin, so its page cannot use Evie's API or storage; popups escape
// the sandbox so GitHub links open normally.

export function SystemView() {
  // Like Data, the view is not remembered: it opens on the map every time.
  const [tab, setTab] = useState<SystemTab>("map");
  return <SystemViewTabs tab={tab} onTab={setTab} />;
}

export function SystemViewTabs({ tab, onTab }: { tab: SystemTab; onTab: (tab: SystemTab) => void }) {
  const id = useId();
  const current = systemTabs.find((item) => item.id === tab) ?? systemTabs[0];
  const onKeyDown = (event: KeyboardEvent<HTMLButtonElement>) => {
    const next = systemTabAfterKey(current.id, event.key);
    if (!next) return;
    event.preventDefault();
    onTab(next);
    (event.currentTarget.parentElement?.children[systemTabs.findIndex((item) => item.id === next)] as HTMLButtonElement | undefined)?.focus();
  };
  return (
    <main className="flex min-h-0 flex-1 flex-col overflow-hidden">
      <header className="border-hair flex min-w-0 flex-none items-end gap-3 border-b px-5 pt-4 sm:gap-8 sm:px-7">
        <div className="pb-3">
          <h1 className="text-ink text-[19px] font-semibold tracking-[-0.02em]">System</h1>
        </div>
        <div role="tablist" aria-label="System views" className="flex min-w-0 self-stretch overflow-x-auto">
          {systemTabs.map((item) => (
            <button
              key={item.id}
              id={`${id}-${item.id}`}
              type="button"
              role="tab"
              aria-selected={item.id === current.id}
              aria-controls={`${id}-panel`}
              tabIndex={item.id === current.id ? 0 : -1}
              onClick={() => onTab(item.id)}
              onKeyDown={onKeyDown}
              className={`${item.id === current.id ? "border-teal text-body" : "border-transparent text-faint hover:text-body"} focus-visible:ring-teal flex-none border-b px-3 text-[11.5px] font-medium focus-visible:ring-2 focus-visible:outline-none sm:px-4`}
            >
              {item.label}
            </button>
          ))}
        </div>
      </header>
      <div id={`${id}-panel`} role="tabpanel" aria-labelledby={`${id}-${current.id}`} className="flex min-h-0 min-w-0 flex-1 flex-col">
        {/* A new frame per tab, so switching never adds to the browser's history. */}
        <iframe
          key={current.id}
          title={current.title}
          src={current.src}
          sandbox="allow-scripts allow-popups allow-popups-to-escape-sandbox"
          referrerPolicy="no-referrer"
          className="bg-app min-h-0 w-full flex-1 border-0"
        />
      </div>
    </main>
  );
}
