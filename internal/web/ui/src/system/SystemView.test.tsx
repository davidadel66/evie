import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";
import { SystemView, SystemViewTabs } from "./SystemView";
import { systemPage, systemTabAddress, systemTabAfterKey, systemTabs, type SystemTab } from "./systemTabs";

const frameOf = (html: string) => {
  const frames = html.match(/<iframe[^>]*>/g) ?? [];
  expect(frames).toHaveLength(1);
  return frames[0];
};

const tabsOf = (html: string) =>
  [...html.matchAll(/<button[^>]*role="tab"[^>]*>([^<]*)<\/button>/g)].map(([button, label]) => ({
    label,
    selected: button.includes('aria-selected="true"'),
    focusable: button.includes('tabindex="0"'),
  }));

describe("SystemView", () => {
  it("opens on the map page's subway, dark, in a frame sandboxed to scripts and popups that escape the sandbox", () => {
    const html = renderToStaticMarkup(<SystemView />);
    const frame = frameOf(html);
    expect(frame).toContain('src="/explorer/?theme=dark#subway"');
    expect(frame).toContain('title="System map"');
    expect(frame).toContain('sandbox="allow-scripts allow-popups allow-popups-to-escape-sandbox"');
    expect(frame).toContain('referrerPolicy="no-referrer"');
    expect(frame).not.toContain("allow-same-origin");
    expect(frame).toMatch(/class="[^"]*\bflex-1\b/);
    expect(tabsOf(html)).toEqual([
      { label: "Map", selected: true, focusable: true },
      { label: "Architecture", selected: false, focusable: false },
      { label: "History", selected: false, focusable: false },
    ]);
  });

  it("names its tab list and ties the panel to the selected tab", () => {
    const html = renderToStaticMarkup(<SystemView />);
    expect(html).toContain('role="tablist" aria-label="System views"');
    const tab = html.match(/<button[^>]*id="([^"]+)"[^>]*aria-selected="true"[^>]*aria-controls="([^"]+)"/);
    expect(tab).not.toBeNull();
    const [, tabId, panelId] = tab!;
    expect(html).toContain(`id="${panelId}" role="tabpanel" aria-labelledby="${tabId}"`);
  });

  it.each([
    ["map", "Map", "/explorer/?theme=dark#subway", "System map"],
    ["architecture", "Architecture", "/explorer/?theme=dark#view=architecture", "System architecture"],
    ["history", "History", "/explorer/?theme=dark#view=history", "System history"],
  ] as [SystemTab, string, string, string][])("opens %s at the map page's own view for it, with the same sandbox", (tab, label, src, title) => {
    const html = renderToStaticMarkup(<SystemViewTabs tab={tab} visit={0} onTab={() => undefined} />);
    const frame = frameOf(html);
    expect(frame).toContain(`src="${src}"`);
    expect(frame).toContain(`title="${title}"`);
    expect(frame).toContain('sandbox="allow-scripts allow-popups allow-popups-to-escape-sandbox"');
    expect(frame).not.toContain("allow-same-origin");
    expect(tabsOf(html).filter((t) => t.selected)).toEqual([{ label, selected: true, focusable: true }]);
  });

  it("puts every tab on the one map page, so a tab moves only the page's fragment and never reloads it", () => {
    // The page routes its own views by fragment (#subway, #view=architecture,
    // #view=history), so the frame's address differs between tabs only after
    // the #.
    expect(systemPage).toBe("/explorer/?theme=dark");
    for (const tab of systemTabs) {
      const address = systemTabAddress(tab.id);
      expect(address.slice(0, address.indexOf("#"))).toBe(systemPage);
    }
    expect(systemTabs.map((tab) => systemTabAddress(tab.id))).toEqual([
      "/explorer/?theme=dark#subway",
      "/explorer/?theme=dark#view=architecture",
      "/explorer/?theme=dark#view=history",
    ]);
  });

  it("moves between tabs with the arrows, wrapping, and with Home and End", () => {
    const at = (id: SystemTab) => systemTabs.findIndex((item) => item.id === id);
    const after = (id: SystemTab, key: string) => {
      const next = systemTabAfterKey(at(id), key);
      return next === undefined ? undefined : systemTabs[next].id;
    };
    expect(after("map", "ArrowRight")).toBe("architecture");
    expect(after("architecture", "ArrowRight")).toBe("history");
    expect(after("history", "ArrowRight")).toBe("map");
    expect(after("map", "ArrowLeft")).toBe("history");
    expect(after("history", "ArrowLeft")).toBe("architecture");
    expect(after("architecture", "Home")).toBe("map");
    expect(after("architecture", "End")).toBe("history");
    expect(after("map", "Enter")).toBeUndefined();
    expect(after("map", "a")).toBeUndefined();
  });
});
