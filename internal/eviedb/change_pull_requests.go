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

// changeMergeArmsSchema is "Merge when checks pass" (story 5c): one marker
// per pull request, with when the owner set it.
const changeMergeArmsSchema = `CREATE TABLE IF NOT EXISTS change_merge_arms (
 workspace_id TEXT NOT NULL REFERENCES workspaces(id),
 number INTEGER NOT NULL CHECK(number > 0),
 armed_at TEXT NOT NULL,
 PRIMARY KEY(workspace_id, number))`

func ensureChangePullRequests(ctx context.Context, db *sql.DB) error {
	if _, err := db.ExecContext(ctx, changePullRequestsSchema); err != nil {
		return fmt.Errorf("create change pull requests: %w", err)
	}
	if _, err := db.ExecContext(ctx, changeMergeArmsSchema); err != nil {
		return fmt.Errorf("create change merge arms: %w", err)
	}
	return nil
}

// ArmChangeMerge sets "Merge when checks pass" on pull request number; set
// again, it keeps the first time.
func (s *Store) ArmChangeMerge(ctx context.Context, ws memory.WorkspaceID, number int) error {
	_, err := s.db.ExecContext(ctx, `INSERT OR IGNORE INTO change_merge_arms(workspace_id,number,armed_at) VALUES(?,?,?)`, ws, number, s.now().UTC().Format(pullRequestTime))
	if err != nil {
		return fmt.Errorf("set merge when checks pass on #%d: %w", number, err)
	}
	return nil
}

// DisarmChangeMerge clears "Merge when checks pass" on pull request number
// and says whether it was set.
func (s *Store) DisarmChangeMerge(ctx context.Context, ws memory.WorkspaceID, number int) (bool, error) {
	res, err := s.db.ExecContext(ctx, `DELETE FROM change_merge_arms WHERE workspace_id=? AND number=?`, ws, number)
	if err != nil {
		return false, fmt.Errorf("clear merge when checks pass on #%d: %w", number, err)
	}
	n, err := res.RowsAffected()
	return n > 0, err
}

// ChangeMergeArms is when "Merge when checks pass" was set on each of the
// Workspace's pull requests that have it.
func (s *Store) ChangeMergeArms(ctx context.Context, ws memory.WorkspaceID) (map[int]time.Time, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT number,armed_at FROM change_merge_arms WHERE workspace_id=?`, ws)
	if err != nil {
		return nil, fmt.Errorf("read merge when checks pass: %w", err)
	}
	defer rows.Close()
	arms := map[int]time.Time{}
	for rows.Next() {
		var number int
		var at string
		if err := rows.Scan(&number, &at); err != nil {
			return nil, err
		}
		if arms[number], err = time.Parse(pullRequestTime, at); err != nil {
			return nil, err
		}
	}
	return arms, rows.Err()
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
// replacing each one's earlier record, all or none. A record without
// mergeability (GitHub's lists carry none) keeps what was known about the
// same head (story 5b).
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
 mergeable=CASE WHEN excluded.mergeable IS NULL AND head_sha=excluded.head_sha THEN mergeable ELSE excluded.mergeable END,url=excluded.url,checks_json=excluded.checks_json,github_updated_at=excluded.github_updated_at,synced_at=excluded.synced_at`,
				ws, p.Number, p.Owner, p.Repository, p.Branch, p.Base, p.Title, p.Body, p.Author, p.State, p.Draft, p.HeadSHA, mergeable, p.URL,
				string(encoded), p.GitHubUpdatedAt.UTC().Format(pullRequestTime), syncedAt); err != nil {
				return fmt.Errorf("record pull request #%d: %w", p.Number, err)
			}
		}
		return nil
	})
}

// RecordChangeMergeable records whether GitHub can merge pull request
// number at head (story 5b): a conflict found when the owner merges lasts
// until new commits change the head. An answer about another head than the
// recorded one changes nothing.
func (s *Store) RecordChangeMergeable(ctx context.Context, ws memory.WorkspaceID, number int, head string, mergeable bool) error {
	_, err := s.db.ExecContext(ctx, `UPDATE change_pull_requests SET mergeable=? WHERE workspace_id=? AND number=? AND head_sha=?`, mergeable, ws, number, head)
	if err != nil {
		return fmt.Errorf("record pull request #%d mergeability: %w", number, err)
	}
	return nil
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
