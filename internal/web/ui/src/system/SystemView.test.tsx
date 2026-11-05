import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";
import { DataHubView } from "../data/DataHub";
import { SystemView } from "./SystemView";

const framesOf = (html: string) => html.match(/<iframe[^>]*>/g) ?? [];
const headerOf = (html: string) => html.match(/<header class="([^"]*)"><div class="([^"]*)"><h1 class="([^"]*)">([^<]*)<\/h1>/)?.slice(1);

describe("SystemView", () => {
  it("is one full-height frame of the map page, dark and landing on the map, sandboxed to scripts and popups that escape the sandbox", () => {
    const html = renderToStaticMarkup(<SystemView />);
    const frames = framesOf(html);
    expect(frames).toHaveLength(1);
    const [frame] = frames;
    // No fragment: the page opens on its subway map by itself.
    expect(frame).toContain('src="/explorer/?theme=dark"');
    expect(frame).toContain('title="System map"');
    expect(frame).toContain('sandbox="allow-scripts allow-popups allow-popups-to-escape-sandbox"');
    expect(frame).toContain('referrerPolicy="no-referrer"');
    expect(frame).not.toContain("allow-same-origin");
    expect(frame).toMatch(/class="[^"]*\bflex-1\b/);
  });

  it("draws no tabs of its own, so the page's Map, Architecture and History are the only tab bar", () => {
    const html = renderToStaticMarkup(<SystemView />);
    expect(html).not.toContain('role="tablist"');
    expect(html).not.toContain('role="tab"');
    expect(html).not.toContain("<button");
    for (const label of [">Map<", ">Architecture<", ">History<"]) expect(html).not.toContain(label);
  });

  it("keeps a System header styled like Data's", () => {
    const system = headerOf(renderToStaticMarkup(<SystemView />));
    const data = headerOf(renderToStaticMarkup(<DataHubView source="database" onSource={() => undefined} database={null} memory={null} />));
    expect(system?.[3]).toBe("System");
    expect(data?.[3]).toBe("Data");
    expect(system?.slice(0, 3)).toEqual(data?.slice(0, 3));
  });
});
