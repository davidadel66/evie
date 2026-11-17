// The shapes shared across the core. Everything a plugin or a client touches
// is defined here.

// ---- Events -----------------------------------------------------------------

// What each stored event carries. The event log holds these and nothing else.
export type EventBodies = {
  turn_start: Record<string, never>
  user_message: { text: string }
  assistant_message: { text: string }
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

export type Message = { role: "user" | "assistant"; text: string }

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

export type Chunk =
  | { type: "text"; text: string }
  | {
      type: "finish"
      reason: "stop" | "tool_calls"
      usage: { inputTokens: number; outputTokens: number }
    }

// How the core reaches a model: a request and the turn's cancel signal go in,
// a stream of chunks comes out.
export type Provider = {
  name: string
  stream(request: Request, signal: AbortSignal): AsyncIterable<Chunk>
}

// ---- Tools --------------------------------------------------------------------

// What the model is told about a tool. Running a tool arrives with the first
// tool.
export type Tool = ToolDefinition

// ---- Plugins ------------------------------------------------------------------

// A list a plugin can add to. Entries keep the order they were added in.
export type Registry<Entry> = {
  add(entry: Entry): void
  list(): readonly Entry[]
}

// The only way a plugin adds anything to Evie.
export type Host = {
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
