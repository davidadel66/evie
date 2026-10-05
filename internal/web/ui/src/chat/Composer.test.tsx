import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";
import { Composer } from "./Composer";

const base = { value: "", onChange: () => {}, onSend: () => {} };

function stopButton(html: string): string | undefined {
  return html.match(/<button[^>]*aria-label="Stop(?:ping)? turn"[^>]*>/)?.[0];
}

describe("Composer turn controls", () => {
  it("offers Stop only while a turn is streaming", () => {
    const streaming = stopButton(renderToStaticMarkup(<Composer {...base} streaming onStop={() => {}} />));
    expect(streaming).toContain('aria-label="Stop turn"');
    expect(streaming).not.toContain('disabled=""');
    expect(stopButton(renderToStaticMarkup(<Composer {...base} streaming={false} onStop={() => {}} />))).toBeUndefined();
  });

  it("disables Stop while the stop request is in flight", () => {
    const html = renderToStaticMarkup(<Composer {...base} streaming stopping onStop={() => {}} />);
    expect(stopButton(html)).toContain('aria-label="Stopping turn"');
    expect(stopButton(html)).toContain('disabled=""');
    expect(html).toContain("Stopping…");
  });

  it("announces command results below the input", () => {
    const html = renderToStaticMarkup(<Composer {...base} streaming={false} notice={{ tone: "info", text: "Context compacted." }} />);
    expect(html).toContain('role="status"');
    expect(html).toContain("Context compacted.");
    const warning = renderToStaticMarkup(<Composer {...base} streaming={false} notice={{ tone: "warning", text: "Usage: /compact" }} />);
    expect(warning).toContain("text-amber-ink");
    expect(renderToStaticMarkup(<Composer {...base} streaming={false} />)).not.toContain('role="status"');
  });
});

describe("Composer command menu", () => {
  const options = (html: string) => [...html.matchAll(/role="option" aria-selected="(true|false)"[^>]*><span[^>]*>([^<]+)<\/span>/g)].map((m) => [m[2], m[1]]);

  it("offers matching commands for a bare slash prefix with the first highlighted", () => {
    expect(options(renderToStaticMarkup(<Composer {...base} value="/" streaming={false} />))).toEqual([["/model", "true"], ["/compact", "false"]]);
    const narrowed = renderToStaticMarkup(<Composer {...base} value="/co" streaming />);
    expect(options(narrowed)).toEqual([["/compact", "true"]]);
    expect(narrowed).toMatch(/aria-activedescendant="[^"]+-0"/);
  });

  it("stays closed for ordinary text, arguments, a disabled composer or an open picker", () => {
    for (const html of [
      renderToStaticMarkup(<Composer {...base} value="hello /model" streaming={false} />),
      renderToStaticMarkup(<Composer {...base} value="/model now" streaming={false} />),
      renderToStaticMarkup(<Composer {...base} value="/" streaming={false} disabled />),
      renderToStaticMarkup(<Composer {...base} value="/" streaming={false} overlay={<div>picker</div>} />),
    ]) {
      expect(html).not.toContain('role="listbox"');
      expect(html).not.toContain("aria-activedescendant");
    }
  });
});
