// The shapes shared across the core. Everything a plugin or a client touches
// is defined here.

// ---- Events -----------------------------------------------------------------

// What each stored event carries. The event log holds these and nothing else.
export type EventBodies = {
  turn_start: Record<string, never>
  user_message: { text: string }
  // The model's reply: its text and any tool calls it asked for.
  // `interrupted` marks a reply that was cut short by a cancel.
  assistant_message: { text: string; toolCalls?: ToolCall[]; interrupted?: true }
  // The text the model reads for one tool call.
  tool_result: { callId: string; name: string; text: string }
  turn_end: { reason: "completed" | "error" | "cancelled"; error?: string }
}

export type StoredEventType = keyof EventBodies

// One row of the event log. `seq` counts up from 1 within a session.
export type StoredEvent = {
  [Type in StoredEventType]: {
    type: Type
    session: string
    seq: number
    time: number
    body: EventBodies[Type]
  }
}[StoredEventType]

// A piece of the model's reply as it streams. Sent to clients, never stored:
// the complete reply is stored as one `assistant_message`.
export type DeltaEvent = { type: "assistant_delta"; session: string; text: string }

export type EvieEvent = StoredEvent | DeltaEvent

// ---- The request to a model ---------------------------------------------------

// One request from the model to run a tool. `arguments` is the input as the
// model wrote it: JSON text that has not been checked yet.
export type ToolCall = { id: string; name: string; arguments: string }

export type Message =
  | { role: "user"; text: string }
  | { role: "assistant"; text: string; toolCalls?: ToolCall[] }
  | { role: "tool"; callId: string; text: string }

export type ToolDefinition = {
  name: string
  description: string
  inputSchema: Record<string, unknown>
}

export type Request = {
  model: string
  // The system prompt. The core sets none; a plugin adds it in a hook.
  system?: string
  messages: Message[]
  tools: ToolDefinition[]
}

// ---- Providers ----------------------------------------------------------------

export type Usage = { inputTokens: number; outputTokens: number }

// What a provider streams back: pieces of text as they are written, each
// tool call once it is complete, and one finish chunk at the end. A request
// that fails is not thrown: it ends with a finish chunk whose reason is
// "error".
export type Chunk =
  | { type: "text"; text: string }
  | { type: "tool_call"; id: string; name: string; arguments: string }
  | { type: "finish"; reason: "stop" | "tool_calls" | "length"; usage: Usage }
  | { type: "finish"; reason: "error"; error: string; usage: Usage }

// How the core reaches a model: a request and the turn's cancel signal go in,
// a stream of chunks comes out.
export type Provider = {
  name: string
  stream(request: Request, signal: AbortSignal): AsyncIterable<Chunk>
}

// ---- Tools --------------------------------------------------------------------

// What a tool is given when it runs.
export type ToolContext = {
  // The turn's cancel signal.
  signal: AbortSignal
  // The folder of the session's project, or null in the general session,
  // which has no project.
  folder: string | null
}

// Every tool has this one shape, built in or added by someone else. `run`
// returns the text the model will read. A failure the model can act on is
// returned as that text, worded so the model knows what to do next; `run`
// throws only for a bug.
export type Tool = ToolDefinition & {
  run(input: Record<string, unknown>, context: ToolContext): Promise<string>
}

// ---- Plugins ------------------------------------------------------------------

// A list a plugin can add to. Entries keep the order they were added in.
export type Registry<Entry> = {
  add(entry: Entry): void
  list(): readonly Entry[]
}

// ---- Hooks --------------------------------------------------------------------

// What every hook is told about the moment it runs in.
export type HookContext = {
  session: string
  // The session's project, or null in the general session.
  project: Project | null
  // The session's events so far.
  events: readonly StoredEvent[]
  // The turn's cancel signal.
  signal: AbortSignal
}

// A tool call after its input has been checked and repaired.
export type CheckedToolCall = { id: string; name: string; input: Record<string, unknown> }

// The named moments in the loop where a plugin's code runs. Hooks run in the
// order their plugins are listed. A hook may be asynchronous.
export type Hooks = {
  // After a request is built and before it is sent. Return a changed request,
  // or nothing to leave it as it is. The tools left in the request are the
  // only ones the model can call in that step.
  before_request(request: Request, context: HookContext): Request | void | Promise<Request | void>
  // Before a tool runs. Return `{ deny: reason }` to stop it: the tool does
  // not run and the reason is its result. Return nothing to allow it.
  before_tool(call: CheckedToolCall, context: HookContext): { deny: string } | void | Promise<{ deny: string } | void>
  // After a tool has run. Return changed result text, or nothing to leave it.
  after_tool(call: CheckedToolCall, text: string, context: HookContext): string | void | Promise<string | void>
}

// The only way a plugin adds anything to Evie. Each project has a host of its
// own, so what a plugin adds for one project is not seen in another.
export type Host = {
  // The project this host is for, or null for the general session.
  project: Project | null
  tools: Registry<Tool>
  providers: Registry<Provider>
  // Attaches a function to a hook.
  on<Name extends keyof Hooks>(name: Name, handler: Hooks[Name]): void
}

export type Plugin = {
  name: string
  setup(host: Host): void | Promise<void>
}

// ---- Projects -----------------------------------------------------------------

// A project is a record with its own id. Its folder is where its files are,
// and can change; the id is what memory, data and sessions are kept under.
export type Project = { id: string; name: string; folder: string }

// ---- The client interface -------------------------------------------------------

// What each command takes and what it gives back.
export type Commands = {
  // Opens a session. Give a folder to work in the project that folder
  // belongs to (created the first time), or a project id, or neither for the
  // general session, which belongs to no project.
  open_session: {
    input: { project?: string; folder?: string }
    output: { session: string; project: Project | null }
  }
  send_message: {
    input: { session: string; text: string }
    output: void
  }
  // Stops the session's turn in progress. Does nothing when it is idle.
  cancel: {
    input: { session: string }
    output: void
  }
}

export type Command = { [Type in keyof Commands]: { type: Type } & Commands[Type]["input"] }[keyof Commands]

// Everything a client can do: send a command, and subscribe to events. Events
// from every open session arrive on the one subscription, each naming its
// session.
export type Evie = {
  send<Sent extends Command>(command: Sent): Promise<Commands[Sent["type"]]["output"]>
  subscribe(listener: (event: EvieEvent) => void): () => void
  close(): Promise<void>
}
