import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";
import { StartChat } from "./StartChat";

// ui-chat.spec.md, plain chats amendment (2026-11-02): with no chat open,
// the owner starts a chat or a Workspace instead of choosing a scope.
describe("start a chat", () => {
  const render = (busy = false) => renderToStaticMarkup(<StartChat busy={busy} onNewChat={() => {}} onNewWorkspace={() => {}} />);

  it("offers a new chat and a new workspace, not a list of scopes", () => {
    const html = render();
    expect(html).toMatch(/<button[^>]*>(?:(?!<\/button>)[\s\S])*New chat<\/button>/);
    expect(html).toMatch(/<button[^>]*>(?:(?!<\/button>)[\s\S])*New workspace<\/button>/);
    expect(html).not.toContain("Choose where this conversation belongs");
    expect(html).not.toContain("Unscoped");
    expect(html).not.toContain('disabled=""');
  });

  it("waits while a chat is being opened", () => {
    expect(render(true).match(/<button[^>]*disabled=""/g)).toHaveLength(2);
  });
});
