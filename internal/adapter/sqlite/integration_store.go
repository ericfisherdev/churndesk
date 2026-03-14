// internal/adapter/sqlite/integration_store.go
package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/churndesk/churndesk/internal/domain"
	"github.com/churndesk/churndesk/internal/domain/port"
)

type integrationStore struct{ db *sql.DB }

// NewIntegrationStore constructs a SQLite adapter implementing port.IntegrationStore.
func NewIntegrationStore(db *sql.DB) port.IntegrationStore {
	return &integrationStore{db: db}
}

func (s *integrationStore) CreateIntegration(ctx context.Context, i domain.Integration) (int, error) {
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO integrations (provider, access_token, base_url, username, account_id, poll_interval_seconds, enabled)
		 VALUES (?,?,?,?,?,?,?)`,
		string(i.Provider), i.AccessToken, i.BaseURL, i.Username, i.AccountID, i.PollIntervalSeconds, boolToInt(i.Enabled),
	)
	if err != nil {
		return 0, fmt.Errorf("create integration: %w", err)
	}
	id, _ := res.LastInsertId()
	return int(id), nil
}

func (s *integrationStore) GetIntegration(ctx context.Context, id int) (*domain.Integration, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, provider, access_token, base_url, username, account_id, poll_interval_seconds, last_synced_at, enabled
		 FROM integrations WHERE id = ?`, id,
	)
	return scanIntegration(row)
}

func (s *integrationStore) UpdateIntegration(ctx context.Context, i domain.Integration) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE integrations SET provider=?, access_token=?, base_url=?, username=?, account_id=?,
		 poll_interval_seconds=?, enabled=? WHERE id=?`,
		string(i.Provider), i.AccessToken, i.BaseURL, i.Username, i.AccountID,
		i.PollIntervalSeconds, boolToInt(i.Enabled), i.ID,
	)
	return err
}

func (s *integrationStore) DeleteIntegration(ctx context.Context, id int) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM integrations WHERE id = ?`, id)
	return err
}

func (s *integrationStore) ListIntegrations(ctx context.Context) ([]domain.Integration, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, provider, access_token, base_url, username, account_id, poll_interval_seconds, last_synced_at, enabled
		 FROM integrations ORDER BY id`,
	)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []domain.Integration
	for rows.Next() {
		i, err := scanIntegration(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *i)
	}
	return out, rows.Err()
}

func (s *integrationStore) UpdateLastSyncedAt(ctx context.Context, id int, t time.Time) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE integrations SET last_synced_at = ? WHERE id = ?`, t.UTC(), id,
	)
	return err
}

func (s *integrationStore) CreateSpace(ctx context.Context, sp domain.Space) (int, error) {
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO spaces (integration_id, provider, owner, name, board_type, jira_board_id, enabled)
		 VALUES (?,?,?,?,?,?,?)`,
		sp.IntegrationID, string(sp.Provider), sp.Owner, sp.Name,
		nullableStr(sp.BoardType), nullableInt(sp.JiraBoardID), boolToInt(sp.Enabled),
	)
	if err != nil {
		return 0, fmt.Errorf("create space: %w", err)
	}
	id, _ := res.LastInsertId()
	return int(id), nil
}

func (s *integrationStore) ListSpaces(ctx context.Context, integrationID int) ([]domain.Space, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, integration_id, provider, owner, name, board_type, jira_board_id, enabled
		 FROM spaces WHERE integration_id = ? ORDER BY id`, integrationID,
	)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []domain.Space
	for rows.Next() {
		var sp domain.Space
		var boardType sql.NullString
		var jiraBoardID sql.NullInt64
		var prov string
		var enabled int
		if err := rows.Scan(&sp.ID, &sp.IntegrationID, &prov, &sp.Owner, &sp.Name,
			&boardType, &jiraBoardID, &enabled); err != nil {
			return nil, err
		}
		sp.Provider = domain.Provider(prov)
		sp.BoardType = boardType.String
		sp.JiraBoardID = int(jiraBoardID.Int64)
		sp.Enabled = enabled == 1
		out = append(out, sp)
	}
	return out, rows.Err()
}

func (s *integrationStore) UpdateSpace(ctx context.Context, sp domain.Space) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE spaces SET owner=?, name=?, board_type=?, jira_board_id=?, enabled=? WHERE id=?`,
		sp.Owner, sp.Name, nullableStr(sp.BoardType), nullableInt(sp.JiraBoardID), boolToInt(sp.Enabled), sp.ID,
	)
	return err
}

func (s *integrationStore) DeleteSpace(ctx context.Context, id int) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM spaces WHERE id = ?`, id)
	return err
}

func (s *integrationStore) CreateTeammate(ctx context.Context, t domain.Teammate) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT OR IGNORE INTO teammates (integration_id, github_username, display_name) VALUES (?,?,?)`,
		t.IntegrationID, t.GitHubUsername, t.DisplayName,
	)
	return err
}

func (s *integrationStore) ListTeammates(ctx context.Context, integrationID int) ([]domain.Teammate, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, integration_id, github_username, display_name FROM teammates WHERE integration_id=? ORDER BY id`,
		integrationID,
	)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []domain.Teammate
	for rows.Next() {
		var t domain.Teammate
		if err := rows.Scan(&t.ID, &t.IntegrationID, &t.GitHubUsername, &t.DisplayName); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

func (s *integrationStore) DeleteTeammate(ctx context.Context, id int) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM teammates WHERE id = ?`, id)
	return err
}

func (s *integrationStore) CreatePrerequisite(ctx context.Context, p domain.ReviewPrerequisite) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT OR IGNORE INTO review_prerequisites (integration_id, github_username, display_name) VALUES (?,?,?)`,
		p.IntegrationID, p.GitHubUsername, p.DisplayName,
	)
	return err
}

func (s *integrationStore) ListPrerequisites(ctx context.Context, integrationID int) ([]domain.ReviewPrerequisite, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, integration_id, github_username, display_name FROM review_prerequisites WHERE integration_id=? ORDER BY id`,
		integrationID,
	)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []domain.ReviewPrerequisite
	for rows.Next() {
		var p domain.ReviewPrerequisite
		if err := rows.Scan(&p.ID, &p.IntegrationID, &p.GitHubUsername, &p.DisplayName); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (s *integrationStore) DeletePrerequisite(ctx context.Context, id int) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM review_prerequisites WHERE id = ?`, id)
	return err
}

func (s *integrationStore) IsOnboardingComplete(ctx context.Context) (bool, error) {
	var count int
	err := s.db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM integrations i
		INNER JOIN spaces sp ON sp.integration_id = i.id AND sp.enabled = 1
		WHERE i.enabled = 1
	`).Scan(&count)
	return count > 0, err
}

func scanIntegration(row interface{ Scan(...any) error }) (*domain.Integration, error) {
	var i domain.Integration
	var provider string
	var lastSynced sql.NullTime
	var enabled int
	if err := row.Scan(&i.ID, &provider, &i.AccessToken, &i.BaseURL, &i.Username, &i.AccountID,
		&i.PollIntervalSeconds, &lastSynced, &enabled); err != nil {
		return nil, fmt.Errorf("scan integration: %w", err)
	}
	i.Provider = domain.Provider(provider)
	i.Enabled = enabled == 1
	if lastSynced.Valid {
		t := lastSynced.Time
		i.LastSyncedAt = &t
	}
	return &i, nil
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func nullableInt(n int) interface{} {
	if n == 0 {
		return nil
	}
	return n
}
