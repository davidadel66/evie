import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";
import { AssistantMessage, DiscardWarning } from "./Message";

const warning = "Response interrupted; streamed text was not saved.";

describe("discarded response presentation", () => {
  it("renders the exact warning inline below partial assistant text", () => {
    const html = renderToStaticMarkup(
      <AssistantMessage
        text="partial answer"
        streaming={false}
        discarded={{ reason: "lease_lost", message: warning }}
      />,
    );
    expect(html).toContain("partial answer");
    expect(html).toContain(warning);
    expect(html.indexOf("partial answer")).toBeLessThan(html.indexOf(warning));
  });

  it("renders the exact standalone warning for reasoning-only output", () => {
    const html = renderToStaticMarkup(<DiscardWarning message={warning} />);
    expect(html).toContain(warning);
    expect(html).toContain('role="status"');
  });
});
