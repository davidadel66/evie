export type SystemTab = "map" | "architecture" | "history";

/** The System view's one page (system-page.decisions.md, 2026-11-05): the
 *  map the vision skill builds, with its Architecture and History, always
 *  dark inside Evie. */
export const systemPage = "/explorer/?theme=dark";

/** The System view's tabs: each opens the page's own view for it by
 *  fragment, as the page routes them, so every tab shares one frame. */
export const systemTabs: readonly { id: SystemTab; label: string; title: string; fragment: string }[] = [
  { id: "map", label: "Map", title: "System map", fragment: "#subway" },
  { id: "architecture", label: "Architecture", title: "System architecture", fragment: "#view=architecture" },
  { id: "history", label: "History", title: "System history", fragment: "#view=history" },
];

/** The address that opens a tab's view of the page. */
export function systemTabAddress(id: SystemTab): string {
  return systemPage + (systemTabs.find((item) => item.id === id) ?? systemTabs[0]).fragment;
}

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
