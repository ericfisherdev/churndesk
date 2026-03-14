// internal/adapter/sqlite/link_store.go
package sqlite

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/churndesk/churndesk/internal/domain"
	"github.com/churndesk/churndesk/internal/domain/port"
)

type linkStore struct{ db *sql.DB }

// NewLinkStore constructs a SQLite adapter implementing port.LinkStore.
func NewLinkStore(db *sql.DB) port.LinkStore {
	return &linkStore{db: db}
}

func (s *linkStore) UpsertPRJiraLinks(ctx context.Context, prOwner, prRepo string, prNumber int, prTitle string, jiraKeys []string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx,
		`DELETE FROM pr_jira_links WHERE pr_owner=? AND pr_repo=? AND pr_number=?`,
		prOwner, prRepo, prNumber,
	); err != nil {
		return fmt.Errorf("delete existing links: %w", err)
	}

	if len(jiraKeys) > 0 {
		stmt, err := tx.PrepareContext(ctx, `
			INSERT INTO pr_jira_links (pr_owner, pr_repo, pr_number, pr_title, jira_issue_key)
			VALUES (?, ?, ?, ?, ?)
			ON CONFLICT(pr_owner, pr_repo, pr_number, jira_issue_key)
			DO UPDATE SET pr_title = excluded.pr_title
		`)
		if err != nil {
			return fmt.Errorf("prepare link insert: %w", err)
		}
		defer func() { _ = stmt.Close() }()

		for _, key := range jiraKeys {
			if _, err := stmt.ExecContext(ctx, prOwner, prRepo, prNumber, prTitle, key); err != nil {
				return fmt.Errorf("insert link %s: %w", key, err)
			}
		}
	}

	return tx.Commit()
}

func (s *linkStore) GetJiraKeysForPR(ctx context.Context, prOwner, prRepo string, prNumber int) ([]string, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT jira_issue_key FROM pr_jira_links WHERE pr_owner=? AND pr_repo=? AND pr_number=?`,
		prOwner, prRepo, prNumber,
	)
	if err != nil {
		return nil, fmt.Errorf("get jira keys: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var keys []string
	for rows.Next() {
		var k string
		if err := rows.Scan(&k); err != nil {
			return nil, err
		}
		keys = append(keys, k)
	}
	return keys, rows.Err()
}

func (s *linkStore) GetPRsForJiraKey(ctx context.Context, jiraKey string) ([]domain.PRRef, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT pr_owner, pr_repo, pr_number, pr_title FROM pr_jira_links WHERE jira_issue_key=?`,
		jiraKey,
	)
	if err != nil {
		return nil, fmt.Errorf("get prs for jira key: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var refs []domain.PRRef
	for rows.Next() {
		var r domain.PRRef
		if err := rows.Scan(&r.Owner, &r.Repo, &r.Number, &r.Title); err != nil {
			return nil, err
		}
		refs = append(refs, r)
	}
	return refs, rows.Err()
}
