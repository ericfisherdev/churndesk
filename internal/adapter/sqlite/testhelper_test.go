// internal/adapter/sqlite/testhelper_test.go
package sqlite_test

import (
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/churndesk/churndesk/internal/db"
	"github.com/stretchr/testify/require"
)

func openTestDB(t *testing.T) *sql.DB {
	t.Helper()
	conn, err := db.Open(filepath.Join(t.TempDir(), "test.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	return conn
}
