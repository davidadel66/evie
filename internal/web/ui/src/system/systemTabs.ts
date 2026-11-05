export type SystemTab = "map" | "history" | "where";

/** The System view's tabs (system-page.decisions.md, 2026-11-04): the
 *  vision map opens first, on its subway and always dark inside Evie; then
 *  the explorer opened on its History and on its components index. */
export const systemTabs: readonly { id: SystemTab; label: string; title: string; src: string }[] = [
  { id: "map", label: "Map", title: "System map", src: "/explorer/system.html?theme=dark#subway" },
  { id: "history", label: "History", title: "System history", src: "/explorer/?embedded=1#view=history" },
  { id: "where", label: "Where is it?", title: "System explorer", src: "/explorer/?embedded=1#view=components" },
];

/** The tab a key moves to from tab: the arrows step and wrap, Home and End
 *  jump to the ends. Any other key moves nowhere. */
export function systemTabAfterKey(tab: SystemTab, key: string): SystemTab | undefined {
  const at = systemTabs.findIndex((item) => item.id === tab);
  const last = systemTabs.length - 1;
  const next = key === "ArrowRight" ? (at + 1) % systemTabs.length
    : key === "ArrowLeft" ? (at + last) % systemTabs.length
    : key === "Home" ? 0
    : key === "End" ? last
    : undefined;
  return next === undefined ? undefined : systemTabs[next].id;
}
