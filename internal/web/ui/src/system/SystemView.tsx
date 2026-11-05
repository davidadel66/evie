import { useEffect, useId, useRef, useState, type KeyboardEvent } from "react";
import { systemTabAddress, systemTabAfterKey, systemTabs, type SystemTab } from "./systemTabs";

// The System view (system-page.spec.md): the system map page, built into the
// binary and served at /explorer/, full height inside Evie. Its Map,
// Architecture and History are views of that one page, in one frame with
// scripts and popups but not same-origin, so it cannot use Evie's API or
// storage; popups escape the sandbox so History's GitHub links open
// normally.

export function SystemView() {
  // Like Data, the view is not remembered: it opens on the map every time.
  // visit counts tab choices, so choosing the selected tab again brings the
  // page back to that tab's view after the reader moved it from inside.
  const [{ tab, visit }, setChoice] = useState<{ tab: SystemTab; visit: number }>({ tab: "map", visit: 0 });
  return <SystemViewTabs tab={tab} visit={visit} onTab={(next) => setChoice((now) => ({ tab: next, visit: now.visit + 1 }))} />;
}

export function SystemViewTabs({ tab, visit, onTab }: { tab: SystemTab; visit: number; onTab: (tab: SystemTab) => void }) {
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
        <SystemFrame src={systemTabAddress(current.id)} visit={visit} title={current.title} />
      </div>
    </main>
  );
}

/** The page's one frame. It opens at the first tab's address; every later
 *  tab choice moves only the fragment, in place, so the page neither
 *  reloads nor adds to the browser's history, and routes to that view
 *  itself. The frame is cross-origin to Evie, and location.replace is one
 *  of the few things Evie may still do to it. */
function SystemFrame({ src, visit, title }: { src: string; visit: number; title: string }) {
  const frame = useRef<HTMLIFrameElement>(null);
  const [opened] = useState(src);
  const shown = useRef({ src, visit });
  useEffect(() => {
    if (src === shown.current.src && visit === shown.current.visit) return;
    shown.current = { src, visit };
    frame.current?.contentWindow?.location.replace(src);
  }, [src, visit]);
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
