import { useEffect, useLayoutEffect, useRef, useState } from "react";
import type { WorkflowCapStatus } from "../api/workflows";
import { ChevronDown } from "../ui/Icon";
import { reviewText, waitingText, workflowSpendLine, workspaceTabs, type WorkspaceTab } from "./workspaceActivity";
import { fitCustomTabs } from "./workspaceTabs";

// The Workspace header under the top bar (workspace-activity.spec.md): the
// tabs, badges for runs and Changes waiting on the owner, and today's
// workflow spend. The Workspace's own tabs (workspace-tabs.spec.md) follow
// the built-in ones; those that do not fit go in More, so the strip never
// scrolls.
// The top bar's breadcrumb already names the Workspace.

const tabGap = 22;

function tabClass(selected: boolean) {
  return `${selected ? "border-teal text-ink" : "text-muted-text hover:text-body border-transparent"} focus-visible:text-ink -mb-px flex flex-none items-center gap-[7px] border-b-2 px-0.5 text-[13px] font-medium whitespace-nowrap focus-visible:outline-none`;
}

type HeaderTab = { id: WorkspaceTab; label: string };

export function WorkspaceHeader({tab, waiting, review = 0, showChanges = false, caps, customTabs = [], onTab, onReload}: {
  tab: WorkspaceTab;
  waiting: number;
  /** Changes waiting on the owner (changes.spec.md story 6a). */
  review?: number;
  /** The Changes tab shows only for a Workspace with a Repository. */
  showChanges?: boolean;
  caps?: WorkflowCapStatus;
  /** The Workspace's own tabs that show, in order (workspace-tabs.spec.md). */
  customTabs?: HeaderTab[];
  onTab: (tab: WorkspaceTab) => void;
  /** Reloads the selected custom tab; offered only while one is selected. */
  onReload?: () => void;
}) {
  const leading = workspaceTabs.filter(({id}) => id !== "settings" && (id !== "changes" || showChanges));
  const settings = workspaceTabs.filter(({id}) => id === "settings");
  const selectedCustom = customTabs.findIndex(({id}) => id === tab);
  const layout = [showChanges, waiting, review, ...customTabs.map(({label}) => label)].join("\u0000");
  const {rowRef, measureRef, visible} = useCustomTabFit(selectedCustom, layout);
  const shown = customTabs.filter((_, i) => !visible || visible.includes(i));
  const overflow = customTabs.filter((_, i) => visible && !visible.includes(i));
  const renderTab = ({id, label}: HeaderTab, fixed: boolean) => {
    const selected = tab === id;
    const badge = id === "runs" ? waiting : id === "changes" ? review : 0;
    return <button key={id} type="button" role="tab" id={`workspace-tab-${id}`} aria-selected={selected} aria-controls="workspace-tab-panel"
      data-fixed-tab={fixed || undefined}
      onClick={() => (selected && !fixed && onReload ? onReload() : onTab(id))}
      className={tabClass(selected)}>
      {label}
      {badge > 0 && <>
        <span aria-hidden="true" title="Waiting on you"
          className="border-amber-hair-strong bg-amber-bg text-amber-ink min-w-4 rounded-full border px-[5px] text-center text-[10.5px] leading-[14px]">{badge}</span>
        <span className="sr-only">, {id === "changes" ? reviewText(badge) : waitingText(badge)}</span>
      </>}
    </button>;
  };
  return <div className="border-hair bg-topbar relative flex h-[42px] flex-none items-stretch gap-6 border-b px-6 sm:px-10">
    <div ref={rowRef} className="flex min-w-0 flex-1 items-stretch gap-[22px]">
      <div role="tablist" aria-label="Workspace sections" className="flex min-w-0 items-stretch gap-[22px]">
        {leading.map((item) => renderTab(item, true))}
        {shown.map((item) => renderTab(item, false))}
        {settings.map((item) => renderTab(item, true))}
      </div>
      {overflow.length > 0 && <MoreTabs tabs={overflow} onTab={onTab} />}
    </div>
    {selectedCustom >= 0 && onReload && <button type="button" onClick={onReload} className="text-faint hover:text-body flex flex-none items-center text-[11.5px]">Reload</button>}
    {caps && <div className={`${caps.reached ? "text-danger" : "text-faint"} hidden items-center text-[11.5px] sm:flex`}>{workflowSpendLine(caps)}</div>}
    {customTabs.length > 0 && <div ref={measureRef} aria-hidden="true" className="invisible absolute top-0 left-0 flex h-0 gap-[22px] overflow-hidden">
      {customTabs.map(({id, label}) => <span key={id} data-custom-tab className={tabClass(false)}>{label}</span>)}
      <span data-more-tab className={tabClass(false)}>More <ChevronDown size={12} /></span>
    </div>}
  </div>;
}

/**
 * useCustomTabFit measures the built-in tabs as shown and every custom tab
 * and the More button in a hidden copy, then keeps the custom tabs that fit
 * the row (fitCustomTabs). Until measured, every tab shows.
 */
function useCustomTabFit(selected: number, layout: string) {
  const rowRef = useRef<HTMLDivElement>(null);
  const measureRef = useRef<HTMLDivElement>(null);
  const [visible, setVisible] = useState<number[]>();
  useLayoutEffect(() => {
    const row = rowRef.current, measure = measureRef.current;
    if (!row || !measure) { setVisible(undefined); return; }
    const width = (element: Element | null) => element?.getBoundingClientRect().width ?? 0;
    const fit = () => {
      const fixedTabs = [...row.querySelectorAll("[data-fixed-tab]")];
      const next = fitCustomTabs({
        custom: [...measure.querySelectorAll("[data-custom-tab]")].map(width),
        fixed: fixedTabs.reduce((sum, element) => sum + width(element), 0) + tabGap * Math.max(0, fixedTabs.length - 1),
        more: width(measure.querySelector("[data-more-tab]")),
        gap: tabGap,
        available: row.clientWidth,
        selected,
      });
      setVisible((current) => current && current.length === next.length && current.every((value, i) => value === next[i]) ? current : next);
    };
    fit();
    const observer = new ResizeObserver(fit);
    observer.observe(row);
    return () => observer.disconnect();
  }, [selected, layout]);
  return {rowRef, measureRef, visible};
}

/** MoreTabs holds the custom tabs that do not fit the header. */
export function MoreTabs({tabs, onTab}: {tabs: HeaderTab[]; onTab: (tab: WorkspaceTab) => void}) {
  const [open, setOpen] = useState(false);
  const ref = useRef<HTMLDivElement>(null);
  useEffect(() => {
    if (!open) return;
    const outside = (event: MouseEvent) => { if (!ref.current?.contains(event.target as Node)) setOpen(false); };
    const escape = (event: KeyboardEvent) => { if (event.key === "Escape") setOpen(false); };
    document.addEventListener("mousedown", outside);
    document.addEventListener("keydown", escape);
    return () => { document.removeEventListener("mousedown", outside); document.removeEventListener("keydown", escape); };
  }, [open]);
  return <div ref={ref} className="relative flex flex-none items-stretch">
    <button type="button" aria-haspopup="menu" aria-expanded={open} onClick={() => setOpen(!open)} className={tabClass(false)}>
      More <ChevronDown size={12} />
    </button>
    {open && <div role="menu" aria-label="More tabs" className="border-hair-input bg-sidebar absolute top-full left-0 z-30 mt-1 min-w-40 rounded-md border py-1 shadow-xl">
      {tabs.map(({id, label}) => <button key={id} type="button" role="menuitem" onClick={() => { setOpen(false); onTab(id); }}
        className="text-body hover:bg-hover block w-full px-3 py-2 text-left text-[13px] whitespace-nowrap">{label}</button>)}
    </div>}
  </div>;
}
