package changes

import (
	"context"
	"errors"
	"fmt"

	"github.com/davidadel66/evie/internal/localfiles"
	"github.com/davidadel66/evie/internal/memory"
	"github.com/davidadel66/evie/internal/worktree"
)

// Show (changes.spec.md story 6a): one Change with its diff for the review
// page, read from local git with the folder view's comparison. Evie's
// branch is compared with the base it left, a local merge with the commit
// it moved its target from, and a pull request no branch of Evie's holds,
// the owner's own included, is fetched from GitHub first.

// ErrUnknownChange is a Change the Workspace does not have.
var ErrUnknownChange = errors.New("no such Change")

// ChangeDetail is a Change with its diff.
type ChangeDetail struct {
	Repository Repository                `json:"repository"`
	Change     Change                    `json:"change"`
	Files      []localfiles.ComparedFile `json:"files"`
	// Truncated says some files have no patch because there are too many.
	Truncated bool `json:"truncated,omitempty"`
	// Problem says why there is no diff to show.
	Problem string `json:"problem,omitempty"`
}

// Show reads Change id of Workspace ws, whose Repository is at folder,
// with its diff.
func (l *Lister) Show(ctx context.Context, ws memory.WorkspaceID, folder, id string) (ChangeDetail, error) {
	repo, list, err := l.List(ctx, ws, folder)
	if err != nil {
		return ChangeDetail{}, err
	}
	c, ok := changeByID(list, id)
	if !ok {
		return ChangeDetail{Repository: repo}, fmt.Errorf("%w: this Workspace has no Change %s", ErrUnknownChange, id)
	}
	detail := ChangeDetail{Repository: repo, Change: c, Files: []localfiles.ComparedFile{}}
	base, head, problem := l.span(ctx, folder, repo, c)
	if problem != "" {
		detail.Problem = problem
		return detail, nil
	}
	compared, err := localfiles.Compare(ctx, folder, base, head)
	if err != nil {
		detail.Problem = err.Error()
		return detail, nil
	}
	detail.Files, detail.Truncated = compared.Files, compared.Truncated
	return detail, nil
}

// span is the two revisions c's diff compares, or why there are none.
func (l *Lister) span(ctx context.Context, folder string, repo Repository, c Change) (string, string, string) {
	switch {
	case c.State == Merged && c.MergedFrom != "" && c.Head != "":
		return c.MergedFrom, c.Head, ""
	case c.Author == ByEvie && c.Branch != "" && hasRef(ctx, folder, "refs/heads/"+c.Branch):
		base := "refs/heads/" + c.Base
		if !hasRef(ctx, folder, base) {
			base = "refs/remotes/origin/" + c.Base
		}
		return base, "refs/heads/" + c.Branch, ""
	case repo.Kind == GitHub && c.PullRequest != nil && c.Base != "":
		pull := fmt.Sprintf("refs/evie/pull/%d", c.PullRequest.Number)
		tracking := "refs/remotes/origin/" + c.Base
		if _, err := l.Repositories.remoteGit(ctx, folder, "fetch", "--quiet", "--no-tags", "origin",
			fmt.Sprintf("+refs/pull/%d/head:%s", c.PullRequest.Number, pull), "+refs/heads/"+c.Base+":"+tracking); err != nil {
			return "", "", fmt.Sprintf("Pull request #%d could not be fetched from GitHub: %v", c.PullRequest.Number, err)
		}
		return tracking, pull, ""
	}
	return "", "", "Evie's branch is gone, so there is no diff to show."
}

func hasRef(ctx context.Context, folder, ref string) bool {
	_, err := worktree.Git(ctx, folder, "rev-parse", "--verify", "--quiet", ref)
	return err == nil
}
