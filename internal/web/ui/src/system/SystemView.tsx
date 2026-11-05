import { useEffect, useId, useRef, useState, type KeyboardEvent } from "react";
import { systemTabAfterKey, systemTabs, type SystemTab } from "./systemTabs";

// The System view (system-page.spec.md): the system map and the system
// explorer, built into the binary and served at /explorer/, full height
// inside Evie. Each page is a frame with scripts and popups but not
// same-origin, so it cannot use Evie's API or storage; popups escape the
// sandbox so GitHub links open normally.

export function SystemView() {
  // Like Data, the view is not remembered: it opens on the map every time.
  const [tab, setTab] = useState<SystemTab>("map");
  return <SystemViewTabs tab={tab} onTab={setTab} />;
}

export function SystemViewTabs({ tab, onTab }: { tab: SystemTab; onTab: (tab: SystemTab) => void }) {
  const id = useId();
  const at = Math.max(0, systemTabs.findIndex((item) => item.id === tab));
  const current = systemTabs[at];
  const onKeyDown = (event: KeyboardEvent<HTMLButtonElement>) => {
    const next = systemTabAfterKey(at, event.key);
    if (next === undefined) return;
    event.preventDefault();
    onTab(systemTabs[next].id);
    (event.currentTarget.parentElement?.children[next] as HTMLButtonElement | undefined)?.focus();
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
        {/* One frame per page: a tab on another page opens a new frame. */}
        <SystemFrame key={current.page} src={current.page + current.fragment} title={current.title} />
      </div>
    </main>
  );
}

/** One page's frame. It opens at its first tab's address; a later tab on
 *  the same page moves only the fragment, in place, so the page neither
 *  reloads nor adds to the browser's history. The frame is cross-origin to
 *  Evie, and location.replace is one of the few things Evie may still do
 *  to it. */
function SystemFrame({ src, title }: { src: string; title: string }) {
  const frame = useRef<HTMLIFrameElement>(null);
  const [opened] = useState(src);
  const shown = useRef(src);
  useEffect(() => {
    if (src === shown.current) return;
    shown.current = src;
    frame.current?.contentWindow?.location.replace(src);
  }, [src]);
  return (
    <iframe
      ref={frame}
      title={title}
      src={opened}
      sandbox="allow-scripts allow-popups allow-popups-to-escape-sandbox"
      referrerPolicy="no-referrer"
      className="bg-app min-h-0 w-full flex-1 border-0"
    />
  );
}
