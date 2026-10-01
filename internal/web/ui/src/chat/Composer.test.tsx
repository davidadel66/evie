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
