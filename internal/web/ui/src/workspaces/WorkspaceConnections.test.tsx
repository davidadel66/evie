import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";
import type { Connection, ToolReview } from "../api/connections";
import type { Workspace } from "../api/contextSessions";
import { WorkspaceHome } from "./Workspaces";
import { ConnectionsView, type ConnectionsViewProps } from "./WorkspaceConnections";

const square: Connection = { id: "mcp-square", name: "square", url: "https://mcp.squareup.com/mcp", status: "connected" };
const canva: Connection = { id: "mcp-canva", name: "canva", url: "https://mcp.canva.com/mcp", status: "needs_sign_in", status_detail: "the authorization server refused the grant" };

const review: ToolReview = {
  connection: square,
  access_revision: 3,
  tools: [
    { name: "make_api_request_read", description: "Read from the Square API.", input_schema: { type: "object", properties: { service: { type: "string" } } }, hash: "r1", state: "enabled", policy: "allow" },
    { name: "make_api_request", description: "Call any Square API method.", hash: "w1", state: "changed" },
    { name: "get_type_info", description: "Describe a Square type.", hash: "t1", state: "available" },
    { name: "bad name", hash: "b1", state: "unselectable" },
  ],
  no_longer_offered: ["retired_tool"],
};

const noop = () => {};
const props = (overrides: Partial<ConnectionsViewProps> = {}): ConnectionsViewProps => ({
  active: true, connections: [canva, square], open: undefined, review: undefined, reviewProblem: "", busy: false, message: "", problem: "",
  draft: { name: "", url: "", clientId: "", clientSecret: "" },
  onToggle: noop, onConnect: noop, onDisconnect: noop, onEnable: noop, onPolicy: noop, onDraft: noop, onAdd: noop, ...overrides,
});
const view = (overrides: Partial<ConnectionsViewProps> = {}) => renderToStaticMarkup(<ConnectionsView {...props(overrides)} />);

describe("Settings → Connections", () => {
  it("lists each connection with its status and the sign-in action it needs", () => {
    const html = view();
    expect(html).toContain("square");
    expect(html).toContain("Connected");
    expect(html).toContain("Needs sign-in");
    expect(html).toContain("the authorization server refused the grant");
    expect(html).toMatch(/<button[^>]*>Reconnect<\/button>/);
    expect(html).toMatch(/<button[^>]*>Disconnect<\/button>/);
    expect(view({ connections: [{ ...square, status: "not_connected" }] })).toMatch(/<button[^>]*>Connect<\/button>/);
    expect(view({ connections: [] })).toContain("No MCP servers yet");
    expect(view({ connections: undefined })).toContain("Reading connections…");
  });

  it("reviews an open connection's tools for this Workspace", () => {
    const html = view({ open: "square", review });
    expect(html).toMatch(/aria-label="Enable make_api_request_read"[^>]*checked=""/);
    expect(html).toMatch(/<option value="allow" selected="">Allow<\/option>/);
    expect(html).toContain("Changed since review");
    expect(html).toContain("Read from the Square API.");
    expect(html).toMatch(/<details[^>]*><summary[^>]*>Input schema<\/summary>/);
    expect(html).not.toMatch(/aria-label="Enable get_type_info"[^>]*checked=""/);
    expect(html).toMatch(/aria-label="Enable bad name"[^>]*disabled=""/);
    expect(html).toContain("retired_tool");
    // Ask or Allow is offered only for enabled tools.
    expect(html).not.toMatch(/aria-label="Policy for get_type_info"/);
    expect(html).toContain("Allow runs the tool without asking you first");
  });

  it("explains a review it could not load", () => {
    expect(view({ open: "canva", reviewProblem: "the connection is not signed in" })).toContain("the connection is not signed in");
  });

  it("disables every change in an archived Workspace", () => {
    const html = view({ active: false, open: "square", review });
    expect(html).toMatch(/aria-label="Enable make_api_request_read"[^>]*disabled=""/);
    expect(html).toMatch(/<select[^>]*disabled=""/);
    expect(html).toMatch(/<button[^>]*disabled=""[^>]*>Add server<\/button>/);
  });

  it("is a section of the Settings tab", () => {
    const workspace: Workspace = { id: "w", displayName: "CairosKitchen", state: "active", currentRevisionId: "r", createdAt: "", updatedAt: "" };
    const html = renderToStaticMarkup(<WorkspaceHome tab="settings" workspace={workspace} sessions={[]} busy={false} onNewChat={() => {}} onResume={() => {}} />);
    expect(html).toContain('aria-label="Connections"');
  });
});
