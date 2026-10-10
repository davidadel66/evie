package eviedb

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/davidadel66/evie/internal/memory"
)

// A GitHub Repository's pull requests as the last sync saw them
// (changes.spec.md story 3b), one row per Workspace and pull request
// number. Each sync replaces a pull request's row; merged and closed ones
// are kept. The checks are stored as JSON and read back in Go.
const changePullRequestsSchema = `CREATE TABLE IF NOT EXISTS change_pull_requests (
 workspace_id TEXT NOT NULL REFERENCES workspaces(id),
 number INTEGER NOT NULL CHECK(number > 0),
 owner TEXT NOT NULL, repository TEXT NOT NULL,
 branch TEXT NOT NULL, base TEXT NOT NULL,
 title TEXT NOT NULL, body TEXT NOT NULL, author TEXT NOT NULL,
 state TEXT NOT NULL CHECK(state IN ('open','merged','closed')),
 draft INTEGER NOT NULL CHECK(draft IN (0,1)),
 head_sha TEXT NOT NULL,
 mergeable INTEGER CHECK(mergeable IN (0,1)),
 url TEXT NOT NULL,
 checks_json TEXT NOT NULL CHECK(json_valid(checks_json)),
 github_updated_at TEXT NOT NULL, synced_at TEXT NOT NULL,
 PRIMARY KEY(workspace_id, number))`

// pullRequestTime is fixed width, so the stored times sort as text.
const pullRequestTime = "2006-01-02T15:04:05.000000000Z"

func ensureChangePullRequests(ctx context.Context, db *sql.DB) error {
	if _, err := db.ExecContext(ctx, changePullRequestsSchema); err != nil {
		return fmt.Errorf("create change pull requests: %w", err)
	}
	return nil
}

// ChangePullRequest is one synced pull request.
type ChangePullRequest struct {
	WorkspaceID memory.WorkspaceID
	Number      int
	// Owner and Repository name the GitHub repository it belongs to.
	Owner      string
	Repository string
	// Branch is the head branch; Base the branch it targets.
	Branch string
	Base   string
	Title  string
	Body   string
	// Author is the GitHub login that opened it.
	Author string
	// State is open, merged or closed.
	State string
	// Draft is GitHub's own draft marking.
	Draft   bool
	HeadSHA string
	// Mergeable is unknown (nil) until GitHub has computed it for a read of
	// the one pull request.
	Mergeable       *bool
	URL             string
	Checks          []ChangePullRequestCheck
	GitHubUpdatedAt time.Time
	// SyncedAt is when the store recorded it; a save sets it.
	SyncedAt time.Time
}

// ChangePullRequestCheck is one check on a pull request's head commit.
type ChangePullRequestCheck struct {
	Name  string `json:"name"`
	State string `json:"state"`
	URL   string `json:"url,omitempty"`
}

// SaveChangePullRequests records one sync's pull requests for a Workspace,
// replacing each one's earlier record, all or none.
func (s *Store) SaveChangePullRequests(ctx context.Context, ws memory.WorkspaceID, pulls []ChangePullRequest) error {
	syncedAt := s.now().UTC().Format(pullRequestTime)
	return s.withImmediateTransaction(ctx, func(conn *sql.Conn) error {
		for _, p := range pulls {
			checks := p.Checks
			if checks == nil {
				checks = []ChangePullRequestCheck{}
			}
			encoded, err := json.Marshal(checks)
			if err != nil {
				return err
			}
			var mergeable any
			if p.Mergeable != nil {
				mergeable = *p.Mergeable
			}
			if _, err := conn.ExecContext(ctx, `INSERT INTO change_pull_requests(workspace_id,number,owner,repository,branch,base,title,body,author,state,draft,head_sha,mergeable,url,checks_json,github_updated_at,synced_at)
 VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
 ON CONFLICT(workspace_id,number) DO UPDATE SET owner=excluded.owner,repository=excluded.repository,branch=excluded.branch,base=excluded.base,
 title=excluded.title,body=excluded.body,author=excluded.author,state=excluded.state,draft=excluded.draft,head_sha=excluded.head_sha,
 mergeable=excluded.mergeable,url=excluded.url,checks_json=excluded.checks_json,github_updated_at=excluded.github_updated_at,synced_at=excluded.synced_at`,
				ws, p.Number, p.Owner, p.Repository, p.Branch, p.Base, p.Title, p.Body, p.Author, p.State, p.Draft, p.HeadSHA, mergeable, p.URL,
				string(encoded), p.GitHubUpdatedAt.UTC().Format(pullRequestTime), syncedAt); err != nil {
				return fmt.Errorf("record pull request #%d: %w", p.Number, err)
			}
		}
		return nil
	})
}

// ChangePullRequests reads a Workspace's synced pull requests, most
// recently updated on GitHub first.
func (s *Store) ChangePullRequests(ctx context.Context, ws memory.WorkspaceID) ([]ChangePullRequest, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT number,owner,repository,branch,base,title,body,author,state,draft,head_sha,mergeable,url,checks_json,github_updated_at,synced_at
 FROM change_pull_requests WHERE workspace_id=? ORDER BY github_updated_at DESC, number DESC`, ws)
	if err != nil {
		return nil, fmt.Errorf("query pull requests: %w", err)
	}
	defer rows.Close()
	pulls := []ChangePullRequest{}
	for rows.Next() {
		p := ChangePullRequest{WorkspaceID: ws}
		var mergeable sql.NullBool
		var checks, updated, synced string
		if err := rows.Scan(&p.Number, &p.Owner, &p.Repository, &p.Branch, &p.Base, &p.Title, &p.Body, &p.Author, &p.State, &p.Draft,
			&p.HeadSHA, &mergeable, &p.URL, &checks, &updated, &synced); err != nil {
			return nil, fmt.Errorf("read pull request: %w", err)
		}
		if mergeable.Valid {
			p.Mergeable = &mergeable.Bool
		}
		if err := json.Unmarshal([]byte(checks), &p.Checks); err != nil {
			return nil, fmt.Errorf("read pull request #%d checks: %w", p.Number, err)
		}
		if len(p.Checks) == 0 {
			p.Checks = nil
		}
		if p.GitHubUpdatedAt, err = time.Parse(pullRequestTime, updated); err != nil {
			return nil, fmt.Errorf("read pull request #%d update time: %w", p.Number, err)
		}
		if p.SyncedAt, err = time.Parse(pullRequestTime, synced); err != nil {
			return nil, fmt.Errorf("read pull request #%d sync time: %w", p.Number, err)
		}
		pulls = append(pulls, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read pull requests: %w", err)
	}
	return pulls, nil
}
