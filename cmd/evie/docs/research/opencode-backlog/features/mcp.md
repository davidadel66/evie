# F21 - Model Context Protocol integration

Status: deferred, unapproved.

## Purpose

Connect EVIE to an external tool/resource server through MCP when that is less
costly than implementing a native Go tool.

## How OpenCode does it

OpenCode supports local stdio and remote HTTP/SSE MCP servers, OAuth, connection
status, reconnects, dynamic tools, prompts, resources, and server instructions.
MCP tools enter the normal registry and permission flow.

Source: [`mcp/index.ts`](https://github.com/anomalyco/opencode/blob/14b37df39168eaf6a6faf862ec4a7bbe9c825bbd/packages/opencode/src/mcp/index.ts).

## EVIE assessment

Do not build MCP "support" without a named server and use case. It introduces a
second tool-discovery/auth/lifecycle boundary and remote instructions that must
not become trusted policy automatically.

## Conditions to promote

- A specific MCP server provides important capability EVIE cannot reasonably
  expose through an existing CLI or small typed tool.
- Its auth, data sensitivity, and availability are understood.
- F04/F05 can wrap dynamic tools with the same identity, output, permission,
  cancellation, and persistence rules as native tools.

## Proposed first slice if promoted

- One configured local stdio server.
- Tools only; no remote OAuth, prompts, resources, or server instructions.
- Namespace every tool by server.
- Explicit allowlist and connection status.
- No automatic install or execution from project content.

## Acceptance evidence

- MCP failure cannot break native tools or session recovery.
- Server-provided descriptions/instructions remain untrusted data.
- Dynamic tools cannot bypass profile policy or output limits.
- Child agents inherit parent MCP denials.
