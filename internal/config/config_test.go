package config_test

import (
	"testing"

	"github.com/churndesk/churndesk/internal/config"
	"github.com/stretchr/testify/assert"
)

func TestLoadDefaults(t *testing.T) {
	t.Setenv("CHURNDESK_PORT", "")
	t.Setenv("CHURNDESK_DB_PATH", "")
	cfg := config.Load()
	assert.Equal(t, "8080", cfg.Port)
	assert.Equal(t, "/data/churndesk.db", cfg.DBPath)
}

func TestLoadFromEnv(t *testing.T) {
	t.Setenv("CHURNDESK_PORT", "9090")
	t.Setenv("CHURNDESK_DB_PATH", "/tmp/test.db")
	cfg := config.Load()
	assert.Equal(t, "9090", cfg.Port)
	assert.Equal(t, "/tmp/test.db", cfg.DBPath)
}
