// internal/adapter/sqlite/settings_store.go
package sqlite

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/churndesk/churndesk/internal/domain"
	"github.com/churndesk/churndesk/internal/domain/port"
)

type settingsStore struct{ db *sql.DB }

// NewSettingsStore constructs a SQLite adapter implementing port.SettingsStore.
func NewSettingsStore(db *sql.DB) port.SettingsStore {
	return &settingsStore{db: db}
}

func (s *settingsStore) Get(ctx context.Context, key domain.SettingKey) (string, error) {
	var val string
	err := s.db.QueryRowContext(ctx, `SELECT value FROM settings WHERE key = ?`, string(key)).Scan(&val)
	if err != nil {
		return "", fmt.Errorf("get setting %s: %w", key, err)
	}
	return val, nil
}

func (s *settingsStore) Set(ctx context.Context, key domain.SettingKey, value string) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO settings (key, value) VALUES (?,?)
		 ON CONFLICT(key) DO UPDATE SET value = excluded.value`,
		string(key), value,
	)
	return err
}

func (s *settingsStore) GetAll(ctx context.Context) (map[domain.SettingKey]string, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT key, value FROM settings`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[domain.SettingKey]string{}
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			return nil, err
		}
		out[domain.SettingKey(k)] = v
	}
	return out, rows.Err()
}

func (s *settingsStore) GetCategoryWeights(ctx context.Context) ([]domain.CategoryWeight, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT item_type, weight FROM category_weights ORDER BY item_type`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.CategoryWeight
	for rows.Next() {
		var w domain.CategoryWeight
		var itemType string
		if err := rows.Scan(&itemType, &w.Weight); err != nil {
			return nil, err
		}
		w.ItemType = domain.ItemType(itemType)
		out = append(out, w)
	}
	return out, rows.Err()
}

func (s *settingsStore) SetCategoryWeight(ctx context.Context, itemType domain.ItemType, weight int) error {
	if weight < 1 {
		weight = 1
	}
	if weight > 100 {
		weight = 100
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO category_weights (item_type, weight) VALUES (?,?)
		 ON CONFLICT(item_type) DO UPDATE SET weight = excluded.weight`,
		string(itemType), weight,
	)
	return err
}
