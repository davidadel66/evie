import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";
import { AssistantMessage, DiscardWarning, UserMessage } from "./Message";
import { clockTime } from "../ui/time";

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

// The readability amendment (ui-chat.spec.md, 2026-11-02): a user turn shows
// when it was sent, sits clear of the answer above it, and its text is body
// color rather than brighter than Evie's reply.
describe("user message", () => {
  const at = new Date(2026, 10, 2, 14, 14).getTime();

  it("shows the sent time beneath the bubble when the turn started", () => {
    const html = renderToStaticMarkup(<UserMessage text="hello" sentAt={at} />);
    expect(html).toContain(clockTime(at));
    expect(clockTime(at)).toMatch(/2:14/);
  });

  it("adds the date when the turn is from another day", () => {
    expect(clockTime(at, new Date(2026, 10, 2, 18, 0))).toMatch(/^2:14/);
    expect(clockTime(at, new Date(2026, 10, 3, 9, 0))).toMatch(/^Nov 2, 2:14/);
  });

  it("omits the time for a message whose turn has not started", () => {
    const html = renderToStaticMarkup(<UserMessage text="hello" />);
    expect(html).not.toContain("PM");
    expect(html).not.toContain("AM");
  });

  it("separates a later turn from the answer above but not the first", () => {
    expect(renderToStaticMarkup(<UserMessage text="a" first />)).not.toContain("mt-9");
    expect(renderToStaticMarkup(<UserMessage text="b" first={false} />)).toContain("mt-9");
  });

  it("uses the body text color, not a brighter one", () => {
    const html = renderToStaticMarkup(<UserMessage text="a" />);
    expect(html).toContain("text-body");
    expect(html).not.toContain("#d7dcd9");
  });
});

// skills.spec.md (decision 2026-11-03): a message sent with a skill shows
// /name above its text.
describe("user message sent with a skill", () => {
  it("shows the skill's /name label above the text", () => {
    const html = renderToStaticMarkup(<UserMessage text="my plan to open a bakery" skill="grilling" />);
    const label = html.match(/<div class="([^"]*)">\/grilling<\/div>/)?.[1] ?? "";
    expect(label.split(" ")).toEqual(expect.arrayContaining(["font-mono", "text-teal"]));
    expect(html.indexOf("/grilling")).toBeLessThan(html.indexOf("my plan to open a bakery"));
  });

  it("shows only the label for a bare /name", () => {
    const html = renderToStaticMarkup(<UserMessage text="/grilling" skill="grilling" />);
    expect(html.match(/\/grilling/g)).toHaveLength(1);
    expect(html).toMatch(/<div class="[^"]*font-mono[^"]*">\/grilling<\/div>/);
    // Text that only starts with the name is still shown.
    expect(renderToStaticMarkup(<UserMessage text="/grilling now" skill="grilling" />)).toContain("/grilling now");
  });

  it("has no label without a skill", () => {
    expect(renderToStaticMarkup(<UserMessage text="hello" />)).not.toContain("font-mono");
  });
});
