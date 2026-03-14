## Chunk 1: Foundation

### Task 1: Project Scaffold

**Files:**
- Create: `go.mod`
- Create: all directories from File Structure

- [ ] **Step 1: Initialize module and create directory tree**

```bash
cd /home/esfisher/dev/churndesk
go mod init github.com/churndesk/churndesk

mkdir -p cmd/churndesk \
  internal/config \
  internal/db/migrations \
  internal/domain/port \
  internal/adapter/sqlite \
  internal/adapter/github \
  internal/adapter/jira \
  internal/app \
  internal/markdown \
  internal/web/handlers \
  internal/web/templates \
  static
```

- [ ] **Step 2: Add all dependencies**

```bash
go get github.com/mattn/go-sqlite3
go get github.com/google/go-github/v68
go get github.com/ctreminiom/go-atlassian
go get github.com/a-h/templ
go get github.com/yuin/goldmark
go get github.com/alecthomas/chroma/v2
go get github.com/microcosm-cc/bluemonday
go get github.com/stretchr/testify

# Install templ CLI (compiles .templ → .templ.go at build time)
go install github.com/a-h/templ/cmd/templ@latest
```

- [ ] **Step 3: Verify go.mod**

```bash
cat go.mod
```
Expected: all packages listed under `require`

- [ ] **Step 4: Commit**

```bash
git add go.mod go.sum
git commit -m "chore: initialize go module with all dependencies"
```

---

### Task 2: Config Package

**Files:**
- Create: `internal/config/config.go`
- Create: `internal/config/config_test.go`

- [ ] **Step 1: Write the failing test**

```go
// internal/config/config_test.go
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
```

- [ ] **Step 2: Run test to verify it fails**

```bash
CGO_ENABLED=1 go test ./internal/config/...
```
Expected: FAIL

- [ ] **Step 3: Implement**

```go
// internal/config/config.go
package config

import "os"

type Config struct {
	Port   string
	DBPath string
}

func Load() Config {
	port := os.Getenv("CHURNDESK_PORT")
	if port == "" {
		port = "8080"
	}
	dbPath := os.Getenv("CHURNDESK_DB_PATH")
	if dbPath == "" {
		dbPath = "/data/churndesk.db"
	}
	return Config{Port: port, DBPath: dbPath}
}
```

- [ ] **Step 4: Run test to verify it passes**

```bash
CGO_ENABLED=1 go test ./internal/config/...
```
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/config/
git commit -m "feat: add config package with env-based configuration"
```

---

### Task 3: Database Package + Initial Migration

**Files:**
- Create: `internal/db/db.go`
- Create: `internal/db/migrations/001_initial.sql`
- Create: `internal/db/db_test.go`

- [ ] **Step 1: Write the failing test**

```go
// internal/db/db_test.go
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
	defer conn.Close()

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
	conn1.Close()

	conn2, err := db.Open(path)
	require.NoError(t, err)
	defer conn2.Close()

	var count int
	require.NoError(t, conn2.QueryRow("SELECT COUNT(*) FROM settings").Scan(&count))
	assert.Equal(t, 5, count)
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
CGO_ENABLED=1 go test ./internal/db/...
```
Expected: FAIL

- [ ] **Step 3: Write migration SQL**

```sql
-- internal/db/migrations/001_initial.sql

CREATE TABLE IF NOT EXISTS integrations (
    id                    INTEGER PRIMARY KEY AUTOINCREMENT,
    provider              TEXT    NOT NULL,
    access_token          TEXT    NOT NULL DEFAULT '',
    base_url              TEXT    NOT NULL DEFAULT '',
    username              TEXT    NOT NULL DEFAULT '',
    poll_interval_seconds INTEGER NOT NULL DEFAULT 300,
    last_synced_at        DATETIME,
    enabled               INTEGER NOT NULL DEFAULT 1
);

CREATE TABLE IF NOT EXISTS spaces (
    id             INTEGER PRIMARY KEY AUTOINCREMENT,
    integration_id INTEGER NOT NULL REFERENCES integrations(id) ON DELETE CASCADE,
    provider       TEXT    NOT NULL,
    owner          TEXT    NOT NULL DEFAULT '',
    name           TEXT    NOT NULL,
    board_type     TEXT,
    jira_board_id  INTEGER,
    enabled        INTEGER NOT NULL DEFAULT 1
);

CREATE TABLE IF NOT EXISTS teammates (
    id               INTEGER PRIMARY KEY AUTOINCREMENT,
    integration_id   INTEGER NOT NULL REFERENCES integrations(id) ON DELETE CASCADE,
    github_username  TEXT    NOT NULL,
    display_name     TEXT    NOT NULL DEFAULT '',
    UNIQUE(integration_id, github_username)
);

CREATE TABLE IF NOT EXISTS review_prerequisites (
    id               INTEGER PRIMARY KEY AUTOINCREMENT,
    integration_id   INTEGER NOT NULL REFERENCES integrations(id) ON DELETE CASCADE,
    github_username  TEXT    NOT NULL,
    display_name     TEXT    NOT NULL DEFAULT '',
    UNIQUE(integration_id, github_username)
);

CREATE TABLE IF NOT EXISTS items (
    id                TEXT    PRIMARY KEY,
    source            TEXT    NOT NULL,
    type              TEXT    NOT NULL,
    external_id       TEXT    NOT NULL,
    title             TEXT    NOT NULL,
    url               TEXT    NOT NULL DEFAULT '',
    metadata          TEXT    NOT NULL DEFAULT '{}',
    base_score        INTEGER NOT NULL DEFAULT 0,
    age_boost         REAL    NOT NULL DEFAULT 0,
    total_score       REAL    NOT NULL DEFAULT 0,
    pr_owner          TEXT,
    pr_repo           TEXT,
    dismissed         INTEGER NOT NULL DEFAULT 0,
    prerequisites_met INTEGER NOT NULL DEFAULT 1,
    seen              INTEGER NOT NULL DEFAULT 0,
    created_at        DATETIME NOT NULL DEFAULT (datetime('now')),
    updated_at        DATETIME NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS pr_jira_links (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    pr_owner        TEXT    NOT NULL,
    pr_repo         TEXT    NOT NULL,
    pr_number       INTEGER NOT NULL,
    pr_title        TEXT    NOT NULL DEFAULT '',
    jira_issue_key  TEXT    NOT NULL,
    UNIQUE(pr_owner, pr_repo, pr_number, jira_issue_key)
);

CREATE TABLE IF NOT EXISTS category_weights (
    item_type TEXT    PRIMARY KEY,
    weight    INTEGER NOT NULL DEFAULT 50
);

INSERT OR IGNORE INTO category_weights (item_type, weight) VALUES
    ('pr_ci_failing',        90),
    ('pr_changes_requested', 80),
    ('pr_stale_review',      70),
    ('jira_new_bug',         70),
    ('pr_review_needed',     60),
    ('pr_new_comment',       50),
    ('jira_status_change',   40),
    ('jira_comment',         40),
    ('pr_approved',          30);

CREATE TABLE IF NOT EXISTS settings (
    key   TEXT PRIMARY KEY,
    value TEXT NOT NULL
);

INSERT OR IGNORE INTO settings (key, value) VALUES
    ('auto_refresh_interval', '20'),
    ('age_multiplier',        '0.5'),
    ('max_age_boost',         '50'),
    ('feed_columns',          '1'),
    ('min_review_count',      '1');
```

- [ ] **Step 4: Implement db.Open**

Note: `_loc=UTC` makes the driver return `DATETIME` columns as `time.Time` in UTC — required for all adapters that scan timestamps. `_foreign_keys=on` enables FK enforcement (per-connection setting; not in migration SQL).

```go
// internal/db/db.go
package db

import (
	"database/sql"
	"embed"
	"fmt"
	"io/fs"
	"sort"
	"strings"

	_ "github.com/mattn/go-sqlite3"
)

//go:embed migrations/*.sql
var migrationFiles embed.FS

// Open opens (or creates) the SQLite database at path, enables WAL mode,
// foreign-key enforcement, and UTC timestamp parsing; then runs pending migrations.
func Open(path string) (*sql.DB, error) {
	dsn := path + "?_foreign_keys=on&_loc=UTC"
	conn, err := sql.Open("sqlite3", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	if _, err := conn.Exec("PRAGMA journal_mode=WAL"); err != nil {
		conn.Close()
		return nil, fmt.Errorf("set WAL mode: %w", err)
	}
	if err := runMigrations(conn); err != nil {
		conn.Close()
		return nil, fmt.Errorf("run migrations: %w", err)
	}
	return conn, nil
}

func runMigrations(conn *sql.DB) error {
	if _, err := conn.Exec(
		`CREATE TABLE IF NOT EXISTS schema_migrations (version TEXT PRIMARY KEY)`,
	); err != nil {
		return fmt.Errorf("create schema_migrations: %w", err)
	}

	entries, err := fs.ReadDir(migrationFiles, "migrations")
	if err != nil {
		return fmt.Errorf("read migrations dir: %w", err)
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })

	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".sql") {
			continue
		}
		version := strings.TrimSuffix(e.Name(), ".sql")

		var n int
		if err := conn.QueryRow(
			`SELECT COUNT(*) FROM schema_migrations WHERE version = ?`, version,
		).Scan(&n); err != nil {
			return fmt.Errorf("check migration %s: %w", version, err)
		}
		if n > 0 {
			continue
		}

		content, err := migrationFiles.ReadFile("migrations/" + e.Name())
		if err != nil {
			return fmt.Errorf("read migration %s: %w", e.Name(), err)
		}
		if err := execStatements(conn, string(content)); err != nil {
			return fmt.Errorf("exec migration %s: %w", e.Name(), err)
		}
		if _, err := conn.Exec(
			`INSERT INTO schema_migrations (version) VALUES (?)`, version,
		); err != nil {
			return fmt.Errorf("record migration %s: %w", version, err)
		}
	}
	return nil
}

// execStatements splits SQL on ';' and executes each statement individually.
func execStatements(conn *sql.DB, sql string) error {
	for _, stmt := range strings.Split(sql, ";") {
		stmt = strings.TrimSpace(stmt)
		if stmt == "" {
			continue
		}
		if _, err := conn.Exec(stmt); err != nil {
			return fmt.Errorf("statement %q: %w", truncate(stmt, 60), err)
		}
	}
	return nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
```

- [ ] **Step 5: Run test to verify it passes**

```bash
CGO_ENABLED=1 go test ./internal/db/...
```
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add internal/db/
git commit -m "feat: add SQLite db package with WAL mode and migration runner"
```

---

### Task 4: Domain Entities

**Files:**
- Create: `internal/domain/item.go`
- Create: `internal/domain/integration.go`
- Create: `internal/domain/settings.go`

No tests — pure type definitions. Adapter tests exercise them indirectly.

- [ ] **Step 1: Write item.go**

```go
// internal/domain/item.go
package domain

import "time"

// ItemType is the category of a feed item (value object).
type ItemType string

const (
	ItemTypePRReviewNeeded     ItemType = "pr_review_needed"
	ItemTypePRStaleReview      ItemType = "pr_stale_review"
	ItemTypePRChangesRequested ItemType = "pr_changes_requested"
	ItemTypePRNewComment       ItemType = "pr_new_comment"
	ItemTypePRCIFailing        ItemType = "pr_ci_failing"
	ItemTypePRApproved         ItemType = "pr_approved"
	ItemTypeJiraStatusChange   ItemType = "jira_status_change"
	ItemTypeJiraComment        ItemType = "jira_comment"
	ItemTypeJiraNewBug         ItemType = "jira_new_bug"
)

// Item is a single feed entry (entity).
//
// Deleted is NOT persisted. Fetchers set it true to signal the app worker to
// hard-delete the item (e.g. pr_approved revocation) rather than upsert it.
type Item struct {
	ID               string
	Source           string   // "github" or "jira"
	Type             ItemType // one of the ItemType constants
	ExternalID       string   // raw PR number string (e.g. "1042") or Jira issue key (e.g. "FRONT-441")
	Title            string
	URL              string
	Metadata         string // JSON blob; see spec section 6 for schema
	BaseScore        int
	AgeBoost         float64
	TotalScore       float64
	PROwner          string // GitHub org/user — empty for Jira items
	PRRepo           string // GitHub repo name — empty for Jira items
	Dismissed        int    // 0 or 1
	PrerequisitesMet int    // 0 or 1; suppresses age_boost when 0
	Seen             int    // 0 or 1; drives unread dot indicator
	CreatedAt        time.Time
	UpdatedAt        time.Time

	// Deleted signals the app worker to call Delete(id) instead of Upsert.
	// Never read from or written to the database.
	Deleted bool
}

// PRDetail is full pull request data fetched live from the GitHub API.
type PRDetail struct {
	Number       int
	Title        string
	Owner        string
	Repo         string
	Branch       string
	BaseBranch   string
	Author       string
	HeadSHA      string
	Additions    int
	Deletions    int
	FilesChanged int
	State        string // "open", "closed", "merged"
	Body         string
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// Comment is a comment on a PR or Jira issue.
// IsChangeRequest is GitHub-only; always false for Jira comments.
type Comment struct {
	ID              int64
	Author          string
	Body            string
	CreatedAt       time.Time
	IsChangeRequest bool
}

// Review is a GitHub PR review submission.
type Review struct {
	ID     int64
	Author string
	State  string // "APPROVED", "CHANGES_REQUESTED", "DISMISSED"
}

// CheckRun is a single CI check result on a PR.
type CheckRun struct {
	Name       string
	Status     string
	Conclusion string // "success", "failure", "neutral", etc.
}

// PRRef is a lightweight reference used in the Jira linked-PRs bar.
type PRRef struct {
	Owner  string
	Repo   string
	Title  string
	Number int
}

// JiraIssue is full issue data fetched live from the Jira API.
type JiraIssue struct {
	Key         string
	Summary     string
	Status      string
	Priority    string
	IssueType   string
	Assignee    string
	Reporter    string
	Description string
	Sprint      string
	StoryPoints float64
	CreatedAt   time.Time
	UpdatedAt   time.Time
	Comments    []Comment
}

// Board is a Jira board (scrum or kanban).
type Board struct {
	ID   int
	Name string
	Type string // "scrum" or "kanban"
}
```

- [ ] **Step 2: Write integration.go**

```go
// internal/domain/integration.go
package domain

import "time"

// Provider identifies which external service an integration connects to (value object).
type Provider string

const (
	ProviderGitHub Provider = "github"
	ProviderJira   Provider = "jira"
)

// Integration is an aggregate root representing a configured external service connection.
// LastSyncedAt is nil until the first sync completes; fetchers treat nil as
// "match nothing" — no comment or status-change items are generated on first sync.
type Integration struct {
	ID                  int
	Provider            Provider
	AccessToken         string
	BaseURL             string // Jira only: e.g. "https://myorg.atlassian.net"
	Username            string // GitHub username or Jira account ID
	PollIntervalSeconds int
	LastSyncedAt        *time.Time // nil = never synced
	Enabled             bool
}

// Space is a tracked GitHub repo or Jira project (part of Integration aggregate).
// BoardType and JiraBoardID are Jira-only; empty/zero until first sync.
type Space struct {
	ID            int
	IntegrationID int
	Provider      Provider
	Owner         string // GitHub org/user or Jira project key
	Name          string // GitHub repo name or Jira space name
	BoardType     string // "scrum" or "kanban" — Jira only
	JiraBoardID   int    // cached board ID — Jira only; 0 until detected
	Enabled       bool
}

// Teammate is a GitHub user whose open PRs are watched for review needs.
type Teammate struct {
	ID             int
	IntegrationID  int
	GitHubUsername string
	DisplayName    string
}

// ReviewPrerequisite is a bot that must approve a PR before age_boost is applied.
type ReviewPrerequisite struct {
	ID             int
	IntegrationID  int
	GitHubUsername string // e.g. "copilot[bot]"
	DisplayName    string
}
```

- [ ] **Step 3: Write settings.go**

```go
// internal/domain/settings.go
package domain

// SettingKey is a typed constant for a settings table key (value object).
// All application code MUST use these constants — never raw strings.
type SettingKey string

const (
	SettingAutoRefreshInterval SettingKey = "auto_refresh_interval"
	SettingAgeMultiplier       SettingKey = "age_multiplier"
	SettingMaxAgeBoost         SettingKey = "max_age_boost"
	SettingFeedColumns         SettingKey = "feed_columns"
	SettingMinReviewCount      SettingKey = "min_review_count"
)

// CategoryWeight pairs an item type with its base priority weight (1–100).
type CategoryWeight struct {
	ItemType ItemType
	Weight   int
}
```

- [ ] **Step 4: Verify compile**

```bash
CGO_ENABLED=1 go build ./internal/domain/...
```
Expected: clean

- [ ] **Step 5: Commit**

```bash
git add internal/domain/item.go internal/domain/integration.go internal/domain/settings.go
git commit -m "feat: add domain entities and value objects"
```

---

### Task 5: Port Interfaces

**Files:**
- Create: `internal/domain/port/store.go`
- Create: `internal/domain/port/sync.go`

These are the outbound port definitions — the domain's contracts with the outside world. No tests (interfaces). Adapter tests prove the contracts are satisfied.

- [ ] **Step 1: Write store.go**

```go
// internal/domain/port/store.go
package port

import (
	"context"
	"time"

	"github.com/churndesk/churndesk/internal/domain"
)

// ItemStore is the outbound port for persisting and querying feed items.
type ItemStore interface {
	Upsert(ctx context.Context, items []domain.Item) error
	ListRanked(ctx context.Context, limit int) ([]domain.Item, error) // dismissed=0 only, total_score DESC
	Count(ctx context.Context) (int, error)                           // non-dismissed items only
	Dismiss(ctx context.Context, id string) error                     // sets dismissed=1
	Delete(ctx context.Context, id string) error                      // hard-deletes (pr_approved revocation)
	MarkSeen(ctx context.Context, id string) error
	// MarkSeenByPR marks all items WHERE source='github' AND pr_owner=? AND pr_repo=? AND external_id=?
	MarkSeenByPR(ctx context.Context, prOwner, prRepo string, prNumber int) error
	// MarkSeenByJiraKey marks all items WHERE source='jira' AND external_id=?
	MarkSeenByJiraKey(ctx context.Context, jiraKey string) error
	// RescoreAll updates base_score, prerequisites_met, age_boost, and total_score for ALL items
	// (including dismissed ones). The re-scorer is the canonical authority for prerequisites_met.
	// prerequisiteUsernames: GitHub usernames from review_prerequisites that must appear in
	// metadata.reviews with state "APPROVED". If the slice is empty, all items get prerequisites_met=1.
	// Age_boost = prerequisites_met ? min(floor(hours)*ageMultiplier, maxAgeBoost) : 0
	RescoreAll(ctx context.Context, weights map[domain.ItemType]int, prerequisiteUsernames []string, ageMultiplier, maxAgeBoost float64) error
}

// LinkStore is the outbound port for PR ↔ Jira issue relationships.
type LinkStore interface {
	// UpsertPRJiraLinks records Jira issue keys referenced by a PR (title/body/branch).
	// Existing links are preserved; pr_title is updated to reflect latest sync.
	UpsertPRJiraLinks(ctx context.Context, prOwner, prRepo string, prNumber int, prTitle string, jiraKeys []string) error
	GetJiraKeysForPR(ctx context.Context, prOwner, prRepo string, prNumber int) ([]string, error)
	GetPRsForJiraKey(ctx context.Context, jiraKey string) ([]domain.PRRef, error)
}

// IntegrationStore is the outbound port for integration configuration.
type IntegrationStore interface {
	CreateIntegration(ctx context.Context, i domain.Integration) (int, error)
	GetIntegration(ctx context.Context, id int) (*domain.Integration, error)
	UpdateIntegration(ctx context.Context, i domain.Integration) error
	DeleteIntegration(ctx context.Context, id int) error
	ListIntegrations(ctx context.Context) ([]domain.Integration, error)
	UpdateLastSyncedAt(ctx context.Context, id int, t time.Time) error

	CreateSpace(ctx context.Context, s domain.Space) (int, error)
	ListSpaces(ctx context.Context, integrationID int) ([]domain.Space, error)
	UpdateSpace(ctx context.Context, s domain.Space) error
	DeleteSpace(ctx context.Context, id int) error

	CreateTeammate(ctx context.Context, t domain.Teammate) error
	ListTeammates(ctx context.Context, integrationID int) ([]domain.Teammate, error)
	DeleteTeammate(ctx context.Context, id int) error

	CreatePrerequisite(ctx context.Context, p domain.ReviewPrerequisite) error
	ListPrerequisites(ctx context.Context, integrationID int) ([]domain.ReviewPrerequisite, error)
	DeletePrerequisite(ctx context.Context, id int) error

	// IsOnboardingComplete returns true if at least one enabled integration has
	// at least one enabled space. Both GitHub-only and Jira-only setups are valid.
	IsOnboardingComplete(ctx context.Context) (bool, error)
}

// SettingsStore is the outbound port for app settings and category weights.
type SettingsStore interface {
	Get(ctx context.Context, key domain.SettingKey) (string, error)
	Set(ctx context.Context, key domain.SettingKey, value string) error
	GetAll(ctx context.Context) (map[domain.SettingKey]string, error)
	GetCategoryWeights(ctx context.Context) ([]domain.CategoryWeight, error)
	// SetCategoryWeight clamps weight to [1, 100] before persisting.
	SetCategoryWeight(ctx context.Context, itemType domain.ItemType, weight int) error
}
```

- [ ] **Step 2: Write sync.go**

```go
// internal/domain/port/sync.go
package port

import (
	"context"

	"github.com/churndesk/churndesk/internal/domain"
)

// Fetcher is the outbound port for fetching items from an external integration.
// Each integration provider (GitHub, Jira) has its own Fetcher implementation.
// Items with Deleted=true signal the app worker to hard-delete them.
type Fetcher interface {
	Fetch(ctx context.Context, integration domain.Integration, spaces []domain.Space) ([]domain.Item, error)
}

// GitHubClient is the outbound port for GitHub API operations.
// Implemented by internal/adapter/github/client.go wrapping go-github/v68.
// All methods translate go-github types to domain types — callers never import go-github.
type GitHubClient interface {
	GetPR(ctx context.Context, owner, repo string, number int) (*domain.PRDetail, error)
	ListPRComments(ctx context.Context, owner, repo string, number int) ([]domain.Comment, error)
	ListPRReviews(ctx context.Context, owner, repo string, number int) ([]domain.Review, error)
	ListCheckRuns(ctx context.Context, owner, repo string, headSHA string) ([]domain.CheckRun, error)
	PostPRComment(ctx context.Context, owner, repo string, number int, body string) error
	SubmitReview(ctx context.Context, owner, repo string, number int, verdict, body string) error
	RequestReviewers(ctx context.Context, owner, repo string, number int, logins []string) error
	ListPRsForRepo(ctx context.Context, owner, repo string) ([]*domain.PRDetail, error)
}

// JiraClient is the outbound port for Jira API operations.
// Implemented by internal/adapter/jira/client.go wrapping go-atlassian.
// All methods translate go-atlassian types to domain types.
type JiraClient interface {
	GetIssue(ctx context.Context, key string) (*domain.JiraIssue, error)
	ListIssueComments(ctx context.Context, key string) ([]domain.Comment, error)
	PostComment(ctx context.Context, key string, body string) error
	SearchIssues(ctx context.Context, jql string) ([]*domain.JiraIssue, error)
	ListBoards(ctx context.Context, projectKey, boardType string) ([]*domain.Board, error)
	// GetActiveSprintIssues resolves the active sprint ID internally, then fetches sprint issues.
	GetActiveSprintIssues(ctx context.Context, boardID int) ([]*domain.JiraIssue, error)
}
```

- [ ] **Step 3: Verify compile**

```bash
CGO_ENABLED=1 go build ./internal/domain/...
```
Expected: clean

- [ ] **Step 4: Commit**

```bash
git add internal/domain/port/
git commit -m "feat: add domain port interfaces (stores, fetcher, API clients)"
```

---

