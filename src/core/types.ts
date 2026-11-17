// The shapes shared across the core. Everything a plugin or a client touches
// is defined here.

// ---- Events -----------------------------------------------------------------

// What each stored event carries. The event log holds these and nothing else.
export type EventBodies = {
  turn_start: Record<string, never>
  user_message: { text: string }
  // The model's reply: its text and any tool calls it asked for.
  assistant_message: { text: string; toolCalls?: ToolCall[] }
  // The text the model reads for one tool call.
  tool_result: { callId: string; name: string; text: string }
  turn_end: { reason: "completed" | "error"; error?: string }
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

// The only way a plugin adds anything to Evie. Each project has a host of its
// own, so what a plugin adds for one project is not seen in another.
export type Host = {
  // The project this host is for, or null for the general session.
  project: Project | null
  tools: Registry<Tool>
  providers: Registry<Provider>
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
