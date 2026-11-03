package main

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/davidadel66/evie/internal/agent"
	"github.com/davidadel66/evie/internal/composition"
	"github.com/davidadel66/evie/internal/eviedb"
	"github.com/davidadel66/evie/internal/memory"
	"github.com/davidadel66/evie/internal/openrouter"
	"github.com/davidadel66/evie/internal/plugins"
	"github.com/davidadel66/evie/internal/procedural"
	"github.com/davidadel66/evie/internal/tools"
	"github.com/davidadel66/evie/internal/web"
)

// skillChat is the composed chat seam of skills.spec.md: a real SQLite
// store that can be reopened as after a restart, a temporary procedural
// repository, the owner's skill CLI, the plugin manager with the skills
// plugin, the real web controller and a request-capturing model client
// per opened chat.
type skillChat struct {
	t          *testing.T
	path       string
	db         *sql.DB
	store      *eviedb.Store
	repo       *procedural.Repo
	cli        *skillCLI
	manager    *plugins.Manager
	controller *webContextSessionController
	clients    map[memory.SessionID]*skillChatClient
}

func newSkillChat(t *testing.T) *skillChat {
	t.Helper()
	c := &skillChat{t: t, path: filepath.Join(t.TempDir(), "evie.db"), repo: procedural.New(filepath.Join(t.TempDir(), "procedural"))}
	c.open()
	t.Cleanup(func() { _ = c.db.Close() })
	return c
}

// open opens the store at the same path, as a restarted Evie does, with a
// new plugin manager and web controller over it.
func (c *skillChat) open() {
	c.t.Helper()
	db, err := eviedb.OpenDBAt(c.path)
	if err != nil {
		c.t.Fatal(err)
	}
	c.db, c.store = db, eviedb.NewStore(db)
	c.cli = &skillCLI{t: c.t, db: db, store: c.store, repo: c.repo}
	c.manager = skillsCompositionManager(c.t, c.store, c.repo)
	c.clients = map[memory.SessionID]*skillChatClient{}
	c.controller = newWebContextSessionController(c.store, c.manager, func(session memory.Session, resolved plugins.ResolvedComposition) (*agent.Session, error) {
		client := &skillChatClient{}
		c.clients[session.ID] = client
		holder := memory.LeaseHolderID("web-" + session.ID)
		return agent.NewWithToolset(client, evieTestContextProfile("skills/model"), c.store.BindHistory(session.ID, holder), session.ScopeContext(),
			c.store.BindTurnOwner(session.ID, holder), resolved.Toolset, agent.WithAutomaticMemoryRecall(false), agent.WithSkillFiles(c.repo)), nil
	})
}

func (c *skillChat) restart() {
	c.t.Helper()
	if err := c.db.Close(); err != nil {
		c.t.Fatal(err)
	}
	c.open()
}

// skillsCompositionManager is sessionCompositionManager's plugins plus the
// skills plugin, all enabled.
func skillsCompositionManager(t *testing.T, store *eviedb.Store, repo *procedural.Repo) *plugins.Manager {
	t.Helper()
	manager, err := plugins.NewManager(tools.KernelToolset(), plugins.NewWeb(), plugins.NewFinance(), plugins.NewYouTube(), plugins.NewTodo(testTaskStore(t)),
		plugins.NewSkills(store, repo))
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []plugins.PluginID{plugins.WebPluginID, plugins.FinancePluginID, plugins.YouTubePluginID, plugins.TodoPluginID, plugins.SkillsPluginID} {
		if err := manager.SetEnabled(id, true); err != nil {
			t.Fatal(err)
		}
	}
	return manager
}

// approve proposes a skill folder and approves exactly the version the
// proposal showed, as the owner does, and returns its version hash.
func (c *skillChat) approve(name string, files map[string]string) string {
	c.t.Helper()
	m := versionHashLine.FindStringSubmatch(c.cli.mustRun("skills", "propose", writeSkillFolder(c.t, name, files)))
	if m == nil {
		c.t.Fatalf("proposing %s printed no version hash", name)
	}
	state, err := c.store.SkillState(context.Background(), name)
	if err != nil {
		c.t.Fatal(err)
	}
	var expected int64
	if active, ok := state.Active(); ok {
		expected = active.Version
	}
	c.cli.mustRun("skills", "approve", name, "--hash", m[1], "--expect-version", strconv.FormatInt(expected, 10))
	return m[1]
}

func (c *skillChat) selectChat(selection web.ContextSessionSelection) web.OpenedContextSession {
	c.t.Helper()
	opened, err := c.controller.SelectSession(context.Background(), selection)
	if err != nil {
		c.t.Fatal(err)
	}
	return opened
}

func (c *skillChat) receipt(id memory.SessionID) composition.Receipt {
	c.t.Helper()
	receipt, err := c.store.GetCompositionReceipt(context.Background(), id)
	if err != nil {
		c.t.Fatal(err)
	}
	return receipt
}

// skillRefs are a receipt's skill references as "id@hash".
func skillRefs(receipt composition.Receipt) []string {
	var out []string
	for _, ref := range receipt.Instructions {
		if strings.HasPrefix(ref.ID, "skill.") {
			out = append(out, ref.ID+"@"+ref.SHA256)
		}
	}
	return out
}

// skillChatClient captures every request and, on the first request of a
// turn, makes the tool calls the test queued for it. It answers the
// compactor with a valid summary and keeps its requests apart.
type skillChatClient struct {
	requests    []openrouter.ChatRequest
	compactions []openrouter.ChatRequest
	calls       []openrouter.ToolCall
}

const skillChatSummary = "## Goal / criteria / constraints\nPlan dinner.\n## Current state / completed actions\nAsked.\n## Decisions / discoveries\nNone.\n" +
	"## Durable paths / IDs / artifacts / tool outcomes\nNone.\n## Unresolved questions / blockers / risks\nNone.\n## Next steps\nContinue.\n## User preferences / commitments\nNone."

func (c *skillChatClient) ChatStream(_ context.Context, request openrouter.ChatRequest, _ openrouter.StreamHandlers) (openrouter.ChatResponse, error) {
	if strings.HasPrefix(request.Messages[0].Content, "You are Evie's context compactor.") {
		c.compactions = append(c.compactions, request)
		return openrouter.ChatResponse{Choices: []openrouter.Choice{{Message: openrouter.Message{Role: "assistant", Content: skillChatSummary}}}}, nil
	}
	c.requests = append(c.requests, request)
	message := openrouter.Message{Role: "assistant", Content: "done"}
	if len(c.calls) > 0 && request.Messages[len(request.Messages)-1].Role == "user" {
		message = openrouter.Message{Role: "assistant", ToolCalls: c.calls}
		c.calls = nil
	}
	return openrouter.ChatResponse{Choices: []openrouter.Choice{{Message: message}}}, nil
}

const owlSkill = `---
name: owl
description: Night shift checklist the owner runs himself.
disable-model-invocation: true
---
Check the doors.
`

// A chat pins the Global skills active when it is created, on the web and
// in the REPL; a chat created earlier, or after a skill's deactivation,
// does not have it.
func TestNewChatsPinTheSkillsApprovedWhenTheyStart(t *testing.T) {
	c := newSkillChat(t)
	ctx := context.Background()
	earlier := c.selectChat(web.ContextSessionSelection{Unscoped: true})
	if refs := skillRefs(c.receipt(earlier.Session.ID)); len(refs) != 0 {
		t.Fatalf("a chat created before any approval pins %v", refs)
	}

	grilling := c.approve("grilling", map[string]string{"SKILL.md": grillingSkill, "references/questions.md": "What breaks first?\n"})
	owl := c.approve("owl", map[string]string{"SKILL.md": owlSkill})
	c.approve("retired", map[string]string{"SKILL.md": "---\nname: retired\ndescription: An old procedure.\n---\nDo the old thing.\n"})
	c.cli.mustRun("skills", "deactivate", "retired")

	workspace, err := c.store.RegisterWorkspace(ctx, "Kitchen")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"skill.global.grilling@" + grilling, "skill.global.owl@" + owl}
	for name, selection := range map[string]web.ContextSessionSelection{
		"unscoped":  {Unscoped: true},
		"workspace": {WorkspaceID: workspace.ID, WorkspaceRevision: workspace.CurrentRevisionID},
	} {
		opened := c.selectChat(selection)
		if refs := skillRefs(c.receipt(opened.Session.ID)); !slices.Equal(refs, want) {
			t.Fatalf("%s chat pins %v, want %v", name, refs, want)
		}
	}
	if refs := skillRefs(c.receipt(earlier.Session.ID)); len(refs) != 0 {
		t.Fatalf("approving changed an earlier chat's pins to %v", refs)
	}

	standard, err := c.manager.ResolvePresetContext(ctx, plugins.StandardPresetID)
	if err != nil {
		t.Fatal(err)
	}
	repl := &receiptBoundREPLStore{Store: c.store, composition: standard, resolvePreset: c.manager.ResolvePresetContext}
	session, err := repl.CreateGlobalSessionForChooser(ctx, t.TempDir(), "")
	if err != nil {
		t.Fatal(err)
	}
	stored := c.receipt(session.ID)
	if refs := skillRefs(stored); !slices.Equal(refs, want) {
		t.Fatalf("REPL chat pins %v, want %v", refs, want)
	}
	if created := repl.selection(session).createdComposition; created == nil || !reflect.DeepEqual(created.Receipt, stored) {
		t.Fatal("the REPL's created composition is not the receipt it stored")
	}

	// A chat from before Composition Receipts is bound to Standard when
	// first opened, without skills; worker presets have no skill tool.
	legacy, err := c.store.CreateGlobalSession(ctx)
	if err != nil {
		t.Fatal(err)
	}
	c.selectChat(web.ContextSessionSelection{SessionID: legacy.ID})
	if refs := skillRefs(c.receipt(legacy.ID)); len(refs) != 0 {
		t.Fatalf("a pre-receipt chat pins %v", refs)
	}
	for _, worker := range []plugins.PresetID{plugins.ResearchPresetID, plugins.ExplorePresetID, plugins.BuildPresetID} {
		resolved, err := c.manager.ResolvePresetContext(ctx, worker)
		if err != nil {
			t.Fatal(err)
		}
		if containsSchema(resolved.Toolset, "skill") {
			t.Fatalf("worker preset %s has the skill tool", worker)
		}
	}
	// With the skills plugin disabled a new chat has no skill tool and pins
	// nothing.
	if err := c.manager.SetEnabled(plugins.SkillsPluginID, false); err != nil {
		t.Fatal(err)
	}
	disabled := c.selectChat(web.ContextSessionSelection{Unscoped: true})
	if refs := skillRefs(c.receipt(disabled.Session.ID)); len(refs) != 0 {
		t.Fatalf("a chat without the skills plugin pins %v", refs)
	}
}

// catalogFrame starts the skill catalog message.
const catalogFrame = "Skills approved by the owner for this chat"

// catalogMessages are the request's skill catalog messages, with their
// positions.
func catalogMessages(request openrouter.ChatRequest) (indexes []int, contents []string) {
	for i, message := range request.Messages {
		if strings.HasPrefix(message.Content, catalogFrame) {
			indexes, contents = append(indexes, i), append(contents, message.Content)
		}
	}
	return indexes, contents
}

func (c *skillChat) send(opened web.OpenedContextSession, text string, calls ...openrouter.ToolCall) *skillChatClient {
	c.t.Helper()
	client := c.clients[opened.Session.ID]
	client.calls = calls
	if err := opened.Agent.Send(context.Background(), text, &replEvents{out: io.Discard}, nil); err != nil {
		c.t.Fatal(err)
	}
	return client
}

// A Workspace chat's every request carries one framed user-role catalog of
// its model-invocable skills right after the repository instructions; a
// chat without skills carries none. The system prompt tells a chat with the
// skill tool to load a listed skill that fits.
func TestAChatSeesItsSkillCatalogAfterTheRepositoryInstructions(t *testing.T) {
	c := newSkillChat(t)
	ctx := context.Background()
	none := c.selectChat(web.ContextSessionSelection{Unscoped: true})
	c.approve("grilling", map[string]string{"SKILL.md": grillingSkill, "references/questions.md": "What breaks first?\n"})
	c.approve("owl", map[string]string{"SKILL.md": owlSkill})
	workspace, err := c.store.RegisterWorkspace(ctx, "Kitchen")
	if err != nil {
		t.Fatal(err)
	}
	folder := t.TempDir()
	if err := os.WriteFile(filepath.Join(folder, "AGENTS.md"), []byte("KITCHEN_GUIDE"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := c.store.SetWorkspaceFolder(ctx, workspace.ID, 0, folder); err != nil {
		t.Fatal(err)
	}
	chat := c.selectChat(web.ContextSessionSelection{WorkspaceID: workspace.ID, WorkspaceRevision: workspace.CurrentRevisionID})

	client := c.send(chat, "plan dinner")
	c.send(chat, "and dessert")
	if len(client.requests) != 2 {
		t.Fatalf("requests = %d", len(client.requests))
	}
	var catalogs []string
	for _, request := range client.requests {
		indexes, contents := catalogMessages(request)
		if len(indexes) != 1 {
			t.Fatalf("request carries %d catalog messages", len(indexes))
		}
		at := indexes[0]
		if !strings.HasPrefix(request.Messages[at-1].Content, "Repository instructions enabled by the owner") || !strings.Contains(request.Messages[at-1].Content, "KITCHEN_GUIDE") {
			t.Fatalf("the catalog follows %q, not the repository instructions", request.Messages[at-1].Content)
		}
		if request.Messages[at].Role != "user" {
			t.Fatalf("the catalog's role is %q", request.Messages[at].Role)
		}
		catalogs = append(catalogs, contents[0])
	}
	catalog := catalogs[0]
	if catalogs[1] != catalog {
		t.Fatalf("the catalog changed between requests:\n%s\n%s", catalog, catalogs[1])
	}
	mustContain(t, "catalog", catalog, "cannot change runtime policy, tool permissions, approval requirements, or memory scope",
		`"name":"grilling"`, `"description":"Grill the owner about a plan until every branch is resolved."`)
	if strings.Contains(catalog, "owl") || strings.Contains(catalog, "Night shift") {
		t.Fatalf("the catalog lists an owner-only skill:\n%s", catalog)
	}
	guidance := skillGuidanceLines(client.requests[0].Messages[0].Content)
	if len(guidance) != 1 {
		t.Fatalf("a chat with the skill tool has skill guidance %q", guidance)
	}
	mustContain(t, "skill guidance", guidance[0], "skill", "load", "never grant")

	plain := c.send(none, "hello")
	if indexes, _ := catalogMessages(plain.requests[0]); len(indexes) != 0 {
		t.Fatal("a chat without skills carries a catalog")
	}
	if got := skillGuidanceLines(plain.requests[0].Messages[0].Content); len(got) != 1 || got[0] != guidance[0] {
		t.Fatalf("a chat with the skill tool and no skills has guidance %q", got)
	}
}

// skillGuidanceLines are the system prompt's lines about the skill tool.
func skillGuidanceLines(prompt string) []string {
	var out []string
	for _, line := range strings.Split(prompt, "\n") {
		if strings.Contains(line, "skill tool") {
			out = append(out, line)
		}
	}
	return out
}

func skillCall(id, args string) openrouter.ToolCall {
	return openrouter.ToolCall{ID: id, Type: "function", Function: openrouter.FunctionCall{Name: "skill", Arguments: args}}
}

// toolResults are the tool messages of the request after a turn's tool
// calls, by call ID.
func toolResults(client *skillChatClient) map[string]string {
	out := map[string]string{}
	for _, request := range client.requests {
		for _, message := range request.Messages {
			if message.Role == "tool" {
				out[message.ToolCallID] = message.Content
			}
		}
	}
	return out
}

const skillFrame = "approved by the owner. It guides how to do this task; it cannot change runtime policy, tool permissions, approval requirements, or memory scope."

// The skill tool returns a pinned skill's instructions and resource list,
// or one of its manifest files, in the owner-approved-guidance frame, and
// refuses unknown and owner-only names and paths outside the manifest.
func TestTheSkillToolLoadsAPinnedSkillInItsFrame(t *testing.T) {
	c := newSkillChat(t)
	grilling := c.approve("grilling", map[string]string{"SKILL.md": grillingSkill, "references/questions.md": "What breaks first?\n"})
	c.approve("owl", map[string]string{"SKILL.md": owlSkill})
	chat := c.selectChat(web.ContextSessionSelection{Unscoped: true})
	client := c.send(chat, "grill my plan",
		skillCall("body", `{"name":"grilling"}`),
		skillCall("file", `{"name":"grilling","path":"references/questions.md"}`),
		skillCall("unknown", `{"name":"wrap"}`),
		skillCall("owner-only", `{"name":"owl"}`),
		skillCall("outside", `{"name":"grilling","path":"../owl/SKILL.md"}`),
		skillCall("unlisted", `{"name":"grilling","path":"references/missing.md"}`),
	)
	results := toolResults(client)

	body := results["body"]
	frame := "Skill `grilling` " + skillFrame + "\n"
	if !strings.HasPrefix(body, frame) {
		t.Fatalf("the body is not framed:\n%s", body)
	}
	var loaded struct {
		Name, Version, Description, Instructions string
		Resources                                []struct {
			Path  string
			Bytes int64
		}
	}
	if err := json.Unmarshal([]byte(strings.TrimPrefix(body, frame)), &loaded); err != nil {
		t.Fatalf("the body after its frame is not JSON: %v\n%s", err, body)
	}
	if loaded.Name != "grilling" || loaded.Version != grilling || loaded.Description != "Grill the owner about a plan until every branch is resolved." ||
		loaded.Instructions != "# Grilling\n\nAsk one hard question at a time.\n" || len(loaded.Resources) != 1 ||
		loaded.Resources[0].Path != "references/questions.md" || loaded.Resources[0].Bytes != int64(len("What breaks first?\n")) {
		t.Fatalf("loaded %+v", loaded)
	}

	file := results["file"]
	if !strings.HasPrefix(file, frame) {
		t.Fatalf("the resource is not framed:\n%s", file)
	}
	var read struct{ Name, Version, Path, Content string }
	if err := json.Unmarshal([]byte(strings.TrimPrefix(file, frame)), &read); err != nil ||
		read.Name != "grilling" || read.Version != grilling || read.Path != "references/questions.md" || read.Content != "What breaks first?\n" {
		t.Fatalf("read %+v, %v", read, err)
	}

	for id, want := range map[string]string{
		"unknown":    `skill "wrap" is not one of this chat's skills`,
		"owner-only": `skill "owl" is invoked by David only`,
		"outside":    `"../owl/SKILL.md" is not a file of skill "grilling"`,
		"unlisted":   `"references/missing.md" is not a file of skill "grilling"`,
	} {
		if got := results[id]; !strings.HasPrefix(got, "tool call came back with error ") || !strings.Contains(got, want) {
			t.Errorf("%s = %q, want an error saying %q", id, got, want)
		}
		if strings.Contains(results[id], "Check the doors") {
			t.Errorf("%s leaked an owner-only skill's text", id)
		}
	}
}

// Reopening Evie keeps each chat's pinned versions, even after the owner
// approves a newer version: the chat resumes with its receipt, catalog and
// skill text, while a new chat pins the newer version. A pinned version
// with no approval record fails the chat visibly instead of falling back.
func TestReopeningKeepsAChatsPinnedSkillVersions(t *testing.T) {
	c := newSkillChat(t)
	ctx := context.Background()
	first := c.approve("grilling", map[string]string{"SKILL.md": grillingSkill})
	chat := c.selectChat(web.ContextSessionSelection{Unscoped: true})
	c.send(chat, "hello")
	receipt := c.receipt(chat.Session.ID)

	edited := strings.NewReplacer("one hard question", "two hard questions", "every branch", "every last branch").Replace(grillingSkill)
	second := c.approve("grilling", map[string]string{"SKILL.md": edited})
	if second == first {
		t.Fatal("the edited skill has the same hash")
	}
	c.restart()

	resumed := c.selectChat(web.ContextSessionSelection{SessionID: chat.Session.ID})
	if got := c.receipt(chat.Session.ID); !reflect.DeepEqual(got, receipt) || !slices.Equal(skillRefs(got), []string{"skill.global.grilling@" + first}) {
		t.Fatalf("reopened receipt pins %v", skillRefs(got))
	}
	client := c.send(resumed, "grill it", skillCall("body", `{"name":"grilling"}`))
	_, catalogs := catalogMessages(client.requests[0])
	if len(catalogs) != 1 || !strings.Contains(catalogs[0], "until every branch is resolved") {
		t.Fatalf("the reopened chat's catalog is %q", catalogs)
	}
	if body := toolResults(client)["body"]; !strings.Contains(body, "Ask one hard question") || !strings.Contains(body, first) {
		t.Fatalf("the reopened chat loads %q", body)
	}

	fresh := c.selectChat(web.ContextSessionSelection{Unscoped: true})
	if refs := skillRefs(c.receipt(fresh.Session.ID)); !slices.Equal(refs, []string{"skill.global.grilling@" + second}) {
		t.Fatalf("a new chat pins %v", refs)
	}

	standard, err := c.manager.ResolvePresetContext(ctx, plugins.StandardPresetID)
	if err != nil {
		t.Fatal(err)
	}
	ghost := composition.Clone(standard.Receipt)
	ghost.Instructions = append(ghost.Instructions, composition.InstructionReference{ID: "skill.global.ghost", SHA256: strings.Repeat("a", 64)})
	lost, err := c.store.CreateGlobalSessionWithComposition(ctx, ghost)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.controller.SelectSession(ctx, web.ContextSessionSelection{SessionID: lost.ID}); err == nil || !strings.Contains(err.Error(), "ghost") {
		t.Fatalf("a chat pinning a never-approved skill opened: %v", err)
	}
}

// A chat pinned to the Standard preset version before skills (695e1058…)
// reopens with exactly its tools and prompt: no skill tool, guidance or
// catalog, though skills are approved.
func TestAPreSkillsStandardChatResumesUnchanged(t *testing.T) {
	c := newSkillChat(t)
	ctx := context.Background()
	c.approve("grilling", map[string]string{"SKILL.md": grillingSkill})
	// What a binary before skills stored for this plugin set: the same
	// composition, at the pre-skills version, with no skill capability.
	without, err := sessionCompositionManager(t).ResolvePresetContext(ctx, plugins.StandardPresetID)
	if err != nil {
		t.Fatal(err)
	}
	receipt := composition.Clone(without.Receipt)
	receipt.Preset.Version = "sha256:695e10586716301681141b44b53e41eef1050bef971bf92655957cf65e928339"
	receipt.Warnings = slices.DeleteFunc(receipt.Warnings, func(w composition.Warning) bool { return w.ProviderID == "skills" })
	old, err := c.store.CreateGlobalSessionWithComposition(ctx, receipt)
	if err != nil {
		t.Fatal(err)
	}
	c.restart()

	chat := c.selectChat(web.ContextSessionSelection{SessionID: old.ID})
	if got := c.receipt(old.ID); !reflect.DeepEqual(got, receipt) {
		t.Fatalf("resuming changed the pre-skills receipt: %+v", got)
	}
	request := c.send(chat, "hello").requests[0]
	var tools, pinned []string
	for _, tool := range request.Tools {
		tools = append(tools, tool.Function.Name)
	}
	for _, schema := range receipt.ToolSchemas {
		pinned = append(pinned, schema.Name)
	}
	if !slices.Equal(tools, pinned) || slices.Contains(tools, "skill") {
		t.Fatalf("the pre-skills chat has tools %v, pinned %v", tools, pinned)
	}
	if indexes, _ := catalogMessages(request); len(indexes) != 0 {
		t.Fatal("the pre-skills chat carries a skill catalog")
	}
	if lines := skillGuidanceLines(request.Messages[0].Content); len(lines) != 0 {
		t.Fatalf("the pre-skills chat's prompt gained %q", lines)
	}
	current := c.send(c.selectChat(web.ContextSessionSelection{Unscoped: true}), "hello").requests[0].Messages[0].Content
	if lines := skillGuidanceLines(current); len(lines) != 1 || strings.Replace(current, lines[0]+"\n", "", 1) != request.Messages[0].Content {
		t.Fatal("a new chat's prompt differs from the pre-skills prompt by more than its skill guidance line")
	}
}

// With many skills the catalog stays within its 8 KiB budget: every name
// is listed, a short description is kept whole and the long ones are
// shortened to one common length.
func TestTheSkillCatalogKeepsWithinItsBudget(t *testing.T) {
	c := newSkillChat(t)
	names := []string{"brief"}
	c.approve("brief", map[string]string{"SKILL.md": "---\nname: brief\ndescription: Short.\n---\nBe brief.\n"})
	for i := range 12 {
		name := fmt.Sprintf("procedure-%02d", i)
		names = append(names, name)
		description := fmt.Sprintf("Procedure %02d ", i) + strings.Repeat("with many careful steps ", 40)
		c.approve(name, map[string]string{"SKILL.md": fmt.Sprintf("---\nname: %s\ndescription: %s\n---\nStep.\n", name, strings.TrimSpace(description))})
	}
	request := c.send(c.selectChat(web.ContextSessionSelection{Unscoped: true}), "hello").requests[0]
	_, catalogs := catalogMessages(request)
	if len(catalogs) != 1 || len(catalogs[0]) > 8192 {
		t.Fatalf("catalog of %d messages, %d bytes", len(catalogs), len(catalogs[0]))
	}
	_, list, _ := strings.Cut(catalogs[0], "\n")
	var entries []struct{ Name, Description string }
	if err := json.Unmarshal([]byte(list), &entries); err != nil {
		t.Fatalf("catalog list is not JSON: %v\n%s", err, catalogs[0])
	}
	shortened := map[int]bool{}
	for i, entry := range entries {
		if i >= len(names) || entry.Name != names[i] {
			t.Fatalf("catalog lists %+v, want names %v", entries, names)
		}
		if entry.Name == "brief" {
			if entry.Description != "Short." {
				t.Fatalf("a short description was changed to %q", entry.Description)
			}
			continue
		}
		if !strings.HasSuffix(entry.Description, "…") || !strings.HasPrefix(entry.Description, fmt.Sprintf("Procedure %02d with", i-1)) {
			t.Fatalf("description %d is %q", i, entry.Description)
		}
		shortened[utf8.RuneCountInString(entry.Description)] = true
	}
	if len(entries) != len(names) || len(shortened) != 1 {
		t.Fatalf("%d entries; shortened lengths %v", len(entries), shortened)
	}
}

const handoffSkill = `---
name: handoff
description: Write a prompt that starts a fresh session on the next phase.
argument-hint: "[next phase]"
---
Write the handoff.
`

// chatServer is the web server over the chat's controller, as evie serve
// runs it, and posts JSON to it as the browser does.
func (c *skillChat) chatServer() func(path string, body any) *httptest.ResponseRecorder {
	c.t.Helper()
	handler := web.NewContextServer(nil, nil, nil, c.controller).Handler()
	return func(path string, body any) *httptest.ResponseRecorder {
		c.t.Helper()
		data, err := json.Marshal(body)
		if err != nil {
			c.t.Fatal(err)
		}
		request := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(data))
		request.Host = "127.0.0.1:6687"
		request.Header.Set("Origin", "http://127.0.0.1:6687")
		request.Header.Set("Content-Type", "application/json")
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, request)
		return recorder
	}
}

// ownerMessage is the index of the request's user message with exactly
// this text.
func ownerMessage(t *testing.T, request openrouter.ChatRequest, text string) int {
	t.Helper()
	for i, message := range request.Messages {
		if message.Role == "user" && message.Content == text {
			return i
		}
	}
	t.Fatalf("the request has no owner message %q", text)
	return -1
}

// skillBodies are the request's framed skill bodies, by skill name, with
// their positions.
func skillBodies(request openrouter.ChatRequest) map[string]int {
	out := map[string]int{}
	for i, message := range request.Messages {
		if name, ok := strings.CutPrefix(message.Content, "Skill `"); ok && message.Role == "user" {
			name, _, _ = strings.Cut(name, "`")
			out[name] = i
		}
	}
	return out
}

// The owner invokes a pinned skill with /name (skills.spec.md, Owner
// invocation): the chat's skill list names its pinned skills, owner-only
// ones included; POST /api/chat with skill records the invocation on the
// turn's root message and puts the skill's framed body as a user-role
// message right before his message, on that request and every later one
// until compaction removes the turn, and the compactor never sees it.
// History returns the skill on his message. A skill the chat didn't pin,
// and a chat without the skill capability, are refused.
func TestTheOwnerInvokesAPinnedSkillWithSlashName(t *testing.T) {
	c := newSkillChat(t)
	ctx := context.Background()
	grilling := c.approve("grilling", map[string]string{"SKILL.md": grillingSkill, "references/questions.md": "What breaks first?\n"})
	owl := c.approve("owl", map[string]string{"SKILL.md": owlSkill})
	c.approve("handoff", map[string]string{"SKILL.md": handoffSkill})
	post := c.chatServer()
	open := func(body any) memory.SessionID {
		t.Helper()
		result := post("/api/context-sessions/select", body)
		var opened struct {
			Session memory.Session `json:"session"`
		}
		if result.Code != http.StatusOK || json.Unmarshal(result.Body.Bytes(), &opened) != nil {
			t.Fatalf("open: %d %s", result.Code, result.Body)
		}
		return opened.Session.ID
	}
	chat := open(map[string]any{"unscoped": true})
	client := c.clients[chat]

	listed := post("/api/skills/list", map[string]any{"sessionId": chat})
	var list struct {
		Skills []struct {
			Name         string `json:"name"`
			Description  string `json:"description"`
			ArgumentHint string `json:"argumentHint"`
			OwnerOnly    bool   `json:"ownerOnly"`
		} `json:"skills"`
	}
	if listed.Code != http.StatusOK || json.Unmarshal(listed.Body.Bytes(), &list) != nil {
		t.Fatalf("skill list: %d %s", listed.Code, listed.Body)
	}
	if got := fmt.Sprintf("%+v", list.Skills); got != "[{Name:grilling Description:Grill the owner about a plan until every branch is resolved. ArgumentHint: OwnerOnly:false} "+
		"{Name:handoff Description:Write a prompt that starts a fresh session on the next phase. ArgumentHint:[next phase] OwnerOnly:false} "+
		"{Name:owl Description:Night shift checklist the owner runs himself. ArgumentHint: OwnerOnly:true}]" {
		t.Fatalf("the chat lists skills %s", got)
	}

	chatTurn := func(body map[string]any) *httptest.ResponseRecorder {
		t.Helper()
		body["sessionId"] = chat
		return post("/api/chat", body)
	}
	if sent := chatTurn(map[string]any{"message": "my plan to open a bakery", "skill": "grilling"}); sent.Code != http.StatusOK || !strings.Contains(sent.Body.String(), "turn_done") {
		t.Fatalf("sending with grilling: %d %s", sent.Code, sent.Body)
	}
	first := client.requests[len(client.requests)-1]
	at := ownerMessage(t, first, "my plan to open a bakery")
	if bodies := skillBodies(first); len(bodies) != 1 || bodies["grilling"] != at-1 {
		t.Fatalf("the grilling body is not right before the owner's message: %v, message at %d", bodies, at)
	}
	body := first.Messages[at-1].Content
	frame := "Skill `grilling` " + skillFrame + "\n"
	if !strings.HasPrefix(body, frame) {
		t.Fatalf("the invoked body is not framed:\n%s", body)
	}
	var loaded struct {
		Name, Version, Instructions string
		Resources                   []struct{ Path string }
	}
	if err := json.Unmarshal([]byte(strings.TrimPrefix(body, frame)), &loaded); err != nil || loaded.Name != "grilling" || loaded.Version != grilling ||
		loaded.Instructions != "# Grilling\n\nAsk one hard question at a time.\n" || len(loaded.Resources) != 1 || loaded.Resources[0].Path != "references/questions.md" {
		t.Fatalf("the invoked body is %+v, %v", loaded, err)
	}

	// An owner-only skill is invocable by him; a bare /name is the message.
	if sent := chatTurn(map[string]any{"message": "/owl", "skill": "owl"}); sent.Code != http.StatusOK {
		t.Fatalf("sending with owl: %d %s", sent.Code, sent.Body)
	}
	if sent := chatTurn(map[string]any{"message": "and then?"}); sent.Code != http.StatusOK {
		t.Fatalf("sending without a skill: %d %s", sent.Code, sent.Body)
	}
	later := client.requests[len(client.requests)-1]
	bodies := skillBodies(later)
	if len(bodies) != 2 || bodies["grilling"] != ownerMessage(t, later, "my plan to open a bakery")-1 || bodies["owl"] != ownerMessage(t, later, "/owl")-1 ||
		later.Messages[ownerMessage(t, later, "and then?")-1].Role != "assistant" {
		t.Fatalf("a later request's skill bodies are at %v", bodies)
	}
	if !strings.Contains(later.Messages[bodies["owl"]].Content, "Check the doors.") || later.Messages[bodies["grilling"]].Content != body {
		t.Fatal("a later request does not carry the same framed bodies")
	}

	for name, refused := range map[string]*httptest.ResponseRecorder{
		"an unpinned skill": chatTurn(map[string]any{"message": "wrap it up", "skill": "wrap"}),
		"a malformed name":  chatTurn(map[string]any{"message": "hello", "skill": "../owl"}),
	} {
		if refused.Code < 400 || refused.Code >= 500 || !strings.Contains(refused.Body.String(), "skill") {
			t.Errorf("%s: %d %s", name, refused.Code, refused.Body)
		}
	}
	if err := c.manager.SetEnabled(plugins.SkillsPluginID, false); err != nil {
		t.Fatal(err)
	}
	without := open(map[string]any{"unscoped": true})
	if refused := post("/api/chat", map[string]any{"sessionId": without, "message": "grill me", "skill": "grilling"}); refused.Code < 400 || refused.Code >= 500 ||
		!strings.Contains(refused.Body.String(), "skill") || len(c.clients[without].requests) != 0 {
		t.Fatalf("a chat without the skill capability: %d %s", refused.Code, refused.Body)
	}
	if listed := post("/api/skills/list", map[string]any{"sessionId": without}); listed.Code != http.StatusOK || !strings.Contains(listed.Body.String(), `"skills":[]`) {
		t.Fatalf("a chat without the skill capability lists %d %s", listed.Code, listed.Body)
	}
	if closed := post("/api/skills/list", map[string]any{"sessionId": "not-open"}); closed.Code != http.StatusConflict {
		t.Fatalf("a chat that isn't open lists skills: %d", closed.Code)
	}

	// The invocation is recorded with the turn's root message.
	events, err := c.store.BindHistory(chat, "reader").Events(ctx)
	if err != nil {
		t.Fatal(err)
	}
	payloads := map[string]string{}
	for _, event := range events {
		if event.Type == memory.EventUserMessage {
			payloads[event.Content] = string(event.Payload)
		}
	}
	if want := `{"skill":{"name":"grilling","version":"` + grilling + `"}}`; payloads["my plan to open a bakery"] != want {
		t.Fatalf("the grilling turn's root payload is %s, want %s", payloads["my plan to open a bakery"], want)
	}
	if want := `{"skill":{"name":"owl","version":"` + owl + `"}}`; payloads["/owl"] != want {
		t.Fatalf("the owl turn's root payload is %s", payloads["/owl"])
	}
	if strings.Contains(payloads["and then?"], "skill") || len(payloads) != 3 {
		t.Fatalf("root payloads %v", payloads)
	}

	history := post("/api/context-sessions/history", map[string]any{"sessionId": chat})
	var page struct {
		Items []struct{ Kind, Text, Skill string }
	}
	if history.Code != http.StatusOK || json.Unmarshal(history.Body.Bytes(), &page) != nil {
		t.Fatalf("history: %d %s", history.Code, history.Body)
	}
	var owners []string
	for _, item := range page.Items {
		if item.Kind == "user" {
			owners = append(owners, item.Text+"|"+item.Skill)
		}
	}
	if !slices.Equal(owners, []string{"my plan to open a bakery|grilling", "/owl|owl", "and then?|"}) {
		t.Fatalf("history's owner messages are %q", owners)
	}

	// Compaction removes the first turn and its body with it; the compactor
	// never sees a body.
	if compacted := post("/api/compact", map[string]any{"sessionId": chat}); compacted.Code != http.StatusOK {
		t.Fatalf("compact: %d %s", compacted.Code, compacted.Body)
	}
	if len(client.compactions) != 1 {
		t.Fatalf("compactor requests = %d", len(client.compactions))
	}
	for _, message := range client.compactions[0].Messages {
		if strings.Contains(message.Content, skillFrame) || !strings.Contains(fmt.Sprint(client.compactions[0].Messages), "my plan to open a bakery") {
			t.Fatalf("the compactor saw a skill body or not the turn: %q", message.Content)
		}
	}
	if sent := chatTurn(map[string]any{"message": "after compaction"}); sent.Code != http.StatusOK {
		t.Fatalf("sending after compaction: %d %s", sent.Code, sent.Body)
	}
	compacted := client.requests[len(client.requests)-1]
	if bodies := skillBodies(compacted); len(bodies) != 1 || bodies["owl"] != ownerMessage(t, compacted, "/owl")-1 {
		t.Fatalf("after compaction the skill bodies are at %v", bodies)
	}
}
