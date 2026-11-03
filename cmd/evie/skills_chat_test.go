package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
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
			c.store.BindTurnOwner(session.ID, holder), resolved.Toolset, agent.WithAutomaticMemoryRecall(false)), nil
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
// turn, makes the tool calls the test queued for it.
type skillChatClient struct {
	requests []openrouter.ChatRequest
	calls    []openrouter.ToolCall
}

func (c *skillChatClient) ChatStream(_ context.Context, request openrouter.ChatRequest, _ openrouter.StreamHandlers) (openrouter.ChatResponse, error) {
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
