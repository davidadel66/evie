import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";
import { SystemView, SystemViewTabs } from "./SystemView";
import { systemTabAfterKey, type SystemTab } from "./systemTabs";

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
  it("opens on the map, dark, at the subway, in a frame sandboxed to scripts and popups that escape the sandbox", () => {
    const html = renderToStaticMarkup(<SystemView />);
    const frame = frameOf(html);
    expect(frame).toContain('src="/explorer/system.html?theme=dark#subway"');
    expect(frame).toContain('title="System map"');
    expect(frame).toContain('sandbox="allow-scripts allow-popups allow-popups-to-escape-sandbox"');
    expect(frame).toContain('referrerPolicy="no-referrer"');
    expect(frame).not.toContain("allow-same-origin");
    expect(frame).toMatch(/class="[^"]*\bflex-1\b/);
    expect(tabsOf(html)).toEqual([
      { label: "Map", selected: true, focusable: true },
      { label: "History", selected: false, focusable: false },
      { label: "Where is it?", selected: false, focusable: false },
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
    ["history", "History", "/explorer/?embedded=1#view=history", "System history"],
    ["where", "Where is it?", "/explorer/?embedded=1#view=components", "System explorer"],
    ["map", "Map", "/explorer/system.html?theme=dark#subway", "System map"],
  ] as [SystemTab, string, string, string][])("frames %s for the %s tab, with the same sandbox", (tab, label, src, title) => {
    const html = renderToStaticMarkup(<SystemViewTabs tab={tab} onTab={() => undefined} />);
    const frame = frameOf(html);
    expect(frame).toContain(`src="${src}"`);
    expect(frame).toContain(`title="${title}"`);
    expect(frame).toContain('sandbox="allow-scripts allow-popups allow-popups-to-escape-sandbox"');
    expect(frame).not.toContain("allow-same-origin");
    expect(tabsOf(html).filter((t) => t.selected)).toEqual([{ label, selected: true, focusable: true }]);
  });

  it("moves between tabs with the arrows, wrapping, and with Home and End", () => {
    expect(systemTabAfterKey("map", "ArrowRight")).toBe("history");
    expect(systemTabAfterKey("history", "ArrowRight")).toBe("where");
    expect(systemTabAfterKey("where", "ArrowRight")).toBe("map");
    expect(systemTabAfterKey("map", "ArrowLeft")).toBe("where");
    expect(systemTabAfterKey("where", "ArrowLeft")).toBe("history");
    expect(systemTabAfterKey("history", "Home")).toBe("map");
    expect(systemTabAfterKey("history", "End")).toBe("where");
    expect(systemTabAfterKey("map", "Enter")).toBeUndefined();
    expect(systemTabAfterKey("map", "a")).toBeUndefined();
  });
});
