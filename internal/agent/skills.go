package agent

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/davidadel66/evie/internal/memory"
	"github.com/davidadel66/evie/internal/openrouter"
	"github.com/davidadel66/evie/internal/skill"
	"github.com/davidadel66/evie/internal/tools"
)

// skillsProvider is implemented by durable histories that can resolve the
// skills their bound session pinned when it was created (skills.spec.md).
type skillsProvider interface {
	SessionSkills(context.Context) ([]skill.Pinned, error)
}

// SkillFiles reads a file of the procedural repository at an exact commit,
// where each pinned skill version's files were approved.
type SkillFiles interface {
	ReadFile(ctx context.Context, commit, name string) ([]byte, error)
}

// WithSkillFiles lets the session read the skills its owner invokes with
// /name, whose framed bodies its requests carry.
func WithSkillFiles(files SkillFiles) SessionOption {
	return func(session *Session) { session.skillFiles = files }
}

var (
	// ErrNoSkills refuses a skill invocation in a chat without the skill
	// capability.
	ErrNoSkills = errors.New("this chat can't use skills")
	// ErrSkillNotPinned refuses a skill invocation naming a skill the chat
	// did not pin.
	ErrSkillNotPinned = errors.New("this chat has no such skill")
)

// skillCatalog renders the session's skill catalog message: its pinned
// model-invocable skills' names and descriptions. The pins and the
// approvals they name are immutable, so the catalog is the same for the
// chat's whole life and stays in the cached prefix. Delegated workers start
// without skills.
func (s *Session) skillCatalog(ctx context.Context) (string, error) {
	if s.workerInstructions != "" {
		return "", nil
	}
	pinned, err := s.pinnedSkills(ctx)
	if err != nil {
		return "", err
	}
	return skill.Catalog(pinned), nil
}

func (s *Session) pinnedSkills(ctx context.Context) ([]skill.Pinned, error) {
	provider, ok := s.history.(skillsProvider)
	if !ok {
		return nil, nil
	}
	return provider.SessionSkills(ctx)
}

// Skills are the skills the owner can invoke in this chat with /name
// (skills.spec.md, Owner invocation): every skill it pinned, owner-only ones
// included. A chat without the skill tool, such as one pinned to an older
// preset version or a delegated worker, has none.
func (s *Session) Skills(ctx context.Context) ([]skill.Pinned, error) {
	if !s.hasSkillTool() {
		return nil, nil
	}
	return s.pinnedSkills(ctx)
}

// OwnerSkill is the pinned skill the owner invokes by name. It refuses a
// chat without the skill tool (ErrNoSkills) and a name the chat did not pin
// (ErrSkillNotPinned).
func (s *Session) OwnerSkill(ctx context.Context, name string) (skill.Pinned, error) {
	if !s.hasSkillTool() {
		return skill.Pinned{}, ErrNoSkills
	}
	pinned, err := s.pinnedSkills(ctx)
	if err != nil {
		return skill.Pinned{}, err
	}
	i := slices.IndexFunc(pinned, func(p skill.Pinned) bool { return p.Name == name })
	if i < 0 {
		return skill.Pinned{}, fmt.Errorf("%w: %q", ErrSkillNotPinned, name)
	}
	return pinned[i], nil
}

func (s *Session) hasSkillTool() bool {
	if s.workerInstructions != "" {
		return false
	}
	return slices.ContainsFunc(s.toolset.Schemas(), func(tool openrouter.Tool) bool { return tool.Function.Name == skillTool })
}

// SendSkill runs a turn for a message the owner sent with one of the chat's
// pinned skills. The invocation is recorded in the turn's root message, in
// the same event, and every request whose window holds that message carries
// the skill's framed body right before it.
func (s *Session) SendSkill(ctx context.Context, input, name string, ev Events, approve tools.Approver) error {
	invoked, err := s.OwnerSkill(ctx, name)
	if err != nil {
		return err
	}
	return s.send(ctx, input, &memory.SkillInvocation{Name: invoked.Name, Version: invoked.Hash}, ev, approve)
}

// skillBodyViewEvent marks where an invoked skill's framed body sits in a
// projected conversation: right before the owner message that invoked it.
const skillBodyViewEvent memory.EventType = "evie_skill_body_view"

// invokedSkillBodies are the framed bodies of the skills the owner invoked
// in the turns a request can carry, by the root message that invoked each:
// the roots from the summary's retained frontier on. Each version is read
// from the procedural repository at its pinned commit once per bodies
// cache.
func (s *Session) invokedSkillBodies(ctx context.Context, events []memory.Event, summary *ContextSummary, bodies map[string]string) (map[memory.EventID]string, error) {
	retained := summary == nil || summary.FirstRetainedEventID == ""
	var out map[memory.EventID]string
	var pinned []skill.Pinned
	loaded := false
	for _, event := range events {
		if !retained && event.ID == summary.FirstRetainedEventID {
			retained = true
		}
		if !retained || event.Type != memory.EventUserMessage || len(event.Payload) == 0 {
			continue
		}
		var payload memory.UserMessagePayload
		if err := decodeEventPayload(event, &payload); err != nil {
			return nil, err
		}
		if payload.Skill == nil {
			continue
		}
		key := payload.Skill.Name + "@" + payload.Skill.Version
		body, ok := bodies[key]
		if !ok {
			if !loaded {
				var err error
				if pinned, err = s.pinnedSkills(ctx); err != nil {
					return nil, err
				}
				loaded = true
			}
			i := slices.IndexFunc(pinned, func(p skill.Pinned) bool {
				return p.Name == payload.Skill.Name && p.Hash == payload.Skill.Version
			})
			if i < 0 {
				return nil, fmt.Errorf("message %q invoked skill %s at version %s, which this chat did not pin", event.ID, payload.Skill.Name, payload.Skill.Version)
			}
			if s.skillFiles == nil {
				return nil, fmt.Errorf("message %q invoked skill %s, but this session cannot read skills", event.ID, payload.Skill.Name)
			}
			p := pinned[i]
			var err error
			if body, err = skill.Body(p, pinnedSkillSource{ctx: ctx, files: s.skillFiles, commit: p.Commit, dir: p.Path}); err != nil {
				return nil, fmt.Errorf("read invoked skill %s: %w", p.Name, err)
			}
			bodies[key] = body
		}
		if out == nil {
			out = map[memory.EventID]string{}
		}
		out[event.ID] = body
	}
	return out, nil
}

// withInvokedSkills places each invoked skill's framed body right before
// the owner message that invoked it. Only a request's conversation carries
// them; the compactor's transcript never does.
func withInvokedSkills(events []memory.Event, bodies map[memory.EventID]string) []memory.Event {
	if len(bodies) == 0 {
		return events
	}
	out := make([]memory.Event, 0, len(events)+len(bodies))
	for _, event := range events {
		if body, ok := bodies[event.ID]; ok && event.Type == memory.EventUserMessage {
			out = append(out, memory.Event{
				ID: event.ID + "-skill", SessionID: event.SessionID, Sequence: event.Sequence,
				Type: skillBodyViewEvent, Role: memory.RoleUser, Content: body,
			})
		}
		out = append(out, event)
	}
	return out
}

// pinnedSkillSource reads a skill's folder at the commit it was approved
// at.
type pinnedSkillSource struct {
	ctx         context.Context
	files       SkillFiles
	commit, dir string
}

func (s pinnedSkillSource) ReadFile(name string) ([]byte, error) {
	return s.files.ReadFile(s.ctx, s.commit, s.dir+"/"+name)
}
