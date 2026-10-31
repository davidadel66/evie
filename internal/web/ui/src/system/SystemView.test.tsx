import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";
import { SystemView } from "./SystemView";

describe("SystemView", () => {
  it("frames the embedded explorer, sandboxed to scripts and popups that escape the sandbox", () => {
    const html = renderToStaticMarkup(<SystemView />);
    const frame = html.match(/<iframe[^>]*>/)?.[0];
    expect(frame).toBeDefined();
    expect(frame).toContain('src="/explorer/?embedded=1"');
    expect(frame).toContain('sandbox="allow-scripts allow-popups allow-popups-to-escape-sandbox"');
    expect(frame).toContain('title="System explorer"');
    expect(frame).not.toContain("allow-same-origin");
    expect(frame).toMatch(/class="[^"]*\bflex-1\b/);
  });
});
