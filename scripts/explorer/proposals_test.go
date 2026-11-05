package main

import (
	"strings"
	"testing"
)

// withProposal writes a design proposal file and rebuilds the page with it,
// so check reports only what is wrong with the proposal.
func withProposal(name, text string) func(*testing.T, string) {
	return func(t *testing.T, root string) {
		writeFile(t, root, visionProposalsDir+"/"+name, text)
		buildVisionPage(t, root)
	}
}

func TestCheckHoldsDesignProposalsToWhatCICanCheckWithoutTheSkill(t *testing.T) {
	// The skill's own check validates a proposal in full; CI can only
	// hold each file to being JSON, named by its id, with a status the
	// skill knows.
	file := visionProposalsDir + "/top-k.json"
	valid := proposalJSON("top-k", "proposed")
	runVisionCases(t, []checkCase{
		{
			name:   "not JSON",
			break_: withProposal("top-k.json", `{"id": "top-k", "status": "proposed",`+"\n"),
			want:   []string{file + ": proposal: the file is not valid JSON", "Design proposals", "vision.py build", "1 problem(s)"},
		},
		{
			name:   "two JSON values",
			break_: withProposal("top-k.json", valid+valid),
			want:   []string{file + ": proposal: the file is not valid JSON: there is more than one JSON value", "1 problem(s)"},
		},
		{
			name:   "not an object",
			break_: withProposal("top-k.json", `["top-k", "proposed"]`+"\n"),
			want:   []string{file + ": proposal: the file is not a JSON object", "1 problem(s)"},
		},
		{
			name:   "an id that is not the file's name",
			break_: withProposal("top-k.json", proposalJSON("topk", "proposed")),
			want:   []string{file + `: id: id "topk" does not match the file name`, `set id to "top-k"`, "rename the file topk.json", "vision.py build", "1 problem(s)"},
		},
		{
			name:   "no id",
			break_: withProposal("top-k.json", strings.Replace(valid, `"id": "top-k", `, "", 1)),
			want:   []string{file + ": id: the proposal has no id", `set id to "top-k"`, "1 problem(s)"},
		},
		{
			name:   "an id that is not a string",
			break_: withProposal("top-k.json", strings.Replace(valid, `"id": "top-k"`, `"id": 7`, 1)),
			want:   []string{file + ": id: id 7 does not match the file name", "1 problem(s)"},
		},
		{
			name:   "a status the skill does not know",
			break_: withProposal("top-k.json", proposalJSON("top-k", "done")),
			want:   []string{file + `: status: status "done" is not one of proposed, accepted, building or example`, "delete the proposal once its feature merges", "vision.py build", "1 problem(s)"},
		},
		{
			name:   "no status",
			break_: withProposal("top-k.json", strings.Replace(valid, `"status": "proposed",`, "", 1)),
			want:   []string{file + ": status: the proposal has no status", "1 problem(s)"},
		},
		{
			name: "every proposal, in file-name order",
			break_: func(t *testing.T, root string) {
				writeFile(t, root, visionProposalsDir+"/b.json", proposalJSON("b", "done"))
				withProposal("a.json", proposalJSON("x", "proposed"))(t, root)
			},
			want: []string{visionProposalsDir + "/a.json: id:", visionProposalsDir + "/b.json: status:", "2 problem(s)"},
		},
	})
}

func TestCheckReadsOnlyTheProposalFilesTheSkillReads(t *testing.T) {
	// Like the skill, check reads only .json files directly in the folder
	// whose names do not start with a dot.
	root := newVisionFixture(t)
	for name, text := range map[string]string{
		".draft.json":       "{ not yet\n",
		"notes.md":          "Not a proposal.\n",
		"top-k.JSON":        "{ not read either\n",
		"later/inner.json":  "{ nor this\n",
		"accepted-one.json": proposalJSON("accepted-one", "accepted"),
		"building-one.json": proposalJSON("building-one", "building"),
		"example-one.json":  proposalJSON("example-one", "example"),
	} {
		writeFile(t, root, visionProposalsDir+"/"+name, text)
	}
	buildVisionPage(t, root)
	if code, out := checkFixture(t, root); code != 0 {
		t.Fatalf("check exit %d, want 0:\n%s", code, out)
	}
}
