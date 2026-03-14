// internal/adapter/sqlite/item_store.go
package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"time"

	"github.com/churndesk/churndesk/internal/domain"
	"github.com/churndesk/churndesk/internal/domain/port"
)

type itemStore struct{ db *sql.DB }

// NewItemStore constructs a SQLite adapter implementing port.ItemStore.
func NewItemStore(db *sql.DB) port.ItemStore {
	return &itemStore{db: db}
}

func (s *itemStore) Upsert(ctx context.Context, items []domain.Item) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	stmt, err := tx.PrepareContext(ctx, `
		INSERT INTO items
			(id, source, type, external_id, title, url, metadata, base_score,
			 age_boost, total_score, pr_owner, pr_repo, dismissed, prerequisites_met,
			 seen, created_at, updated_at)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
		ON CONFLICT(id) DO UPDATE SET
			title             = excluded.title,
			url               = excluded.url,
			metadata          = excluded.metadata,
			base_score        = excluded.base_score,
			age_boost         = excluded.age_boost,
			prerequisites_met = excluded.prerequisites_met,
			total_score       = excluded.total_score,
			updated_at        = excluded.updated_at
	`)
	if err != nil {
		return fmt.Errorf("prepare upsert: %w", err)
	}
	defer func() { _ = stmt.Close() }()

	now := time.Now().UTC()
	for _, it := range items {
		if it.CreatedAt.IsZero() {
			it.CreatedAt = now
		}
		if it.UpdatedAt.IsZero() {
			it.UpdatedAt = now
		}
		if _, err := stmt.ExecContext(ctx,
			it.ID, it.Source, string(it.Type), it.ExternalID, it.Title, it.URL,
			it.Metadata, it.BaseScore, it.AgeBoost, it.TotalScore,
			nullableStr(it.PROwner), nullableStr(it.PRRepo),
			it.Dismissed, it.PrerequisitesMet, it.Seen,
			it.CreatedAt.UTC(), it.UpdatedAt.UTC(),
		); err != nil {
			return fmt.Errorf("upsert item %s: %w", it.ID, err)
		}
	}
	return tx.Commit()
}

func (s *itemStore) ListRanked(ctx context.Context, limit int) ([]domain.Item, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, source, type, external_id, title, url, metadata, base_score,
		       age_boost, total_score, pr_owner, pr_repo, dismissed, prerequisites_met,
		       seen, created_at, updated_at
		FROM items
		WHERE dismissed = 0
		ORDER BY total_score DESC
		LIMIT ?
	`, limit)
	if err != nil {
		return nil, fmt.Errorf("list ranked: %w", err)
	}
	defer func() { _ = rows.Close() }()
	return scanItems(rows)
}

func (s *itemStore) Count(ctx context.Context) (int, error) {
	var n int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM items WHERE dismissed = 0`).Scan(&n)
	return n, err
}

func (s *itemStore) Dismiss(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE items SET dismissed = 1, updated_at = ? WHERE id = ?`, time.Now().UTC(), id,
	)
	return err
}

func (s *itemStore) Delete(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM items WHERE id = ?`, id)
	return err
}

func (s *itemStore) MarkSeen(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE items SET seen = 1, updated_at = ? WHERE id = ?`, time.Now().UTC(), id,
	)
	return err
}

func (s *itemStore) MarkSeenByPR(ctx context.Context, prOwner, prRepo string, prNumber int) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE items SET seen = 1, updated_at = ?
		 WHERE source = ? AND pr_owner = ? AND pr_repo = ? AND external_id = ?`,
		time.Now().UTC(), string(domain.ProviderGitHub), prOwner, prRepo, strconv.Itoa(prNumber),
	)
	return err
}

func (s *itemStore) MarkSeenByJiraKey(ctx context.Context, jiraKey string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE items SET seen = 1, updated_at = ?
		 WHERE source = ? AND external_id = ?`,
		time.Now().UTC(), string(domain.ProviderJira), jiraKey,
	)
	return err
}

type reviewEntry struct {
	Login string `json:"login"`
	State string `json:"state"`
}

type metadataReviews struct {
	Reviews []reviewEntry `json:"reviews"`
}

func (s *itemStore) RescoreAll(ctx context.Context, weights map[domain.ItemType]int, prerequisiteUsernames []string, ageMultiplier, maxAgeBoost float64) error {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, type, metadata, created_at FROM items`,
	)
	if err != nil {
		return fmt.Errorf("query items for rescore: %w", err)
	}
	defer func() { _ = rows.Close() }()

	type row struct {
		id        string
		itemType  domain.ItemType
		metadata  string
		createdAt time.Time
	}
	var toRescore []row
	for rows.Next() {
		var r row
		if scanErr := rows.Scan(&r.id, &r.itemType, &r.metadata, &r.createdAt); scanErr != nil {
			return scanErr
		}
		toRescore = append(toRescore, r)
	}
	if rowsErr := rows.Err(); rowsErr != nil {
		return rowsErr
	}

	prereqSet := make(map[string]struct{}, len(prerequisiteUsernames))
	for _, u := range prerequisiteUsernames {
		prereqSet[u] = struct{}{}
	}

	now := time.Now()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	stmt, err := tx.PrepareContext(ctx,
		`UPDATE items SET base_score = ?, prerequisites_met = ?, age_boost = ?, total_score = ?, updated_at = ? WHERE id = ?`,
	)
	if err != nil {
		return err
	}
	defer func() { _ = stmt.Close() }()

	for _, r := range toRescore {
		baseScore := weights[r.itemType]
		prereqMet := computePrerequisitesMet(r.metadata, prereqSet)
		hours := now.Sub(r.createdAt).Hours()
		var ageBoost float64
		if prereqMet == 1 {
			ageBoost = math.Min(math.Floor(hours)*ageMultiplier, maxAgeBoost)
		}
		total := float64(baseScore) + ageBoost
		if _, err := stmt.ExecContext(ctx, baseScore, prereqMet, ageBoost, total, now.UTC(), r.id); err != nil {
			return fmt.Errorf("rescore item %s: %w", r.id, err)
		}
	}
	return tx.Commit()
}

func computePrerequisitesMet(metadata string, prereqSet map[string]struct{}) int {
	if len(prereqSet) == 0 {
		return 1
	}
	var m metadataReviews
	if err := json.Unmarshal([]byte(metadata), &m); err != nil {
		return 0
	}
	approved := make(map[string]struct{})
	for _, r := range m.Reviews {
		if r.State == "APPROVED" {
			approved[r.Login] = struct{}{}
		}
	}
	for req := range prereqSet {
		if _, ok := approved[req]; !ok {
			return 0
		}
	}
	return 1
}

func scanItems(rows *sql.Rows) ([]domain.Item, error) {
	var items []domain.Item
	for rows.Next() {
		var it domain.Item
		var prOwner, prRepo sql.NullString
		if err := rows.Scan(
			&it.ID, &it.Source, &it.Type, &it.ExternalID, &it.Title, &it.URL,
			&it.Metadata, &it.BaseScore, &it.AgeBoost, &it.TotalScore,
			&prOwner, &prRepo, &it.Dismissed, &it.PrerequisitesMet, &it.Seen,
			&it.CreatedAt, &it.UpdatedAt,
		); err != nil {
			return nil, err
		}
		it.PROwner = prOwner.String
		it.PRRepo = prRepo.String
		items = append(items, it)
	}
	return items, rows.Err()
}

func nullableStr(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}
