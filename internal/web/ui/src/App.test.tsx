import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";
import { TopBar } from "./App";

describe("TopBar", () => {
  it("renders every tab as a keyboard-focusable semantic button", () => {
    const html = renderToStaticMarkup(
      <TopBar tab="system" onTab={() => undefined} textSize="default" onTextSize={() => undefined} />,
    );
    for (const label of ["Chat", "Whiteboard", "Reports", "System"]) {
      expect(html).toContain(`<button type="button"`);
      expect(html).toContain(`>${label}</span>`);
    }
    expect(html).toContain(`aria-current="page"`);
  });
});
