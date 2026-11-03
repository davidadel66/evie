package skill

import "strings"

// A chat pins each skill it can use in its Composition Receipt as an
// instruction reference: ReferencePrefix, the skill's scope, and its name,
// with the version hash. Stage 1 has only Global skills
// (skills.decisions.md, 2026-11-03).
const (
	ReferencePrefix       = "skill."
	GlobalReferencePrefix = ReferencePrefix + "global."
)

// GlobalReference is the receipt reference of the Global skill name.
func GlobalReference(name string) string { return GlobalReferencePrefix + name }

// GlobalReferenceName is the skill name a receipt reference pins, when it
// is a well-formed Global skill reference.
func GlobalReferenceName(id string) (string, bool) {
	name, ok := strings.CutPrefix(id, GlobalReferencePrefix)
	return name, ok && ValidName(name)
}

// Pinned is one approved version a chat pinned: its name and version hash,
// the procedural commit and folder its files were approved at, its manifest
// and what its frontmatter says.
type Pinned struct {
	Name     string
	Hash     string
	Commit   string
	Path     string
	Manifest Manifest
	Metadata Metadata
}

// ModelInvocable reports whether the model may see and load the skill;
// owner-only skills are invoked by David alone.
func (p Pinned) ModelInvocable() bool { return !p.Metadata.DisableModelInvocation }
