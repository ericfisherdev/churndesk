## Chunk 2: SQLite Adapters

All SQLite adapter implementations live in `internal/adapter/sqlite/`. They import `internal/domain` and `internal/domain/port`, implementing the port interfaces. Tests use a real file-based SQLite via `db.Open`.

**Shared test helper** — create once, used by all adapter tests:

```go
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
	t.Cleanup(func() { conn.Close() })
	return conn
}
```

---

### Task 6: SQLite ItemStore Adapter

**Files:**
- Create: `internal/adapter/sqlite/item_store.go`
- Create: `internal/adapter/sqlite/item_store_test.go`
- Create: `internal/adapter/sqlite/testhelper_test.go`

- [ ] **Step 1: Write the failing tests**

```go
// internal/adapter/sqlite/item_store_test.go
package sqlite_test

import (
	"context"
	"testing"
	"time"

	"github.com/churndesk/churndesk/internal/adapter/sqlite"
	"github.com/churndesk/churndesk/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func makeItem(id string, score float64, itemType domain.ItemType) domain.Item {
	return domain.Item{
		ID: id, Source: "github", Type: itemType,
		ExternalID: "42", Title: "Test PR",
		BaseScore: int(score), TotalScore: score, PrerequisitesMet: 1,
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
}

func TestItemStore_UpsertAndListRanked(t *testing.T) {
	s := sqlite.NewItemStore(openTestDB(t))
	ctx := context.Background()

	require.NoError(t, s.Upsert(ctx, []domain.Item{
		makeItem("github:review_needed:1", 60, domain.ItemTypePRReviewNeeded),
		makeItem("github:ci_failing:2", 90, domain.ItemTypePRCIFailing),
	}))

	ranked, err := s.ListRanked(ctx, 10)
	require.NoError(t, err)
	require.Len(t, ranked, 2)
	assert.Equal(t, "github:ci_failing:2", ranked[0].ID)
	assert.Equal(t, "github:review_needed:1", ranked[1].ID)
}

func TestItemStore_DismissExcludesFromFeed(t *testing.T) {
	s := sqlite.NewItemStore(openTestDB(t))
	ctx := context.Background()

	require.NoError(t, s.Upsert(ctx, []domain.Item{makeItem("github:review_needed:1", 60, domain.ItemTypePRReviewNeeded)}))
	require.NoError(t, s.Dismiss(ctx, "github:review_needed:1"))

	ranked, err := s.ListRanked(ctx, 10)
	require.NoError(t, err)
	assert.Empty(t, ranked)
}

func TestItemStore_Delete(t *testing.T) {
	s := sqlite.NewItemStore(openTestDB(t))
	ctx := context.Background()

	require.NoError(t, s.Upsert(ctx, []domain.Item{makeItem("github:approved:1", 30, domain.ItemTypePRApproved)}))
	require.NoError(t, s.Delete(ctx, "github:approved:1"))

	ranked, err := s.ListRanked(ctx, 10)
	require.NoError(t, err)
	assert.Empty(t, ranked)
}

func TestItemStore_Count(t *testing.T) {
	s := sqlite.NewItemStore(openTestDB(t))
	ctx := context.Background()

	require.NoError(t, s.Upsert(ctx, []domain.Item{
		makeItem("github:review_needed:1", 60, domain.ItemTypePRReviewNeeded),
		makeItem("github:review_needed:2", 60, domain.ItemTypePRReviewNeeded),
	}))
	require.NoError(t, s.Dismiss(ctx, "github:review_needed:1"))

	count, err := s.Count(ctx)
	require.NoError(t, err)
	assert.Equal(t, 1, count)
}

func TestItemStore_MarkSeenByPR(t *testing.T) {
	s := sqlite.NewItemStore(openTestDB(t))
	ctx := context.Background()

	items := []domain.Item{
		{ID: "github:review_needed:42", Source: "github", Type: domain.ItemTypePRReviewNeeded,
			ExternalID: "42", PROwner: "myorg", PRRepo: "myrepo", Title: "PR",
			TotalScore: 60, PrerequisitesMet: 1, CreatedAt: time.Now(), UpdatedAt: time.Now()},
		{ID: "github:comment:42", Source: "github", Type: domain.ItemTypePRNewComment,
			ExternalID: "42", PROwner: "myorg", PRRepo: "myrepo", Title: "PR comment",
			TotalScore: 50, PrerequisitesMet: 1, CreatedAt: time.Now(), UpdatedAt: time.Now()},
		{ID: "github:review_needed:99", Source: "github", Type: domain.ItemTypePRReviewNeeded,
			ExternalID: "99", PROwner: "myorg", PRRepo: "myrepo", Title: "Other PR",
			TotalScore: 60, PrerequisitesMet: 1, CreatedAt: time.Now(), UpdatedAt: time.Now()},
	}
	require.NoError(t, s.Upsert(ctx, items))
	require.NoError(t, s.MarkSeenByPR(ctx, "myorg", "myrepo", 42))

	ranked, err := s.ListRanked(ctx, 10)
	require.NoError(t, err)
	seenMap := map[string]int{}
	for _, it := range ranked {
		seenMap[it.ID] = it.Seen
	}
	assert.Equal(t, 1, seenMap["github:review_needed:42"])
	assert.Equal(t, 1, seenMap["github:comment:42"])
	assert.Equal(t, 0, seenMap["github:review_needed:99"])
}

func TestItemStore_MarkSeenByJiraKey(t *testing.T) {
	s := sqlite.NewItemStore(openTestDB(t))
	ctx := context.Background()

	items := []domain.Item{
		{ID: "jira:status_change:FRONT-441", Source: "jira", Type: domain.ItemTypeJiraStatusChange,
			ExternalID: "FRONT-441", Title: "Status", TotalScore: 40, PrerequisitesMet: 1,
			CreatedAt: time.Now(), UpdatedAt: time.Now()},
		{ID: "jira:comment:FRONT-441", Source: "jira", Type: domain.ItemTypeJiraComment,
			ExternalID: "FRONT-441", Title: "Comment", TotalScore: 40, PrerequisitesMet: 1,
			CreatedAt: time.Now(), UpdatedAt: time.Now()},
		{ID: "jira:new_bug:FRONT-999", Source: "jira", Type: domain.ItemTypeJiraNewBug,
			ExternalID: "FRONT-999", Title: "Bug", TotalScore: 70, PrerequisitesMet: 1,
			CreatedAt: time.Now(), UpdatedAt: time.Now()},
	}
	require.NoError(t, s.Upsert(ctx, items))
	require.NoError(t, s.MarkSeenByJiraKey(ctx, "FRONT-441"))

	ranked, err := s.ListRanked(ctx, 10)
	require.NoError(t, err)
	seenMap := map[string]int{}
	for _, it := range ranked {
		seenMap[it.ID] = it.Seen
	}
	assert.Equal(t, 1, seenMap["jira:status_change:FRONT-441"])
	assert.Equal(t, 1, seenMap["jira:comment:FRONT-441"])
	assert.Equal(t, 0, seenMap["jira:new_bug:FRONT-999"])
}

func TestItemStore_RescoreAll_NoPrerequisites(t *testing.T) {
	s := sqlite.NewItemStore(openTestDB(t))
	ctx := context.Background()

	old := makeItem("github:review_needed:1", 60, domain.ItemTypePRReviewNeeded)
	old.CreatedAt = time.Now().Add(-2 * time.Hour)
	old.UpdatedAt = old.CreatedAt
	// metadata has no reviews (empty), no prerequisites configured
	old.Metadata = `{"reviews":[]}`
	require.NoError(t, s.Upsert(ctx, []domain.Item{old}))

	weights := map[domain.ItemType]int{domain.ItemTypePRReviewNeeded: 60}
	// empty prerequisiteUsernames → all items get prerequisites_met=1
	require.NoError(t, s.RescoreAll(ctx, weights, []string{}, 0.5, 50))

	ranked, err := s.ListRanked(ctx, 10)
	require.NoError(t, err)
	require.Len(t, ranked, 1)
	assert.Equal(t, 1, ranked[0].PrerequisitesMet)
	assert.Equal(t, 60, ranked[0].BaseScore)
	assert.InDelta(t, 1.0, ranked[0].AgeBoost, 0.1)   // floor(2) * 0.5 = 1.0
	assert.InDelta(t, 61.0, ranked[0].TotalScore, 0.1) // 60 + 1
}

func TestItemStore_RescoreAll_PrerequisitesNotApproved(t *testing.T) {
	s := sqlite.NewItemStore(openTestDB(t))
	ctx := context.Background()

	old := makeItem("github:review_needed:1", 60, domain.ItemTypePRReviewNeeded)
	old.CreatedAt = time.Now().Add(-10 * time.Hour)
	old.UpdatedAt = old.CreatedAt
	// copilot[bot] has not approved — CHANGES_REQUESTED
	old.Metadata = `{"reviews":[{"login":"copilot[bot]","state":"CHANGES_REQUESTED"}]}`
	require.NoError(t, s.Upsert(ctx, []domain.Item{old}))

	weights := map[domain.ItemType]int{domain.ItemTypePRReviewNeeded: 60}
	require.NoError(t, s.RescoreAll(ctx, weights, []string{"copilot[bot]"}, 0.5, 50))

	ranked, err := s.ListRanked(ctx, 10)
	require.NoError(t, err)
	require.Len(t, ranked, 1)
	assert.Equal(t, 0, ranked[0].PrerequisitesMet)      // not approved → prerequisites not met
	assert.InDelta(t, 0.0, ranked[0].AgeBoost, 0.001)  // age_boost suppressed
	assert.InDelta(t, 60.0, ranked[0].TotalScore, 0.001)
}

func TestItemStore_RescoreAll_PrerequisitesMet(t *testing.T) {
	s := sqlite.NewItemStore(openTestDB(t))
	ctx := context.Background()

	old := makeItem("github:review_needed:1", 60, domain.ItemTypePRReviewNeeded)
	old.CreatedAt = time.Now().Add(-4 * time.Hour)
	old.UpdatedAt = old.CreatedAt
	// copilot[bot] has approved
	old.Metadata = `{"reviews":[{"login":"copilot[bot]","state":"APPROVED"}]}`
	require.NoError(t, s.Upsert(ctx, []domain.Item{old}))

	weights := map[domain.ItemType]int{domain.ItemTypePRReviewNeeded: 60}
	require.NoError(t, s.RescoreAll(ctx, weights, []string{"copilot[bot]"}, 0.5, 50))

	ranked, err := s.ListRanked(ctx, 10)
	require.NoError(t, err)
	require.Len(t, ranked, 1)
	assert.Equal(t, 1, ranked[0].PrerequisitesMet)
	assert.InDelta(t, 2.0, ranked[0].AgeBoost, 0.1)   // floor(4) * 0.5 = 2.0
	assert.InDelta(t, 62.0, ranked[0].TotalScore, 0.1) // 60 + 2
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
CGO_ENABLED=1 go test ./internal/adapter/sqlite/...
```
Expected: FAIL

- [ ] **Step 3: Implement ItemStore adapter**

```go
// internal/adapter/sqlite/item_store.go
package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"math"
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
	defer tx.Rollback()

	stmt, err := tx.PrepareContext(ctx, `
		INSERT INTO items
			(id, source, type, external_id, title, url, metadata, base_score,
			 age_boost, total_score, pr_owner, pr_repo, dismissed, prerequisites_met,
			 seen, created_at, updated_at)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
		ON CONFLICT(id) DO UPDATE SET
			title       = excluded.title,
			url         = excluded.url,
			metadata    = excluded.metadata,
			total_score = excluded.total_score,
			updated_at  = excluded.updated_at
	`)
	if err != nil {
		return fmt.Errorf("prepare upsert: %w", err)
	}
	defer stmt.Close()

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
	defer rows.Close()
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
		 WHERE source = 'github' AND pr_owner = ? AND pr_repo = ? AND external_id = ?`,
		time.Now().UTC(), prOwner, prRepo, fmt.Sprintf("%d", prNumber),
	)
	return err
}

func (s *itemStore) MarkSeenByJiraKey(ctx context.Context, jiraKey string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE items SET seen = 1, updated_at = ?
		 WHERE source = 'jira' AND external_id = ?`,
		time.Now().UTC(), jiraKey,
	)
	return err
}

// reviewEntry matches the reviews array in item metadata JSON.
type reviewEntry struct {
	Login string `json:"login"`
	State string `json:"state"`
}

// metadataReviews is the subset of metadata we parse for prerequisite checking.
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
	defer rows.Close()

	type row struct {
		id       string
		itemType domain.ItemType
		metadata string
		createdAt time.Time
	}
	var toRescore []row
	for rows.Next() {
		var r row
		if err := rows.Scan(&r.id, &r.itemType, &r.metadata, &r.createdAt); err != nil {
			return err
		}
		toRescore = append(toRescore, r)
	}
	if err := rows.Err(); err != nil {
		return err
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
	defer tx.Rollback()

	stmt, err := tx.PrepareContext(ctx,
		`UPDATE items SET base_score = ?, prerequisites_met = ?, age_boost = ?, total_score = ?, updated_at = ? WHERE id = ?`,
	)
	if err != nil {
		return err
	}
	defer stmt.Close()

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

// computePrerequisitesMet returns 1 if all required bots have APPROVED the PR.
// If prereqSet is empty, returns 1 (no prerequisites configured).
// Silently returns 0 on unparseable metadata rather than failing the whole rescore.
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

// scanItems reads all rows into []domain.Item.
// Requires _loc=UTC in DSN so datetime columns scan directly into time.Time.
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
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
CGO_ENABLED=1 go test ./internal/adapter/sqlite/... -run TestItemStore
```
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/adapter/sqlite/
git commit -m "feat: add SQLite ItemStore adapter implementing port.ItemStore"
```

---

### Task 7: SQLite LinkStore Adapter

**Files:**
- Create: `internal/adapter/sqlite/link_store.go`
- Create: `internal/adapter/sqlite/link_store_test.go`

- [ ] **Step 1: Write the failing tests**

```go
// internal/adapter/sqlite/link_store_test.go
package sqlite_test

import (
	"context"
	"testing"

	"github.com/churndesk/churndesk/internal/adapter/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLinkStore_UpsertAndGetJiraKeys(t *testing.T) {
	s := sqlite.NewLinkStore(openTestDB(t))
	ctx := context.Background()

	require.NoError(t, s.UpsertPRJiraLinks(ctx, "myorg", "myrepo", 42, "Fix login", []string{"FRONT-1", "BACK-2"}))

	keys, err := s.GetJiraKeysForPR(ctx, "myorg", "myrepo", 42)
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"FRONT-1", "BACK-2"}, keys)
}

func TestLinkStore_UpsertUpdatesPRTitle(t *testing.T) {
	s := sqlite.NewLinkStore(openTestDB(t))
	ctx := context.Background()

	require.NoError(t, s.UpsertPRJiraLinks(ctx, "myorg", "myrepo", 42, "Old title", []string{"FRONT-1"}))
	require.NoError(t, s.UpsertPRJiraLinks(ctx, "myorg", "myrepo", 42, "New title", []string{"FRONT-1"}))

	prs, err := s.GetPRsForJiraKey(ctx, "FRONT-1")
	require.NoError(t, err)
	require.Len(t, prs, 1)
	assert.Equal(t, "New title", prs[0].Title) // pr_title updated to reflect latest sync
}

func TestLinkStore_GetPRsForJiraKey(t *testing.T) {
	s := sqlite.NewLinkStore(openTestDB(t))
	ctx := context.Background()

	require.NoError(t, s.UpsertPRJiraLinks(ctx, "myorg", "myrepo", 42, "Fix login", []string{"FRONT-1"}))
	require.NoError(t, s.UpsertPRJiraLinks(ctx, "myorg", "myrepo", 99, "Fix timeout", []string{"FRONT-1"}))

	prs, err := s.GetPRsForJiraKey(ctx, "FRONT-1")
	require.NoError(t, err)
	assert.Len(t, prs, 2)
	numbers := []int{prs[0].Number, prs[1].Number}
	assert.ElementsMatch(t, []int{42, 99}, numbers)
}

func TestLinkStore_EmptyJiraKeys_IsNoOp(t *testing.T) {
	s := sqlite.NewLinkStore(openTestDB(t))
	ctx := context.Background()

	require.NoError(t, s.UpsertPRJiraLinks(ctx, "myorg", "myrepo", 42, "PR", []string{}))
	keys, err := s.GetJiraKeysForPR(ctx, "myorg", "myrepo", 42)
	require.NoError(t, err)
	assert.Empty(t, keys)
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
CGO_ENABLED=1 go test ./internal/adapter/sqlite/... -run TestLinkStore
```
Expected: FAIL

- [ ] **Step 3: Implement LinkStore adapter**

```go
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
	if len(jiraKeys) == 0 {
		return nil
	}
	// ON CONFLICT updates pr_title to reflect PR title at time of last sync (spec §4.4).
	stmt, err := s.db.PrepareContext(ctx, `
		INSERT INTO pr_jira_links (pr_owner, pr_repo, pr_number, pr_title, jira_issue_key)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(pr_owner, pr_repo, pr_number, jira_issue_key)
		DO UPDATE SET pr_title = excluded.pr_title
	`)
	if err != nil {
		return fmt.Errorf("prepare link upsert: %w", err)
	}
	defer stmt.Close()

	for _, key := range jiraKeys {
		if _, err := stmt.ExecContext(ctx, prOwner, prRepo, prNumber, prTitle, key); err != nil {
			return fmt.Errorf("insert link %s: %w", key, err)
		}
	}
	return nil
}

func (s *linkStore) GetJiraKeysForPR(ctx context.Context, prOwner, prRepo string, prNumber int) ([]string, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT jira_issue_key FROM pr_jira_links WHERE pr_owner=? AND pr_repo=? AND pr_number=?`,
		prOwner, prRepo, prNumber,
	)
	if err != nil {
		return nil, fmt.Errorf("get jira keys: %w", err)
	}
	defer rows.Close()
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
	defer rows.Close()
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
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
CGO_ENABLED=1 go test ./internal/adapter/sqlite/... -run TestLinkStore
```
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/adapter/sqlite/link_store.go internal/adapter/sqlite/link_store_test.go
git commit -m "feat: add SQLite LinkStore adapter implementing port.LinkStore"
```

---

### Task 8: SQLite IntegrationStore Adapter

**Files:**
- Create: `internal/adapter/sqlite/integration_store.go`
- Create: `internal/adapter/sqlite/integration_store_test.go`

- [ ] **Step 1: Write the failing tests**

```go
// internal/adapter/sqlite/integration_store_test.go
package sqlite_test

import (
	"context"
	"testing"
	"time"

	"github.com/churndesk/churndesk/internal/adapter/sqlite"
	"github.com/churndesk/churndesk/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIntegrationStore_CreateAndGet(t *testing.T) {
	s := sqlite.NewIntegrationStore(openTestDB(t))
	ctx := context.Background()

	id, err := s.CreateIntegration(ctx, domain.Integration{
		Provider: domain.ProviderGitHub, AccessToken: "tok",
		Username: "alice", PollIntervalSeconds: 300, Enabled: true,
	})
	require.NoError(t, err)
	assert.Greater(t, id, 0)

	got, err := s.GetIntegration(ctx, id)
	require.NoError(t, err)
	assert.Equal(t, domain.ProviderGitHub, got.Provider)
	assert.Equal(t, "alice", got.Username)
	assert.Nil(t, got.LastSyncedAt)
}

func TestIntegrationStore_UpdateLastSyncedAt(t *testing.T) {
	s := sqlite.NewIntegrationStore(openTestDB(t))
	ctx := context.Background()

	id, _ := s.CreateIntegration(ctx, domain.Integration{Provider: domain.ProviderGitHub, Enabled: true})
	now := time.Now().Truncate(time.Second)
	require.NoError(t, s.UpdateLastSyncedAt(ctx, id, now))

	got, err := s.GetIntegration(ctx, id)
	require.NoError(t, err)
	require.NotNil(t, got.LastSyncedAt)
	assert.WithinDuration(t, now, *got.LastSyncedAt, time.Second)
}

func TestIntegrationStore_SpaceCRUD(t *testing.T) {
	s := sqlite.NewIntegrationStore(openTestDB(t))
	ctx := context.Background()

	intID, _ := s.CreateIntegration(ctx, domain.Integration{Provider: domain.ProviderGitHub, Enabled: true})
	spaceID, err := s.CreateSpace(ctx, domain.Space{
		IntegrationID: intID, Provider: domain.ProviderGitHub,
		Owner: "myorg", Name: "myrepo", Enabled: true,
	})
	require.NoError(t, err)
	assert.Greater(t, spaceID, 0)

	spaces, err := s.ListSpaces(ctx, intID)
	require.NoError(t, err)
	require.Len(t, spaces, 1)
	assert.Equal(t, "myrepo", spaces[0].Name)
	assert.Empty(t, spaces[0].BoardType) // nullable, empty for GitHub spaces

	require.NoError(t, s.DeleteSpace(ctx, spaceID))
	spaces, _ = s.ListSpaces(ctx, intID)
	assert.Empty(t, spaces)
}

func TestIntegrationStore_IsOnboardingComplete(t *testing.T) {
	s := sqlite.NewIntegrationStore(openTestDB(t))
	ctx := context.Background()

	ok, err := s.IsOnboardingComplete(ctx)
	require.NoError(t, err)
	assert.False(t, ok) // empty DB

	intID, _ := s.CreateIntegration(ctx, domain.Integration{Provider: domain.ProviderGitHub, Enabled: true})
	ok, _ = s.IsOnboardingComplete(ctx)
	assert.False(t, ok) // integration but no spaces

	_, _ = s.CreateSpace(ctx, domain.Space{IntegrationID: intID, Provider: domain.ProviderGitHub, Name: "repo", Enabled: true})
	ok, err = s.IsOnboardingComplete(ctx)
	require.NoError(t, err)
	assert.True(t, ok)
}

func TestIntegrationStore_TeammateAndPrerequisiteCRUD(t *testing.T) {
	s := sqlite.NewIntegrationStore(openTestDB(t))
	ctx := context.Background()

	intID, _ := s.CreateIntegration(ctx, domain.Integration{Provider: domain.ProviderGitHub, Enabled: true})

	require.NoError(t, s.CreateTeammate(ctx, domain.Teammate{IntegrationID: intID, GitHubUsername: "bob"}))
	teammates, err := s.ListTeammates(ctx, intID)
	require.NoError(t, err)
	require.Len(t, teammates, 1)
	assert.Equal(t, "bob", teammates[0].GitHubUsername)
	require.NoError(t, s.DeleteTeammate(ctx, teammates[0].ID))

	require.NoError(t, s.CreatePrerequisite(ctx, domain.ReviewPrerequisite{
		IntegrationID: intID, GitHubUsername: "copilot[bot]", DisplayName: "Copilot",
	}))
	prereqs, err := s.ListPrerequisites(ctx, intID)
	require.NoError(t, err)
	require.Len(t, prereqs, 1)
	assert.Equal(t, "copilot[bot]", prereqs[0].GitHubUsername)
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
CGO_ENABLED=1 go test ./internal/adapter/sqlite/... -run TestIntegrationStore
```
Expected: FAIL

- [ ] **Step 3: Implement IntegrationStore adapter**

```go
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
		`INSERT INTO integrations (provider, access_token, base_url, username, poll_interval_seconds, enabled)
		 VALUES (?,?,?,?,?,?)`,
		string(i.Provider), i.AccessToken, i.BaseURL, i.Username, i.PollIntervalSeconds, boolToInt(i.Enabled),
	)
	if err != nil {
		return 0, fmt.Errorf("create integration: %w", err)
	}
	id, _ := res.LastInsertId()
	return int(id), nil
}

func (s *integrationStore) GetIntegration(ctx context.Context, id int) (*domain.Integration, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, provider, access_token, base_url, username, poll_interval_seconds, last_synced_at, enabled
		 FROM integrations WHERE id = ?`, id,
	)
	return scanIntegration(row)
}

func (s *integrationStore) UpdateIntegration(ctx context.Context, i domain.Integration) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE integrations SET provider=?, access_token=?, base_url=?, username=?,
		 poll_interval_seconds=?, enabled=? WHERE id=?`,
		string(i.Provider), i.AccessToken, i.BaseURL, i.Username,
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
		`SELECT id, provider, access_token, base_url, username, poll_interval_seconds, last_synced_at, enabled
		 FROM integrations ORDER BY id`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
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
	defer rows.Close()
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
	defer rows.Close()
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
	defer rows.Close()
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

// scanIntegration accepts both *sql.Row and *sql.Rows via interface.
// Requires _loc=UTC in DSN for correct time.Time scanning of last_synced_at.
func scanIntegration(row interface{ Scan(...any) error }) (*domain.Integration, error) {
	var i domain.Integration
	var provider string
	var lastSynced sql.NullTime
	var enabled int
	if err := row.Scan(&i.ID, &provider, &i.AccessToken, &i.BaseURL, &i.Username,
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
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
CGO_ENABLED=1 go test ./internal/adapter/sqlite/... -run TestIntegrationStore
```
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/adapter/sqlite/integration_store.go internal/adapter/sqlite/integration_store_test.go
git commit -m "feat: add SQLite IntegrationStore adapter implementing port.IntegrationStore"
```

---

### Task 9: SQLite SettingsStore Adapter

**Files:**
- Create: `internal/adapter/sqlite/settings_store.go`
- Create: `internal/adapter/sqlite/settings_store_test.go`

- [ ] **Step 1: Write the failing tests**

```go
// internal/adapter/sqlite/settings_store_test.go
package sqlite_test

import (
	"context"
	"testing"

	"github.com/churndesk/churndesk/internal/adapter/sqlite"
	"github.com/churndesk/churndesk/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSettingsStore_GetAndSet(t *testing.T) {
	s := sqlite.NewSettingsStore(openTestDB(t))
	ctx := context.Background()

	val, err := s.Get(ctx, domain.SettingAutoRefreshInterval)
	require.NoError(t, err)
	assert.Equal(t, "20", val) // seeded by migration

	require.NoError(t, s.Set(ctx, domain.SettingAutoRefreshInterval, "30"))
	val, _ = s.Get(ctx, domain.SettingAutoRefreshInterval)
	assert.Equal(t, "30", val)
}

func TestSettingsStore_GetAll(t *testing.T) {
	s := sqlite.NewSettingsStore(openTestDB(t))
	ctx := context.Background()

	all, err := s.GetAll(ctx)
	require.NoError(t, err)
	assert.Equal(t, "20", all[domain.SettingAutoRefreshInterval])
	assert.Equal(t, "0.5", all[domain.SettingAgeMultiplier])
	assert.Equal(t, "1", all[domain.SettingFeedColumns])
}

func TestSettingsStore_CategoryWeights(t *testing.T) {
	s := sqlite.NewSettingsStore(openTestDB(t))
	ctx := context.Background()

	weights, err := s.GetCategoryWeights(ctx)
	require.NoError(t, err)
	assert.Len(t, weights, 9)

	wmap := map[domain.ItemType]int{}
	for _, w := range weights {
		wmap[w.ItemType] = w.Weight
	}
	assert.Equal(t, 90, wmap[domain.ItemTypePRCIFailing])
	assert.Equal(t, 30, wmap[domain.ItemTypePRApproved])

	require.NoError(t, s.SetCategoryWeight(ctx, domain.ItemTypePRReviewNeeded, 75))
	weights, _ = s.GetCategoryWeights(ctx)
	wmap = map[domain.ItemType]int{}
	for _, w := range weights {
		wmap[w.ItemType] = w.Weight
	}
	assert.Equal(t, 75, wmap[domain.ItemTypePRReviewNeeded])
}

func TestSettingsStore_SetCategoryWeight_Clamps(t *testing.T) {
	s := sqlite.NewSettingsStore(openTestDB(t))
	ctx := context.Background()

	require.NoError(t, s.SetCategoryWeight(ctx, domain.ItemTypePRCIFailing, 200))
	weights, _ := s.GetCategoryWeights(ctx)
	for _, w := range weights {
		if w.ItemType == domain.ItemTypePRCIFailing {
			assert.Equal(t, 100, w.Weight)
		}
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
CGO_ENABLED=1 go test ./internal/adapter/sqlite/... -run TestSettingsStore
```
Expected: FAIL

- [ ] **Step 3: Implement SettingsStore adapter**

```go
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
```

- [ ] **Step 4: Run all adapter tests**

```bash
CGO_ENABLED=1 go test ./internal/adapter/sqlite/... -v
```
Expected: all PASS

- [ ] **Step 5: Commit**

```bash
git add internal/adapter/sqlite/settings_store.go internal/adapter/sqlite/settings_store_test.go \
        internal/adapter/sqlite/testhelper_test.go
git commit -m "feat: add SQLite SettingsStore adapter implementing port.SettingsStore"
```

---
