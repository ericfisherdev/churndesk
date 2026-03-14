package db_test

import (
	"path/filepath"
	"testing"

	"github.com/churndesk/churndesk/internal/db"
	"github.com/churndesk/churndesk/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOpen_WALModeAndMigrations(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.db")
	conn, err := db.Open(path)
	require.NoError(t, err)
	defer func() { _ = conn.Close() }()

	var mode string
	require.NoError(t, conn.QueryRow("PRAGMA journal_mode").Scan(&mode))
	assert.Equal(t, "wal", mode)

	var count int
	require.NoError(t, conn.QueryRow("SELECT COUNT(*) FROM settings").Scan(&count))
	assert.Equal(t, 5, count)

	var val string
	require.NoError(t, conn.QueryRow(
		"SELECT value FROM settings WHERE key = ?",
		string(domain.SettingAutoRefreshInterval),
	).Scan(&val))
	assert.Equal(t, "20", val)

	var weightCount int
	require.NoError(t, conn.QueryRow("SELECT COUNT(*) FROM category_weights").Scan(&weightCount))
	assert.Equal(t, 9, weightCount)
}

func TestOpen_MigrationIdempotent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.db")
	conn1, err := db.Open(path)
	require.NoError(t, err)
	_ = conn1.Close()

	conn2, err := db.Open(path)
	require.NoError(t, err)
	defer func() { _ = conn2.Close() }()

	var count int
	require.NoError(t, conn2.QueryRow("SELECT COUNT(*) FROM settings").Scan(&count))
	assert.Equal(t, 5, count)
}
