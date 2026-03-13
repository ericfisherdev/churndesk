# Churndesk — Design Spec

**Date:** 2026-03-13
**Status:** Approved

---

## 1. Overview

Churndesk is a locally-run developer task aggregator that consolidates actionable items from GitHub and Jira into a single priority-ranked feed. It runs as a Docker container, persists data in SQLite, and is accessible via a browser at `localhost:8080`.

The core value proposition: a developer opens one page in the morning and sees everything that needs their attention, ranked by urgency, without context-switching between tools.

---

## 2. Tech Stack

| Layer | Technology |
|-------|-----------|
| Backend | Go |
| Templating | Templ (Go templates) |
| Interactivity | HTMX + Alpine.js |
| Animations | GSAP |
| Database | SQLite (via CGO, `mattn/go-sqlite3`) |
| Container | Docker (multi-stage build, `debian:bookworm-slim` final stage) |
| Distribution | Dockerhub (`churndesk/churndesk:latest`, `churndesk/churndesk:v{semver}`) |

Static assets (JS, CSS) are embedded into the binary via `embed.FS` declared in `internal/web/server.go`. Templ templates are compiled to Go at build time and are part of the binary — they are not served from the filesystem at runtime.

---

## 3. Architecture

### 3.1 Process Model

A single Go binary runs four concurrent components:

1. **HTTP Server** — serves the UI and handles all user actions
2. **Integration Workers** — one goroutine per integration (GitHub, Jira), each polling on its own configurable interval
3. **Re-scorer Goroutine** — runs every 60 seconds, recalculates `age_boost` and `total_score` for all non-dismissed items
4. **Manual Sync Handler** — HTTP endpoint (`POST /sync`) that triggers an immediate sync for a given integration and blocks until complete

### 3.2 Data Flow

```
Integration worker
  → fetches from external API
  → upserts raw items into SQLite (items table)
  → notifies re-scorer to run immediately

Re-scorer goroutine
  → reads all active items
  → computes age_boost = floor(hours_since_created) * age_multiplier
  → writes total_score = base_score + age_boost back to items table

HTMX auto-refresh (configurable interval, default 20s)
  → GET /feed (returns only the #feed fragment)
  → server queries items ordered by total_score DESC
  → renders ranked feed partial
  → if new items appeared since previous render, server sets
    HX-Trigger: {"newItems": N} response header
  → Alpine.js listens for htmx:afterSwap and reads HX-Trigger to show toasts
```

### 3.3 Onboarding Gate

The onboarding gate is enforced via **HTTP middleware** applied to all routes except `/settings`, `/static/*`, and action endpoints (`POST /sync`, `POST /items/:id/*`). On every request through the middleware, if the database has zero enabled integrations with at least one associated space, the request is redirected to `/settings` with a `?setup=1` query parameter that triggers the setup prompt in the UI.

This means: if a user configures an integration, uses the feed, then later removes all integrations via settings, the next page load redirects them back to `/settings` automatically. GitHub-only and Jira-only setups are both valid.

---

## 4. Data Model

### 4.1 `items`

The unified feed across all integrations.

| Column | Type | Notes |
|--------|------|-------|
| `id` | TEXT PK | `{source}:{external_id}` e.g. `github:PR-1042` |
| `source` | TEXT | `github` or `jira` |
| `type` | TEXT | See item types below |
| `external_id` | TEXT | ID in the source system |
| `title` | TEXT | Display title |
| `url` | TEXT | Deep link to the item |
| `metadata` | TEXT | JSON blob of source-specific fields |
| `base_score` | INTEGER | From `category_weights` at time of first upsert; not updated when weights change |
| `age_boost` | REAL | Computed by re-scorer; capped at `max_age_boost` setting (default 50) |
| `total_score` | REAL | `base_score + age_boost` |
| `dismissed` | INTEGER | 0 or 1 |
| `seen` | INTEGER | 0 or 1; set to 1 when user opens detail view; drives unread dot indicator |
| `created_at` | DATETIME | When item first appeared |
| `updated_at` | DATETIME | Last sync update |

**Notes on `base_score`:** Weight changes in settings apply only to items created after the change. This is intentional — retroactively rescoring all existing items could cause surprising reordering mid-session. Users who want weight changes to apply immediately can use the "Re-score all items" button in settings (triggers a one-time full rescore using current weights).

**Item types:**

| Type | Source | Description |
|------|--------|-------------|
| `pr_review_needed` | GitHub | Open PR from a tracked teammate with no/insufficient reviews |
| `pr_stale_review` | GitHub | PR where existing approval was dismissed (new commits pushed) |
| `pr_changes_requested` | GitHub | User's own PR has a change request |
| `pr_new_comment` | GitHub | New comment on user's own PR since last sync |
| `pr_ci_failing` | GitHub | User's own PR has failing CI checks |
| `pr_approved` | GitHub | User's own PR meets minimum review threshold; auto-dismissed when PR is merged or closed |
| `jira_status_change` | Jira | Assigned ticket status or priority changed |
| `jira_comment` | Jira | New comment on a ticket where the user is the author of any existing comment |
| `jira_new_bug` | Jira | New bug ticket (assigned or unassigned) in a tracked space |

**`pr_approved` lifecycle:** Created when a PR first meets the minimum review threshold. Auto-dismissed (set `dismissed=1`) by the GitHub fetcher when the PR is detected as merged or closed. If approvals are later revoked (dropping below the threshold), the item is deleted and a new `pr_review_needed` item is created for the same PR.

**`pr_new_comment` and `jira_comment` — `external_id` and upsert behavior:** These item types use the PR number (e.g. `1042`) or Jira issue key (e.g. `FRONT-441`) as their `external_id`, meaning there is exactly **one feed item per PR/issue**, not one per comment. When additional comments arrive on the same PR/issue before the user dismisses the item, the upsert updates `updated_at` and stores the latest comment in `metadata.latest_comment`. The item is not duplicated. This means `id` for these types follows the pattern `github:comment:1042` and `jira:comment:FRONT-441` to distinguish them from other item types targeting the same PR/issue.

**`pr_new_comment` trigger:** "New since last sync" means comments whose `created_at` timestamp in the GitHub API is after the integration's `last_synced_at`. This uses the integration clock, not the `seen` field.

### 4.2 `integrations`

One row per connected service.

| Column | Type | Notes |
|--------|------|-------|
| `id` | INTEGER PK | |
| `provider` | TEXT | `github` or `jira` |
| `access_token` | TEXT | OAuth token or PAT |
| `base_url` | TEXT | Jira instance URL (e.g. `https://myorg.atlassian.net`) |
| `username` | TEXT | GitHub username or Jira account ID |
| `poll_interval_seconds` | INTEGER | Default: 300 (5 min) |
| `last_synced_at` | DATETIME | |
| `enabled` | INTEGER | 0 or 1 |

### 4.3 `spaces`

Tracked GitHub repos and Jira project spaces.

| Column | Type | Notes |
|--------|------|-------|
| `id` | INTEGER PK | |
| `integration_id` | INTEGER FK | → `integrations.id` ON DELETE CASCADE |
| `provider` | TEXT | `github` or `jira` |
| `owner` | TEXT | GitHub org/user or Jira project key |
| `name` | TEXT | GitHub repo name or Jira space name |
| `board_type` | TEXT | `scrum` or `kanban` (Jira only; detected on first sync) |
| `jira_board_id` | INTEGER | Cached Jira board ID (Jira only) |
| `enabled` | INTEGER | 0 or 1 |

### 4.4 `teammates`

GitHub users to watch for PR review needs.

| Column | Type | Notes |
|--------|------|-------|
| `id` | INTEGER PK | |
| `integration_id` | INTEGER FK | → `integrations.id` ON DELETE CASCADE |
| `github_username` | TEXT | |
| `display_name` | TEXT | |

**Unique constraint:** `UNIQUE(integration_id, github_username)` enforced in migration to prevent duplicate teammate entries that would cause duplicate `pr_review_needed` items.

### 4.5 `category_weights`

User-configured base priority per item type.

| Column | Type | Notes |
|--------|------|-------|
| `item_type` | TEXT PK | Matches `items.type` |
| `weight` | INTEGER | 1–100, default varies by type |

**Default weights:**

| Type | Default Weight |
|------|---------------|
| `pr_ci_failing` | 90 |
| `pr_changes_requested` | 80 |
| `pr_stale_review` | 70 |
| `jira_new_bug` | 70 |
| `pr_review_needed` | 60 |
| `pr_new_comment` | 50 |
| `jira_status_change` | 40 |
| `jira_comment` | 40 |
| `pr_approved` | 30 |

### 4.6 `settings`

Key/value store for UI and behavioral configuration. All setting keys must be defined as typed constants in `internal/domain/settings.go` — no magic strings in application code.

| Key | Default | Description |
|-----|---------|-------------|
| `auto_refresh_interval` | `20` | Feed auto-refresh in seconds |
| `age_multiplier` | `0.5` | Points added per hour of age |
| `max_age_boost` | `50` | Cap on age_boost to prevent stale low-priority items from dominating |
| `feed_columns` | `1` | 1, 2, or 3 column layout; handler clamps value to `[1, 3]` before rendering |
| `min_review_count` | `1` | Min approvals for a PR to be considered approved |

---

## 5. Scoring

```
total_score = base_score + age_boost
age_boost   = min(floor(hours_since_created) * age_multiplier, max_age_boost)
```

- `base_score` is set from `category_weights` when an item is **first created**; not updated retroactively when weights change (see Section 4.1 notes)
- `age_boost` is capped at `max_age_boost` (default 50) to prevent very old low-priority items from dominating over fresh high-priority items
- `age_boost` is recalculated by the re-scorer goroutine every 60 seconds
- `total_score` is stored in the database; feed is sorted by `total_score DESC`
- Dismissed items are excluded from the feed query

---

## 6. Integration Sync Logic

### 6.1 GitHub Fetcher

Fetches for each tracked repo:
- Open PRs from tracked teammates → `pr_review_needed` (filtered by: no existing review from authenticated user, PR not authored by authenticated user, open review count < `min_review_count`)
- PRs where authenticated user's previous approval was `DISMISSED` → `pr_stale_review`
- Authenticated user's own open PRs:
  - Has a `CHANGES_REQUESTED` review → `pr_changes_requested`
  - Has comments with `created_at` after integration's `last_synced_at` → `pr_new_comment`
  - Has any failing/errored check run → `pr_ci_failing`
  - Has `>= min_review_count` `APPROVED` reviews and no `CHANGES_REQUESTED` → `pr_approved`
- Merged or closed PRs with a `pr_approved` item → mark that item `dismissed=1`

Stale review detection: The fetcher checks review state per PR. If any review has state `DISMISSED`, it surfaces as `pr_stale_review`. GitHub sets this automatically when new commits are pushed and branch protection has "Dismiss stale pull request approvals when new commits are pushed" enabled. **Prerequisite:** if this branch protection setting is not enabled on a repo, `pr_stale_review` items will never appear for that repo — this is documented in the settings UI as a note next to tracked repos.

### 6.2 Jira Fetcher

**Board detection (first sync only):**
1. Call `GET /rest/agile/1.0/board?projectKeyOrId={key}&type=scrum` — if results exist, use the first board as `scrum`
2. If no scrum board found, call `GET /rest/agile/1.0/board?projectKeyOrId={key}&type=kanban` — if results exist, use first as `kanban`
3. If multiple boards of the same type exist, use the one with the lowest ID (oldest board)
4. If zero boards are found for a space, log a warning and skip that space during sync
5. Store detected `board_type` and `jira_board_id` in `spaces` table

**Issue fetching (every sync):**
- **Scrum:** `GET /rest/agile/1.0/board/{id}/sprint?state=active` → get active sprint ID → `GET /rest/agile/1.0/sprint/{sprintId}/issue`
- **Kanban:** `GET /rest/api/3/search?jql=project={key} AND statusCategory != Done`

**Jira item `metadata` JSON schema:**
```json
{
  "key": "FRONT-441",
  "summary": "Fix login timeout",
  "status": "In Progress",
  "priority": "High",
  "assignee": "account-id-string",
  "issue_type": "Bug",
  "sprint": "Sprint 14",
  "story_points": 3,
  "latest_comment": "Looks like the session token is expired...",
  "latest_comment_author": "alice",
  "latest_comment_at": "2026-03-12T14:22:00Z"
}
```

**Surfacing items from fetched issues:**
- Issues assigned to user where current API `status` or `priority` differs from the values stored in `metadata` → `jira_status_change`; metadata is updated on upsert
- New comments (comment `created` timestamp after integration's `last_synced_at`) on issues where the authenticated user is the author of at least one existing comment → `jira_comment`. Watcher list is not used — comment authorship is the sole criterion.
- Issues with `issuetype.name == "Bug"` whose `created` timestamp is after `last_synced_at`, regardless of assignee → `jira_new_bug`

---

## 7. UI Design

### 7.1 Design System

Adapted from the MyGitPanel design — rebranded to Churndesk. Dark minimal aesthetic with a purple accent.

**Color tokens (defined as CSS custom properties in `style.css`):**
```css
--bg:           #0f0f10;   /* page background */
--surface:      #16161a;   /* panels, list hover */
--card:         #1c1c22;   /* cards, dropdowns */
--border:       rgba(255,255,255,0.07);
--border-solid: #2a2a33;
--text:         #e8e8ee;
--muted:        #7b7b8f;
--accent:       #7c6af7;   /* purple — primary actions, PR items */
--green:        #34d399;   /* approved, success */
--amber:        #fbbf24;   /* warnings, changes requested, Jira */
--red:          #f87171;   /* critical, CI failing, bugs */
--blue:         #60a5fa;   /* info, comments */
```

Soft variants at 12–15% opacity for backgrounds. Glow effects via `box-shadow`.

**Typography:** `-apple-system, BlinkMacSystemFont, 'Segoe UI', sans-serif` + `JetBrains Mono` for IDs/code.

### 7.2 Pages

**Main Feed (`/`)**
- Fixed header: Churndesk logo, sync status, manual refresh button (↻), settings link
- Feed body: 1/2/3 column layout (user-configurable), items sorted by `total_score DESC`
- HTMX auto-refresh polls `/feed` (fragment endpoint, not `/`):
  ```html
  <div id="feed"
       hx-get="/feed"
       hx-trigger="every 20s"
       hx-swap="innerHTML">
  ```
- `GET /feed` returns only the feed fragment (no layout wrapper). The client sends the current item count as a query parameter (`/feed?count=N`). The server compares `N` against the current ranked item count from the DB; if the DB count is greater, it sets `HX-Trigger: {"newItems": M}` in the response header. Alpine.js listens on `document` for `htmx:afterSwap` and reads `event.detail.xhr.getResponseHeader("HX-Trigger")` to show toasts. This avoids in-memory server state entirely — the comparison is stateless.
- Each feed item: left status icon, pills (type, source, repo/space), title, meta row (author, time, comment count), latest comment preview, unread dot (shown when `seen=0`), right dismiss button (eye-off, appears on hover)
- Item colors by type: purple (review needed), amber (stale/changes), red (CI failing/bugs), green (approved), blue (comments/status)

**Detail View**
- Triggered by clicking a feed item: HTMX loads the detail fragment via `GET /items/:id/detail` (returns an HTML partial, not a full page) into a `#detail-panel` div; Alpine.js toggles panel visibility with GSAP transitions. There is no URL navigation — the URL does not change.
- `GET /items/:id` is **not** a route. Deep-linking to a specific item is out of scope for v1.
- Sets `seen=1` via `POST /items/:id/seen` on open (fired by Alpine.js `x-init` in the detail partial)
- PR tabs: Conversation / Changes / Linked Jira
- Jira tabs: Comments / Ticket Details
- Back to inbox link returns to feed (GSAP slide back; `#detail-panel` hidden, `#feed` shown)

**Settings (`/settings`)**
- **Integrations section:** GitHub PAT + username, Jira base URL + API token + account ID
- **Tracked repos:** list of `owner/repo` with add/remove, per-repo enabled toggle
- **Jira spaces:** list of `project key` entries with add/remove, board type display (auto-detected on save)
- **Teammates:** GitHub usernames to watch, add/remove
- **Priority weights:** slider per item type (1–100) + "Re-score all items" button to apply current weights retroactively
- **Age multiplier:** number input (points per hour)
- **Max age boost:** number input (cap on age contribution)
- **Layout:** 1/2/3 column selector
- **Poll intervals:** per-integration (seconds)
- **Auto-refresh interval:** seconds (default 20)
- **Minimum review count:** number input

### 7.3 GSAP Animations

| Trigger | Effect |
|---------|--------|
| Page load | Header entrance: y -50→0, opacity 0→1, 0.7s power3.out |
| Page load | Feed items: stagger 0.07s, y 30→0, scale 0.97→1, opacity 0→1 |
| New item | Height expand 0→auto + opacity 0→1 (0.4s), glow pulse 3x. Use `gsap.from(el, {height: 0, overflow: "hidden", duration: 0.4})` — GSAP handles `height: "auto"` correctly when `overflow: hidden` is set on the element |
| Toast | y -30→0, scale 0.85→1, back.out(1.5), auto-dismiss after 4.4s |
| Detail open | Feed: opacity 0, x -30 (0.2s); Detail: opacity 0→1, x 30→0 (0.4s) |
| Back to feed | Detail: opacity 0, x +30 (0.2s); Feed: opacity 0→1, x -30→0 (0.35s) |

### 7.4 Toast Notifications

Appear at top-center when: new items arrive after sync (`HX-Trigger: newItems` response header), sync errors occur (server returns `HX-Trigger: {"syncError": "message"}`), settings saved. Colored border + glow matching item type. Frosted glass via `backdrop-filter: blur(16px)`. Up to 4 visible at once.

---

## 8. Go Package Structure

```
churndesk/
├── cmd/
│   └── churndesk/
│       └── main.go              # entry point, dependency wiring
├── internal/
│   ├── config/
│   │   └── config.go            # app config from env vars (port, db path)
│   ├── db/
│   │   ├── db.go                # SQLite connection, migration runner
│   │   └── migrations/          # 001_initial.sql, 002_*.sql, ...
│   ├── domain/
│   │   ├── item.go              # Item, ItemType typed constants, Score types
│   │   ├── integration.go       # Integration, Provider, Space types
│   │   └── settings.go          # Settings key typed constants, CategoryWeight types
│   ├── store/
│   │   ├── item_store.go        # Upsert, ListRanked, Dismiss, MarkSeen, RescoreAll
│   │   ├── integration_store.go # CRUD for integrations and spaces
│   │   └── settings_store.go   # get/set settings, category weights
│   ├── sync/
│   │   ├── fetcher.go           # Fetcher interface
│   │   ├── scheduler.go         # starts/stops per-integration workers
│   │   ├── worker.go            # poll loop: calls Fetcher, writes to store
│   │   ├── scorer.go            # re-scorer goroutine (runs every 60s)
│   │   ├── github/
│   │   │   └── fetcher.go       # GitHub API → []domain.Item
│   │   └── jira/
│   │       └── fetcher.go       # Jira API → []domain.Item
│   └── web/
│       ├── server.go            # HTTP server setup, route registration, embed.FS
│       ├── handlers/
│       │   ├── feed.go          # GET /, GET /feed (fragment), GET /feed?count=N
│       │   ├── detail.go        # GET /items/:id/detail (HTMX partial only)
│       │   ├── dismiss.go       # POST /items/:id/dismiss
│       │   ├── seen.go          # POST /items/:id/seen
│       │   ├── sync.go          # POST /sync
│       │   └── settings.go      # GET /settings, POST /settings/*
│       └── templates/
│           ├── layout.templ
│           ├── feed.templ        # full page + /feed fragment partial
│           ├── detail.templ
│           ├── settings.templ
│           └── components.templ  # avatar, pill, icon, feed-item, toast
├── static/
│   ├── app.js                   # Alpine.js state + GSAP wiring + HX-Trigger handler
│   └── style.css                # CSS custom properties, scrollbar, global overrides
├── Dockerfile
├── docker-compose.yml
└── go.mod
```

**Key interfaces:**

```go
// internal/sync/fetcher.go
type Fetcher interface {
    Fetch(ctx context.Context, integration domain.Integration, spaces []domain.Space) ([]domain.Item, error)
}

// internal/store/item_store.go
type ItemStore interface {
    Upsert(ctx context.Context, items []domain.Item) error
    ListRanked(ctx context.Context, limit int) ([]domain.Item, error)
    Count(ctx context.Context) (int, error)
    Dismiss(ctx context.Context, id string) error
    MarkSeen(ctx context.Context, id string) error
    // RescoreAll updates base_score from weights AND recalculates age_boost
    // from created_at, then sets total_score = base_score + age_boost for all items.
    // It does not wait for the re-scorer goroutine.
    RescoreAll(ctx context.Context, weights map[domain.ItemType]int, ageMultiplier float64, maxAgeBoost float64) error
}
```

**Handler routing (clarified):**
- `GET /items/:id/detail` — returns detail HTML partial (no full page; used by HTMX on item click)
- `GET /items/:id` — **does not exist**; deep-linking is out of scope for v1

All dependencies injected via constructors. Fetchers depend only on `domain` types. Setting keys are accessed only via typed constants defined in `domain/settings.go` — no raw strings in handlers or store code.

---

## 9. Docker & Deployment

**Multi-stage Dockerfile:**

Stage 1 (`golang:1.23-bookworm`):
```dockerfile
RUN apt-get update && apt-get install -y gcc libc-dev libsqlite3-dev
ENV CGO_ENABLED=1
RUN go build -o /churndesk ./cmd/churndesk
```

Stage 2 (`debian:bookworm-slim`):
```dockerfile
RUN apt-get update && apt-get install -y libsqlite3-0 ca-certificates
COPY --from=builder /churndesk /churndesk
# Static assets and templates are embedded in the binary — no COPY needed
EXPOSE 8080
ENTRYPOINT ["/churndesk"]
```

**docker-compose.yml:**
```yaml
services:
  churndesk:
    image: churndesk/churndesk:latest
    ports:
      - "8080:8080"
    volumes:
      - ./data:/data
    environment:
      - CHURNDESK_DB_PATH=/data/churndesk.db
      - CHURNDESK_PORT=8080
    restart: unless-stopped
```

**Database migrations:** Run automatically on startup. Forward-only, versioned by filename (`001_initial.sql`, etc.).

**First-run experience:** If no integrations are configured (fresh DB), redirect to `/settings`. Settings form validates that at least one integration (GitHub or Jira) with at least one associated space is saved before the feed is accessible.

**CI/CD:** GitHub Actions builds and pushes to Dockerhub on tagged releases (`v*`).

---

## 10. Out of Scope (v1)

- Multiple accounts per integration
- Linear, Slack, or other integrations
- Mobile layout
- Email/desktop notifications
- PR review submission from within Churndesk
- Jira ticket editing from within Churndesk
- Undo-dismiss (dismissed items can be managed via the settings page in a future version)
