// MCP connections and a Workspace's reviewed MCP tools
// (workspace-mcp-connections.spec.md). evie serve holds every sign-in and
// token; these routes never return a credential.

export type ConnectionStatus = "not_connected" | "connected" | "needs_sign_in" | "unreachable" | "keychain_unavailable";

export type Connection = {
  id: string;
  name: string;
  url: string;
  client_id?: string;
  status: ConnectionStatus;
  status_detail?: string;
};

export type Attempt = { id: string; connection_id: string; authorization_url: string; expires_at: string };

export type ToolState = "enabled" | "changed" | "available" | "unselectable";
export type ToolPolicy = "ask" | "allow";

export type ReviewedTool = {
  name: string;
  description?: string;
  input_schema?: unknown;
  annotations?: unknown;
  hash: string;
  state: ToolState;
  policy?: ToolPolicy;
};

export type ToolReview = {
  connection: Connection;
  access_revision: number;
  tools: ReviewedTool[];
  no_longer_offered?: string[];
  unlisted?: boolean;
  reviewed?: string[];
};

export class ConnectionError extends Error {
  readonly code?: string;
  constructor(message: string, code?: string) {
    super(message);
    this.code = code;
  }
}

async function post<T>(path: string, body: unknown): Promise<T> {
  const response = await fetch(`/api/connections/${path}`, { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify(body), cache: "no-store" });
  const value = await response.json().catch(() => ({}));
  if (!response.ok) throw new ConnectionError(value.error ?? `Connections request failed (${response.status})`, value.code);
  return value as T;
}

export const connectionsAPI = {
  list: async (): Promise<Connection[]> => (await post<{ connections: Connection[] }>("list", {})).connections,
  add: (name: string, url: string, clientId?: string, clientSecret?: string): Promise<Connection> =>
    post("add", { name, url, ...(clientId ? { client_id: clientId } : {}), ...(clientSecret ? { client_secret: clientSecret } : {}) }),
  connect: (name: string): Promise<Attempt> => post("connect", { name }),
  /** Resolves once the owner finishes signing in, or the attempt expires. */
  wait: (attemptId: string): Promise<Connection> => post("wait", { attempt_id: attemptId }),
  disconnect: (name: string): Promise<Connection> => post("disconnect", { name }),
  tools: (workspaceId: string, name: string): Promise<ToolReview> => post("tools", { workspace_id: workspaceId, name }),
  setEnabled: (workspaceId: string, name: string, tool: string, enabled: boolean): Promise<ToolReview> =>
    post("tools/set", { workspace_id: workspaceId, name, tool, enabled }),
  setPolicy: (workspaceId: string, name: string, tool: string, policy: ToolPolicy): Promise<ToolReview> =>
    post("tools/policy", { workspace_id: workspaceId, name, tool, policy }),
};
