export type SystemTab = "map" | "history" | "where";

/** The System view's tabs (system-page.decisions.md, 2026-11-04): the
 *  vision map opens first, on its subway and always dark inside Evie; then
 *  the explorer opened on its History and on its components index. Each tab
 *  is a page and a fragment; tabs on one page share its frame. */
export const systemTabs: readonly { id: SystemTab; label: string; title: string; page: string; fragment: string }[] = [
  { id: "map", label: "Map", title: "System map", page: "/explorer/system.html?theme=dark", fragment: "#subway" },
  { id: "history", label: "History", title: "System history", page: "/explorer/?embedded=1", fragment: "#view=history" },
  { id: "where", label: "Where is it?", title: "System explorer", page: "/explorer/?embedded=1", fragment: "#view=components" },
];

/** The index of the tab a key moves to from the tab at index at: the arrows
 *  step and wrap, Home and End jump to the ends. Any other key moves
 *  nowhere. */
export function systemTabAfterKey(at: number, key: string): number | undefined {
  const last = systemTabs.length - 1;
  return key === "ArrowRight" ? (at + 1) % systemTabs.length
    : key === "ArrowLeft" ? (at + last) % systemTabs.length
    : key === "Home" ? 0
    : key === "End" ? last
    : undefined;
}
