# Artifacts

Stage 1 of artifacts: a worker's stored report opens as a document in the
Inspector. Decisions: [artifacts.decisions.md](artifacts.decisions.md).
Evidence: [agent-artifacts.md](../research/agent-artifacts.md).

## Problem Statement

Evie produces work that outlives the chat turn that made it, but David can't
open it. When a chat hands research, exploration or a build to a worker, the
worker writes a full report. Evie stores it and the parent model reads it page
by page through `read_subagent_report`, but the Inspector shows only the
worker's objective, the context it was given and its activity trace. To see
what the worker actually concluded and which sources it checked, David has to
ask Evie to repeat it, and the repeat is the parent's retelling, not the
worker's text.

David wants artifacts in Evie: durable outputs Evie can show him, and that
agents can hand to one another by reference instead of pasting them into
messages. Nothing in Evie is called an artifact yet. Workspace tabs are live
views over data, not kept outputs, and the worker report, the closest thing to
an artifact, has no document view.

## Solution

When David opens a worker in the Inspector, he can switch between its
**Activity** and its **Report**. The Report view shows the exact report the
worker stored, the same bytes the parent model reads, rendered as an inert
document. Evie's own facts about the report sit above it, outside the worker's
text: which kind of worker wrote it, how it ended and why, the sources Evie
verified and the ones it couldn't, notes Evie added, and what the work cost.

This is the first artifact: one stored object with two views, a document for
David and paged, framed text for the parent model. It tests that contract on
real reports before Evie adds any artifact store, artifact tool or new table.

## User Stories

1. As David, I want to open a finished worker's report from the Inspector, so that I can read what it concluded in its own words.
2. As David, I want the report to be the exact text the worker stored, so that what I read is what the parent model read.
3. As David, I want the whole report, not the shortened text the activity trace shows, so that nothing is cut from a long report.
4. As David, I want to switch between a worker's Report and its Activity, so that I can see both what it found and how it got there.
5. As David, I want the Report shown first once a worker has finished with a report, so that the result is what I see when I open it.
6. As David, I want the Activity shown while a worker is still running, so that I can follow it before there is a report.
7. As David, I want a worker that ended partway with a report to show that report and say it is partial, so that partial results aren't hidden.
8. As David, I want a worker that ended without a report to say so and give the reason, so that an empty view doesn't look like a broken one.
9. As David, I want Evie's facts about the report shown apart from the worker's text, so that I can tell what Evie checked from what the worker claimed.
10. As David, I want to see which sources Evie verified the worker actually fetched and which it couldn't verify, so that I know how far to trust each citation.
11. As David, I want notes Evie added about the run shown as Evie's, so that harness remarks aren't mistaken for the worker's findings.
12. As David, I want the worker's own limitations shown as the worker's, so that its self-reported caveats are labelled as such.
13. As David, I want to see the worker kind, its status and why it ended, so that I know what kind of result I'm reading.
14. As David, I want to see what the work cost in tokens and requests, so that I can judge whether delegating was worth it.
15. As David, I want raw HTML or script in a report shown as plain text, so that web content a worker quoted can't run in Evie's page.
16. As David, I want only http and https links to be clickable, so that a report can't hand me a `javascript:` or other unsafe link.
17. As David, I want images in a report not to load, so that opening a report doesn't make requests to sites the worker quoted.
18. As David, I want links in a report to open outside Evie's page, so that following a source doesn't replace my chat.
19. As David, I want only the open chat's workers' reports to open, so that one chat's view never shows another chat's worker output.
20. As David, I want reports stored before Evie checked sources to still open, with their sources marked as unchecked, so that older research stays readable.
21. As David, I want the parent model to keep reading reports exactly as before, so that this view changes nothing about how Evie works.
22. As David, I want reports from research, explore and build workers alike, so that every kind of delegated work has a readable result.
23. As David, I want a report that is unusually long to say plainly how much is not shown, so that I never mistake a cut report for a whole one.
24. As Evie, I want the document view and my paged reads to come from the same stored report, so that David and I never disagree about what a worker said.
25. As a future implementer, I want this view built on the rule that a message points to an artifact rather than carrying it, so that later artifact work extends this instead of replacing it.

## Acceptance Criteria

1. For a parent chat that is open and a worker that chat delegated, the owner report read returns the complete stored report bytes, byte-for-byte equal to the worker's final report event, together with its worker kind, status, reason, sources with their verification state, unverified URLs, child-written limitations, harness notes and usage.
2. The same read for another chat's worker, an unknown execution, or an execution whose parent is not the open chat answers not found, and the store never reads the other session's report.
3. A request for a chat that is not open answers a conflict, as the existing worker routes do.
4. A result recorded before Stage 8 (inline `findings`, plain-string sources) and one recorded before Stage 10 (no `notes`) both load. Their sources show as recorded before verification, and their report text comes from the final report event when one exists.
5. A worker that ended without a final report returns its status and reason with no report text, and the view says the worker produced no report.
6. In the Report view, a report containing `<script>`, `<img src=…>`, an inline `onerror` handler, raw HTML blocks and a `javascript:` link renders all of them as visible text: no script, image or iframe element and no non-http(s) `href` appears in the markup.
7. The provenance (kind, status and reason, verified and unverified sources, notes, usage) renders outside the worker's text, and the worker's text and limitations are labelled as written by the worker.
8. `read_subagent_report` returns the same framed pages for the same execution as before this change.
9. `./scripts/verify-change.sh` passes.

## Implementation Decisions

- **Store: an owner report read.** The subagent store gains a read that takes the open parent session and an execution ID and returns a report document. It matches the execution by ID and parent session, so another session's execution is not found. It loads the report from the final report event of the worker's own session, the same event `read_subagent_report` pages. It returns the full text up to 1 MiB. Anything longer is cut with the number of bytes left out. No current model can write a final message that long, so this is a guard, not a page size. It decodes Stage 8 and pre-Stage 8 results with the existing legacy rules and marks legacy sources as recorded before verification.
- **Owner-view authorization follows the existing worker routes.** The read is for the open chat only, by the same rule the worker list and trace routes use: the chat must be open, and the execution's parent must be that chat. It does not apply the model-side checks that `ReadSubagentReport` adds for the parent model (Task grants, Workspace revision at delegation). Those govern what a model may read, and the trace route already shows David the same text under the open-chat rule (see the decision record).
- **Route.** A new management route beside the worker list and trace routes takes the session and execution IDs and returns the report document as JSON. It uses the same POST, JSON-only and loopback guard, the same 409 when the chat is not open and the same 404 code as the trace route.
- **Report document shape.** Execution ID, worker kind, status, reason, report text, total report bytes, bytes left out, sources (URL, fetched, cited, legacy), unverified URLs, limitations (child-written), notes (harness-written) and usage. Child-session IDs stay internal; child sessions are never listed or opened as chats.
- **UI: Report and Activity views inside the existing worker detail.** The worker detail in the Inspector gains a two-way switch. Report is the default once the worker has ended with a report; Activity is the default while it runs or when it ended without one. This adds no new Inspector kind or panel, so the subagents spec's "no new worker panel" still holds.
- **Report view layout.** Harness-written provenance first: kind, status and reason, usage, verified sources and unverified URLs, then Evie's notes. Then the worker's report, labelled as the worker's text, then the worker's limitations, labelled the same way. A cut report says how many bytes are not shown.
- **Inert Markdown.** The Markdown wrapper, still the only module that knows the renderer, gains an inert mode used only by the Report view:
  - raw HTML is not parsed and shows as text;
  - only http(s) links render as links, and they open outside Evie with no opener or referrer;
  - images are not loaded and render as their alt text linked to their URL when that URL is http(s).

  Chat messages keep their current rendering.
- **Unchanged:** `read_subagent_report`, its framing and paging, the system prompt, the worker list and trace routes, the database schema, Workspace tabs, and the `workspace_tabs` guidance about reports.

## Testing Decisions

Good tests here assert what David or the parent model can observe: the bytes returned, the status codes, the markup produced. They do not assert which internal function was called. The seams below were approved by David on 2026-11-02.

- **Store read on real SQLite** (the eviedb subagent fixtures: `newSubagentAdmission`, a second session, and the legacy-record injection pattern in the budget tests).
  - Catches: a dropped parent-session filter, a report read from the wrong event, a byte mismatch with the paged report, legacy results that fail to decode, and a missing-report state reported as an error.
  - Misses: HTTP status mapping and rendering.
- **HTTP route with the existing fakes** (`fakeSubagents` and `newSubagentServer` in the web package's worker route tests).
  - Catches: answering for a chat that isn't open, passing another session to the reader, wrong status codes, and the guard (POST, JSON, loopback).
  - Misses: the SQL filter, which the store seam covers.
- **UI render of the Report view with a hostile report** (Vitest `renderToStaticMarkup` of the worker detail, as the existing subagent tests do).
  - Catches: script, image, iframe or event-handler markup reaching the page; non-http(s) `href`s; provenance rendered inside the worker's text; the wrong default view.
  - Misses: the fetch wiring. A one-line fetch-stub assertion in the same file can check the request path and body.
- **Prior art:**
  - route authorization: `TestSubagentRoutesReadOnlyTheOpenConversation`;
  - scope refusal: `TestSubagentsOfCallStaysWithinTheParentSession`;
  - legacy records: `TestPreBudgetSubagentRecordsStillLoadAfterMigration`;
  - HTML-as-text: `Panel.test.tsx` and `FileViewer.test.tsx`;
  - the model path stays covered by `TestLongReportIsPagedOnlyForItsParent` and the parent-view framing tests.

## Dependencies

- The subagent Stage 8 and Stage 10 result contracts and the stored final report event, both shipped.
- The owner worker trace view and its open-chat rule (chat-activity.spec.md; subagents.decisions.md, 2026-10-12), shipped.
- No dependency on the skills feature.

## Risks

- **Cross-chat leak.** A route that skipped the open-chat check, or a store read without the parent filter, would show one chat's worker output in another. Contained by the two authorization seams, which test each layer separately.
- **Hostile content in Evie's origin.** Worker reports quote web pages. The chat renderer parses and sanitizes HTML and loads images, which is not safe enough for quoted web text. Contained by the inert mode and the hostile-report render test. Images don't load at all, which also prevents tracking pixels.
- **Owner and model reading different text.** Both read the same final report event. The store seam asserts byte equality with the paged report.
- **Legacy results.** Old record shapes could fail to decode or mislabel sources. Contained by the legacy-injection test for each shape.
- **Very large reports.** Contained by the 1 MiB guard, which states how much is cut.

## Verification

- `./scripts/verify-change.sh` from the repository root.
- Focused while developing:
  - `go test ./internal/eviedb -run Subagent`
  - `go test ./internal/web -run Subagent`
  - `npx --prefix internal/web/ui vitest run --root internal/web/ui src/subagents`
- Manual demonstration:
  1. In a chat with research delegation on, ask Evie to research something with two independent questions.
  2. When the workers finish, open a worker from the chat's activity in the Inspector. It opens on Report: the worker's full text, with verified and unverified sources and usage above it.
  3. Switch to Activity, which shows the trace as before.
  4. Open an older research result from before 2026-10-01. It renders with its sources marked as unchecked.

## Out of Scope

- An artifact store, artifact tables, an `artifact` tool, or a shared artifact reference format. These are later stages.
- Model-written HTML or SVG artifacts, and any rendering through the Workspace tab seal.
- Letting other chats, Workspaces or workers read a worker's report.
- Promoting a report to a Workspace tab or a folder file, and exporting reports.
- Feeding reports into Memory or Automatic Recall. This waits for David's memory redesign.
- Reports of workers owned by workflow runs. Run pages have their own views.
- Any change to the system prompt, including the `workspace_tabs` line that sends "reports" to tabs.
- Stopping, continuing or re-running a worker from the UI.

## Further Notes

- **Direction (from the research, not authorized here).** Messages and artifacts stay distinct:
  - A message is addressed and read once: a request, an answer, a status, a handoff. It carries no authority when it comes from another agent.
  - An artifact is a kept work product: owned by a task or scope, versioned, fetched by reference, and pure data with recorded provenance.

  Agents hand off a short message that points to artifacts. A2A 1.0.0 states the same rule: "Messages SHOULD NOT be used to deliver task outputs." The worker result already follows it: harness fields are the message, and the stored report is the artifact.
- **Later stages, each its own spec or amendment:**
  1. a shared artifact reference format used by the Inspector and tool results;
  2. a SQLite artifact store with immutable versions, a single writer, harness-written provenance and Context-Scope reads, Markdown and JSON first;
  3. SVG and HTML artifacts rendered only through the existing tab seal;
  4. promotion of an artifact to a tab or a folder, as an explicit owner action.

  Tabs stay live views over data; artifacts are kept outputs.
