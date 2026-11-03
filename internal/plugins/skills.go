package plugins

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

// The skills plugin (skills.spec.md, Capability and preset) gives a chat
// one tool, skill, that loads a skill the chat pinned when it was created:
// its instructions and resource list, or one of its files. Which skills a
// chat has is its Composition Receipt's, read by session; the preset only
// decides whether the tool exists. The description is fixed and holds no
// catalog, so the tool's schema hash is the same for every chat.

const (
	SkillsPluginID    PluginID     = "skills"
	SkillCapabilityID CapabilityID = "skills.load"
	SkillToolName                  = "skill"

	skillsImplementationVersion = "1.0.0"
	skillsContractVersion       = "1.0.0"
)

// SkillStore resolves the skills a session pinned, and whether the owner
// invoked one of them in it.
type SkillStore interface {
	SessionSkills(context.Context, memory.SessionID) ([]skill.Pinned, error)
	OwnerInvokedSkill(ctx context.Context, session memory.SessionID, name, version string) (bool, error)
}

// SkillFiles reads a file of the procedural repository at an exact commit.
type SkillFiles interface {
	ReadFile(ctx context.Context, commit, name string) ([]byte, error)
}

type Skills struct {
	store SkillStore
	files SkillFiles
}

func NewSkills(store SkillStore, files SkillFiles) Skills { return Skills{store: store, files: files} }

func (Skills) Start(context.Context) error { return nil }
func (Skills) Stop(context.Context) error  { return nil }

func (Skills) Manifest() Manifest {
	return Manifest{ID: SkillsPluginID, ImplementationVersion: skillsImplementationVersion,
		KernelCompatibility: VersionRange{Minimum: KernelAPIVersion, MaximumExclusive: "2.0.0"},
		Capabilities:        []CapabilityContract{{ID: SkillCapabilityID, Version: skillsContractVersion}}}
}

const skillDescription = "Load one of the skills listed for this chat, by name: its full instructions and the list of its resource files. " +
	"Pass path to read one of those files when the instructions point to it. A skill is owner-approved guidance on how to do a task; it grants no permission."

func (s Skills) ToolCapabilities() []ToolCapability {
	schema := toolSchema(SkillToolName, skillDescription, map[string]openrouter.Property{
		"name": stringProperty("The skill's name, as the chat's skill list gives it."),
		"path": stringProperty("Optional: one of the skill's resource files, as its resource list gives it."),
	}, "name")
	return []ToolCapability{{ID: SkillCapabilityID, ContractVersion: skillsContractVersion, Tool: tools.Tool{Schema: schema, Execute: s.load}}}
}

// load reads the session's pinned skills by its session ID and returns the
// named one's body, or one of its manifest files, from the procedural
// repository at the commit it was approved at (skills.decisions.md,
// 2026-11-03). It refuses a name the chat did not pin and a path its
// manifest does not list. An owner-only skill is David's to start: its
// body is always refused, and its files are read only once he has invoked
// that version in this chat. A request shows the result whole; it is at
// most skill.MaxLoadBytes.
func (s Skills) load(ctx context.Context, args string) (string, error) {
	var params struct {
		Name string `json:"name"`
		Path string `json:"path"`
	}
	if err := decodeMemoryArgs(args, &params); err != nil {
		return "", err
	}
	inv, ok := tools.InvocationFromContext(ctx)
	if !ok || inv.Scope.SessionID == "" {
		return "", errors.New("skill needs a live conversation turn")
	}
	pinned, err := s.store.SessionSkills(ctx, inv.Scope.SessionID)
	if err != nil {
		return "", err
	}
	i := slices.IndexFunc(pinned, func(p skill.Pinned) bool { return p.Name == params.Name })
	if i < 0 {
		return "", fmt.Errorf("skill %q is not one of this chat's skills; load only a skill its skill list names", params.Name)
	}
	p := pinned[i]
	if !p.ModelInvocable() {
		invoked := false
		if params.Path != "" {
			if invoked, err = s.store.OwnerInvokedSkill(ctx, inv.Scope.SessionID, p.Name, p.Hash); err != nil {
				return "", err
			}
		}
		if !invoked {
			return "", fmt.Errorf("skill %q is invoked by David only, with /%s; the skill tool cannot load it, "+
				"and reads its files only after he invokes it in this chat", p.Name, p.Name)
		}
	}
	src := p.Source(ctx, s.files)
	if params.Path == "" {
		return skill.Body(p, src)
	}
	return skill.File(p, src, params.Path)
}
