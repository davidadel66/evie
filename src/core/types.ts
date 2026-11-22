// The shapes shared across the core. Everything a plugin or a client touches
// is defined here.

// ---- Events -----------------------------------------------------------------

// What each stored event carries. The event log holds these and nothing else.
export type EventBodies = {
  turn_start: Record<string, never>
  user_message: { text: string }
  // The model's reply: its text and any tool calls it asked for.
  // `interrupted` marks a reply that was cut short by a cancel. `usage` is
  // the size of the request and the reply, as the provider counted them.
  assistant_message: { text: string; toolCalls?: ToolCall[]; interrupted?: true; usage?: Usage }
  // The text the model reads for one tool call.
  tool_result: { callId: string; name: string; text: string }
  turn_end: { reason: "completed" | "error" | "cancelled"; error?: string }
  // The owner switched model. It applies from the next model request.
  model_changed: { model: string }
  // The conversation before `keepFrom` was replaced by a summary. From here
  // on a request is built from the summary and the events from `keepFrom`
  // onwards. `files` are the files the summarised part worked with.
  compaction: { summary: string; keepFrom: number; files: string[] }
  // Evie asked the owner something. `id` ties the answer to it.
  // `asker` is the session that asked, when it is not this one: a subagent,
  // whose questions are put to the owner in the session that started it.
  question: { id: string; asker?: string } & Question
  // The answer. `choice` is null when the question was refused: the turn was
  // cancelled, Evie closed, or the session's client cannot show questions.
  answer: { id: string; choice: string | null }
  // Something the model is told that neither it nor the owner said, such as
  // that the owner undid its changes. It is shown to the model in place.
  note: { text: string }
  // The owner set the session aside, or with false brought it back. It is
  // not activity in the session, and the model is not shown it.
  settled: { settled: boolean }
}

// Something Evie asks the owner, with the answers to choose from. With
// `freeText` the owner may also answer in their own words.
export type Question = { text: string; choices: string[]; freeText?: true }

// Asks the owner and waits for the answer. It returns the choice, or null
// when the question was refused, which is to be treated as a no.
export type Ask = (question: Question) => Promise<string | null>

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

// A job started or ended. Sent to clients, never stored.
export type JobEvent = { type: "job"; session: string; job: Job }

export type EvieEvent = StoredEvent | DeltaEvent | JobEvent | SummaryEvent

// ---- Sessions in a list ---------------------------------------------------------

// Where a session stands, as a list of sessions shows it: `needs_you` while
// a question in it waits for the owner; `working` while a turn or a
// background job of its own runs; `settled` when neither, and the owner set
// it aside by hand after its last activity, or a week has passed without
// any, counted from when the owner last brought it back by hand if that was
// later; `active` otherwise, idle but recent. It is worked out from the log
// and from what is running each time it is asked for, and never stored.
export type SessionState = "needs_you" | "working" | "active" | "settled"

// One line of a list of sessions. `project` is the id of its project, null
// for a general session. `name` is the opening words of its first message,
// null until it has one. `lastActive` is the time of its latest stored
// event, settling aside, or when it was made if it has none.
export type SessionSummary = { session: string; project: string | null; name: string | null; state: SessionState; lastActive: number }

// A session's summary, sent when it is first listed and whenever its
// state, name or latest activity changes, after the event that changed it.
// Sent to clients, never stored. Evie's server sends it to every client,
// whatever sessions it has open, so that each can keep a list of sessions.
export type SummaryEvent = { type: "summary" } & SessionSummary

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
  // The longest the reply may be, in tokens, when the settings give a limit.
  maxTokens?: number
  // Text that ends the reply the moment the model writes it. The core uses
  // it for the opening of its own notes, which only it may write.
  stop?: string[]
  messages: Message[]
  tools: ToolDefinition[]
}

// ---- Providers ----------------------------------------------------------------

// The size of a request and its reply, in tokens, as the provider counted
// them. `cachedTokens` is how much of the request the provider read from its
// prompt cache, which is charged at a fraction of the usual price, and
// `cacheWriteTokens` how much it stored there for next time. Both are part
// of `inputTokens`, and are given only when the provider reports them.
export type Usage = { inputTokens: number; outputTokens: number; cachedTokens?: number; cacheWriteTokens?: number }

// What a provider streams back: pieces of text as they are written, each
// tool call once it is complete, and one finish chunk at the end. A request
// that fails is not thrown: it ends with a finish chunk whose reason is
// "error".
export type Chunk =
  | { type: "text"; text: string }
  | { type: "tool_call"; id: string; name: string; arguments: string }
  | { type: "finish"; reason: "stop" | "tool_calls" | "length"; usage?: Usage }
  // `tooLong` marks a request refused for being more than the model can read.
  | { type: "finish"; reason: "error"; error: string; usage?: Usage; tooLong?: true }

// How the core reaches a model: a request and the turn's cancel signal go in,
// a stream of chunks comes out.
export type Provider = {
  name: string
  stream(request: Request, signal: AbortSignal): AsyncIterable<Chunk>
  // How many tokens a model can read in one request, when the provider knows.
  contextLimit?(model: string): Promise<number | undefined>
}

// ---- Jobs ---------------------------------------------------------------------

// Work that carries on after the tool call that started it has returned.
// `id` counts up within a session. `outputFile` is where the work may write
// what it prints; the model can read it there.
export type Job = {
  id: string
  session: string
  kind: string
  title: string
  // "unknown" is for work that has ended, but no one can tell how, as for a
  // command that went while no Evie was running without leaving its status.
  status: "running" | "finished" | "failed" | "stopped" | "unknown"
  started: number
  ended?: number
  outputFile: string
  // The session doing the work, for a round of a subagent's: a client can
  // read its conversation with `read_history`.
  subagent?: string
}

// What a tool hands over when it puts work in the background.
export type JobWork = {
  // What sort of work it is, such as "shell".
  kind: string
  // A few words to list it by, such as the command.
  title: string
  // Settles when the work ends, with what the model should be told about
  // how it went. Rejecting means the work failed, and rejecting with an
  // `EndNotKnown` that it ended in a way no one can tell.
  done: Promise<string>
  // Stops the work. `done` must settle soon after.
  stop(): void
  // What the job's record keeps for the work's plugin, to pick the work up
  // again after Evie restarts, such as the id of a subagent.
  keep?: Record<string, unknown>
  // The session doing the work, when it is a subagent's round of work.
  subagent?: string
}

// A job number taken for work a tool call waits on, before it is a job: its
// output file and its record are there from the start, so that if Evie
// restarts while the call waits, the next start finds the work and makes it
// a job then. `start` makes it a job now; `release` gives the number and the
// files back when the work ended within its call.
export type ReservedJob = {
  id: string
  outputFile: string
  // Adds to what the record keeps for the work's plugin, such as the id of
  // the process doing it.
  keep(data: Record<string, unknown>): void
  start(work: JobWork): Job
  release(): void
}

// What is kept of a job in a file beside its output, so that a later run of
// Evie can find it again: written when it starts, and again when it changes.
export type JobRecord = {
  id: string
  session: string
  kind: string
  title: string
  started: number
  outputFile: string
  // The tool call it belongs to. While `inCall` is set, that call is still
  // waiting on the work, which is not yet a job.
  call?: string
  inCall?: true
  status: Job["status"]
  ended?: number
  // Whether the model has been told how it ended, or knows.
  told: boolean
  // What the work's plugin keeps, to pick it up again.
  keep: Record<string, unknown>
}

// How a plugin picks up a job of its own kind that an earlier run of Evie
// left, given the job's record: the work it returns is watched as any job's
// is. `context` is the job's session's, as its tools are given it.
export type JobKind = {
  // The kind, as the plugin's work names it.
  name: string
  resume(job: JobRecord, context: { children: Children }): JobWork
}

// ---- Subagents ----------------------------------------------------------------

// How a tool starts and talks to subagents. A subagent is a child session:
// a conversation of its own, with an empty start, running the same loop in
// the same project. Each is known by the id of its session.
export type Children = {
  // Starts a subagent and sends it its first message. `model` is the model
  // it uses; without one it uses the model this session is using. Fails in
  // a session that is itself a subagent: subagents do not start subagents.
  // With `agent`, it is one of the owner's named agents, which hooks are
  // told. With `readOnly`, it is held to reading for as long as it lives.
  start(input: { text: string; model?: string; agent?: string; readOnly?: boolean }): Promise<string>
  // Sends a further message to one of this session's subagents. It keeps
  // its conversation, whether it is still working or has finished.
  send(child: string, text: string): Promise<void>
  // Settles when the subagent next has nothing left to do, with its last
  // reply and how its last turn ended.
  whenIdle(child: string): Promise<{ reply: string; ended: "completed" | "error" | "cancelled"; error?: string }>
  // Whether the subagent is working now.
  busy(child: string): boolean
  // Stops what the subagent is doing.
  stop(child: string): void
}

// ---- Tools --------------------------------------------------------------------

// What a tool is given when it runs.
export type ToolContext = {
  // The turn's cancel signal.
  signal: AbortSignal
  // The id of the tool call being run.
  callId: string
  // The folder of the session's project, or null in the general session,
  // which has no project.
  folder: string | null
  // Asks the owner a question and waits for the answer.
  ask: Ask
  // The session's background jobs. `start` hands work over to carry on
  // after this call returns: when it ends the model is told, and is woken
  // to deal with the result if the session is idle. `reserve` is for work
  // the call waits on that may become a job, named by the call it belongs to.
  jobs: {
    start(work: JobWork): Job
    reserve(work: { kind: string; title: string; call: string }): ReservedJob
    list(): Job[]
    stop(id: string): Job | undefined
    // For a tool that is waiting on work it could hand over: calls `asked`
    // when the owner asks for whatever the turn is waiting on to go to the
    // background. Returns a function that ends the listening, to be called
    // when the wait is over.
    whileWaiting(asked: () => void): () => void
  }
  // This session's subagents.
  children: Children
}

// Every tool has this one shape, built in or added by someone else. `run`
// returns the text the model will read. A failure the model can act on is
// returned as that text, worded so the model knows what to do next; `run`
// throws only for a bug.
export type Tool = ToolDefinition & {
  run(input: Record<string, unknown>, context: ToolContext): Promise<string>
  // Which end of its output the model is shown when there is too much to
  // show it all: the start, as for a file the model reads on from, or the
  // end, as for a command, whose errors and summary come last. The start
  // when not given. The full output is saved either way.
  longOutput?: "start" | "end"
}

// ---- Plugins ------------------------------------------------------------------

// A list a plugin can add to. Entries keep the order they were added in.
export type Registry<Entry> = {
  add(entry: Entry): void
  list(): readonly Entry[]
}

// ---- Model profiles and commands -------------------------------------------------

// What one family of models is given. Models differ in which tools they
// handle well and how they read a prompt; a profile holds those differences.
export type ModelProfile = {
  name: string
  // The profile covers every model whose name starts with this. When several
  // profiles cover a model, the one with the longest prefix is used; the
  // empty prefix covers every model.
  prefix: string
  // The system prompt for these models.
  prompt: string
  // The tools for changing files that these models are given. A tool that
  // some profile lists and this one does not is left out for these models.
  fileTools: string[]
}

// What a command is given when the owner runs it.
export type SlashContext = {
  session: string
  project: Project | null
  // The model the session is using now.
  model: string
  // Switches the session's model, from its next request.
  setModel(model: string): void
  // Replaces the older part of the conversation with a summary now, and
  // returns a sentence saying what happened.
  compact(): Promise<string>
  // Whether a turn is in progress in the session.
  busy: boolean
  // Tells the model something, by adding a note to the conversation. The
  // model reads it with its next request. During a turn the note is added
  // at the next step, so it never lands between a tool call and its result.
  tell(text: string): void
  // The session's background jobs, to list them and to stop one.
  // `background` asks whatever the turn is waiting on to move to the
  // background, and returns how many things were asked.
  jobs: { list(): Job[]; stop(id: string): Job | undefined; background(): number }
  // The name the owner typed, which differs from the command's own when the
  // command covers it.
  invokedAs: string
  // Sends a message to the session as if the owner had typed it, which
  // starts a turn.
  send(text: string): void
}

// A command the owner types in a client, starting with a slash. It runs at
// once, without a model turn, and returns the text to show.
export type SlashCommand = {
  name: string
  description: string
  run(args: string, context: SlashContext): string | Promise<string>
  // Other names this command answers to, asked about when the owner types a
  // name that is no command's own. It lets a plugin offer names that are
  // not known until then, such as one for each of the owner's skills. A
  // command's own name always wins over a name another command covers.
  covers?(name: string): boolean | Promise<boolean>
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
  // Asks the owner a question and waits for the answer.
  ask: Ask
  // The session that started this one as a subagent, when one did.
  parent?: string
  // For a subagent: the named agent it was started as, when it was one,
  // and whether it is held to reading only.
  agent?: string
  readOnly?: true
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
  profiles: Registry<ModelProfile>
  commands: Registry<SlashCommand>
  jobKinds: Registry<JobKind>
  // Attaches a function to a hook.
  on<Name extends keyof Hooks>(name: Name, handler: Hooks[Name]): void
}

export type Plugin = {
  name: string
  setup(host: Host): void | Promise<void>
  // For a plugin with work of its own over time, such as a backup every
  // hour: called once, when Evie is sure to be the only one running, as the
  // server is once it listens, with every project and a signal that fires
  // when Evie stops. Only the global plugins are started.
  start?(context: { projects: readonly Project[]; signal: AbortSignal }): void
}

// What one project has of its own, on top of the global settings.
export type ProjectSettings = {
  // The model sessions in this project start with, when the project sets one.
  model?: string
  // The longest a reply may be in this project, when the project sets one.
  maxTokens?: number
  // The project's own plugins, set up after the global ones.
  plugins: readonly Plugin[]
  // Global plugins switched off for this project, by name.
  disable?: readonly string[]
  // Things the owner should be told when a session opens in the project.
  notices?: readonly string[]
  // These settings hold for now only, as when the owner has not yet said
  // whether to trust the project. They are not kept: the next session that
  // opens in the project asks for them again.
  forNow?: true
}

// ---- Projects -----------------------------------------------------------------

// A project is a record with its own id. Its folder is where its files are,
// and can change; the id is what memory, data and sessions are kept under.
export type Project = { id: string; name: string; folder: string }

// A project as a list of projects shows it: with the git branch its folder
// is on, read when the list is asked for, and left out when the folder is
// not in a git repository or is on no branch.
export type ListedProject = Project & { branch?: string }

// ---- The client interface -------------------------------------------------------

// What each command takes and what it gives back.
export type Commands = {
  // Opens a session. Give a folder to work in the project that folder
  // belongs to (created the first time), or a project id, or neither for the
  // general session, which belongs to no project.
  // With `continue`, the session that was used last in that project (or
  // the last general session) is reopened, with its conversation, when there
  // is one. `resumed` says whether that happened.
  // Or give `session` to open that session by its id, in its own project.
  // With `after` as well, the session's stored events after that number are
  // returned, which is how a client catches up after its connection dropped.
  // A subagent's session is not opened by itself: its parent is.
  open_session: {
    // `questions` says that this client shows questions and sends back the
    // answers. Without it, every question in the session is refused at once.
    // A session's id is given alone, without a project, a folder or
    // `continue`; the core also refuses the mix from a client that is not
    // type-checked, though it takes `continue: false`, which asks for nothing.
    input:
      | { session: string; after?: number; questions?: boolean; project?: never; folder?: never; continue?: never }
      | { project?: string; folder?: string; continue?: boolean; questions?: boolean; session?: never; after?: never }
    // `notices` are things the owner should be told, such as a project
    // plugin that was not loaded. A session that is continued can be one
    // another client left running: `turnRunning` says whether a turn is
    // running in it, `openQuestions` are the questions it asked that are
    // still waiting for an answer, and `replySoFar` is the text of the reply
    // the model is writing in it now, when it is writing one; the rest of
    // that reply follows as events. `lastSeq` is the number of the last
    // event in the session's log as it was opened (0 when there is none):
    // every event after it is sent to the client as it happens. `jobs` are
    // the session's background jobs, the ended ones included: their events
    // are not stored, so a client that missed some learns here how each stands.
    output: {
      session: string
      project: Project | null
      resumed: boolean
      notices: string[]
      turnRunning: boolean
      openQuestions: EventBodies["question"][]
      lastSeq: number
      jobs: Job[]
      events?: StoredEvent[]
      replySoFar?: string
    }
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
  // Switches the session's model, from its next model request.
  set_model: {
    input: { session: string; model: string }
    output: void
  }
  // Answers a question Evie asked in a `question` event. `choice` is one of
  // its choices, or the owner's own words when it allows that, or null to
  // refuse it.
  answer_question: {
    input: { session: string; id: string; choice: string | null }
    output: void
  }
  // Runs a command a plugin registered, such as /model, and returns the
  // text to show. No model turn is involved.
  run_command: {
    input: { session: string; name: string; args: string }
    output: { text: string }
  }
  // Sets a session aside, or with `settled: false` brings it back, as a
  // `settled` event in its log. It need not be open, but it is not settled
  // while it is working or a question in it waits. What happens in it
  // afterwards makes it active again. Brought back, it settles again after a
  // week of quiet, as it would after activity.
  settle_session: {
    input: { session: string; settled: boolean }
    output: void
  }
  // A page of a session's earlier conversation: its latest stored events
  // numbered below `before`, or its latest of all without it, in order, and
  // whether there are earlier ones, which a page with `before` set to the
  // first one's number reads. A page is 50 events unless `limit` asks for
  // another number, from 1 to 500. The session need not be open, and
  // can be a subagent's.
  read_history: {
    input: { session: string; before?: number; limit?: number }
    output: { events: StoredEvent[]; earlier: boolean }
  }
  // Every project, by name, with the branch each is on.
  list_projects: {
    input: Record<never, never>
    output: ListedProject[]
  }
  // Makes a new project at a folder, given by its full path: the folder
  // itself when it is missing, and a `.evie/evie.json` in it whose
  // `"sync": "private"` backs up the project's `.evie/` folder to a private
  // repository. The project is registered, named after its folder unless
  // `name` is given, and trusted, unless its `.evie/` folder already held
  // something, which its first session then asks about. `madeFolder` says
  // whether the folder was missing, `trusted` whether it is trusted, and
  // `syncs` whether its `.evie/` folder is backed up: it is trusted, and
  // the sync plugin is loaded. A
  // folder that is already a project, has Evie settings of its own, is
  // inside a git repository below its top, or is in Evie's data folder is
  // refused, and nothing is made.
  new_project: {
    input: { folder: string; name?: string }
    output: { project: Project; madeFolder: boolean; trusted: boolean; syncs: boolean }
  }
  // Every session the owner has said something in, or that has asked a
  // question, that is not a subagent's, the latest active first.
  list_sessions: {
    input: Record<never, never>
    output: SessionSummary[]
  }
  // A background job's output so far, as its work wrote it to its output
  // file: from byte `after`, where the last read ended, to the end, or
  // without it from the start. At most the last 64 KiB are given, from the
  // first character that starts in them; `from` and `to` say which bytes
  // of the file the text is, so a `from` past `after` means some were
  // skipped. A character not yet written whole is left for the next read.
  // The jobs are those `open_session` lists: the ones since Evie last started.
  read_job_output: {
    input: { session: string; id: string; after?: number }
    output: { text: string; from: number; to: number }
  }
}

export type Command = { [Type in keyof Commands]: { type: Type } & Commands[Type]["input"] }[keyof Commands]

// Everything a client can do: send a command, and subscribe to events. Events
// from every open session arrive on the one subscription, each naming its
// session, and so do the summaries of every session.
export type Evie = {
  send<Sent extends Command>(command: Sent): Promise<Commands[Sent["type"]]["output"]>
  subscribe(listener: (event: EvieEvent) => void): () => void
  close(): Promise<void>
}

// The core itself, as the program that runs it holds it: the client
// interface, and what only that program does.
export type Core = Omit<Evie, "close"> & {
  // Picks up the work an earlier run of Evie left in the data folder: jobs
  // still running, or that ended with no one there to tell, and turns cut
  // off. Then starts the plugins' own work over time. Called once, by the
  // server, once it is sure to be the only one.
  resumeWork(): Promise<void>
  // Stops Evie for good: turns and background jobs are stopped. With
  // `restart`, nothing is stopped and nothing more is written, so the next
  // start finds the work as it is, as it would after a crash.
  close(options?: { restart?: boolean }): Promise<void>
}
