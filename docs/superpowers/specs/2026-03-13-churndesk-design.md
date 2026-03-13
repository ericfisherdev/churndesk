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

Static assets (JS, CSS) are embedded into the binary via `embed.FS`.

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
  → computes age_boost = hours_since_created * age_multiplier
  → writes total_score = base_score + age_boost back to items table

HTMX auto-refresh (every 20s)
  → GET / (or hx-get on feed container)
  → server queries items ordered by total_score DESC
  → renders ranked feed
```

### 3.3 Onboarding Gate

On startup, if no integrations are configured, the app redirects to `/settings`. The settings page enforces that the user must configure **at least one GitHub repo** and **at least one Jira space** (base URL + project key) before the main feed becomes accessible.

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
| `base_score` | INTEGER | From `category_weights` at time of upsert |
| `age_boost` | REAL | Computed by re-scorer |
| `total_score` | REAL | `base_score + age_boost` |
| `dismissed` | INTEGER | 0 or 1 |
| `seen_at` | DATETIME | When user opened detail view |
| `created_at` | DATETIME | When item first appeared |
| `updated_at` | DATETIME | Last sync update |

**Item types:**

| Type | Source | Description |
|------|--------|-------------|
| `pr_review_needed` | GitHub | Open PR from a tracked teammate with no/insufficient reviews |
| `pr_stale_review` | GitHub | PR where existing approval was dismissed (new commits pushed) |
| `pr_changes_requested` | GitHub | User's own PR has a change request |
| `pr_new_comment` | GitHub | New comment on user's own PR |
| `pr_ci_failing` | GitHub | User's own PR has failing CI checks |
| `pr_approved` | GitHub | User's own PR meets minimum review threshold |
| `jira_status_change` | Jira | Assigned ticket status or priority changed |
| `jira_comment` | Jira | New comment on a ticket the user is watching, or reply to user's comment |
| `jira_new_bug` | Jira | New bug ticket (assigned or unassigned) in a tracked space |

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
| `integration_id` | INTEGER FK | → `integrations.id` |
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
| `github_username` | TEXT | |
| `display_name` | TEXT | |

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
| `pr_review_needed` | 60 |
| `pr_new_comment` | 50 |
| `jira_new_bug` | 70 |
| `jira_status_change` | 40 |
| `jira_comment` | 40 |
| `pr_approved` | 30 |

### 4.6 `settings`

Key/value store for UI and behavioral configuration.

| Key | Default | Description |
|-----|---------|-------------|
| `auto_refresh_interval` | `20` | Feed auto-refresh in seconds |
| `age_multiplier` | `0.5` | Points added per hour of age |
| `feed_columns` | `1` | 1, 2, or 3 column layout |
| `min_review_count` | `1` | Min approvals for a PR to be considered approved |

---

## 5. Scoring

```
total_score = base_score + age_boost
age_boost   = floor(hours_since_created) * age_multiplier
```

- `base_score` is set from `category_weights` when an item is first created
- `age_boost` is recalculated by the re-scorer goroutine every 60 seconds
- `total_score` is stored in the database; feed is sorted by `total_score DESC`
- Dismissed items are excluded from the feed query

---

## 6. Integration Sync Logic

### 6.1 GitHub Fetcher

Fetches for each tracked repo:
- Open PRs from tracked teammates → `pr_review_needed` (filtered by: no existing review from authenticated user, PR not authored by authenticated user)
- PRs where authenticated user's previous approval was dismissed → `pr_stale_review`
- Authenticated user's own open PRs:
  - Has change request → `pr_changes_requested`
  - Has new comments since last seen → `pr_new_comment`
  - Has failing CI checks → `pr_ci_failing`
  - Meets minimum review threshold → `pr_approved`

Stale review detection: GitHub dismisses approvals automatically when new commits are pushed (branch protection setting). The fetcher checks review state — if a PR has a `DISMISSED` review from any reviewer, it surfaces as `pr_stale_review`.

### 6.2 Jira Fetcher

On first sync of a space:
1. Detect board type via `GET /rest/agile/1.0/board?projectKeyOrId={key}`
2. Store `board_type` and `jira_board_id` in the `spaces` table

On every sync:
- **Scrum board:** Fetch active sprint → `GET /rest/agile/1.0/board/{id}/sprint?state=active` → fetch all issues in that sprint
- **Kanban board:** `GET /rest/api/3/search?jql=project={key} AND statusCategory != Done`

From fetched issues, surface:
- Issues assigned to user where status or priority changed since last sync → `jira_status_change`
- New comments on issues user is watching or has commented on → `jira_comment`
- New bug-type issues (assigned or unassigned) → `jira_new_bug`

---

## 7. UI Design

### 7.1 Design System

Adapted from the MyGitPanel design — rebranded to Churndesk. Dark minimal aesthetic with a purple accent.

**Color tokens:**
```
bg:          #0f0f10   (page background)
surface:     #16161a   (panels, list hover)
card:        #1c1c22   (cards, dropdowns)
border:      rgba(255,255,255,0.07)
borderSolid: #2a2a33
text:        #e8e8ee
muted:       #7b7b8f
accent:      #7c6af7   (purple — primary actions, PR items)
green:       #34d399   (approved, success)
amber:       #fbbf24   (warnings, changes requested, Jira)
red:         #f87171   (critical, CI failing, bugs)
blue:        #60a5fa   (info, comments)
```

Soft variants at 12–15% opacity for backgrounds. Glow effects via `box-shadow`.

**Typography:** `-apple-system, BlinkMacSystemFont, 'Segoe UI', sans-serif` + `JetBrains Mono` for IDs/code.

### 7.2 Pages

**Main Feed (`/`)**
- Fixed header: Churndesk logo, sync status, manual refresh button (↻), settings link
- Feed body: 1/2/3 column layout (user-configurable), items sorted by `total_score DESC`
- HTMX auto-refresh: `hx-get="/" hx-trigger="every 20s" hx-target="#feed" hx-swap="innerHTML"`
- Each feed item: left status icon, pills (type, source, repo/space), title, meta row (author, time, comment count), latest comment preview, right dismiss button (eye-off, appears on hover)
- Item colors by type: purple (review needed), amber (stale/changes), red (CI failing/bugs), green (approved), blue (comments/status)

**Detail View**
- Triggered by clicking a feed item; slides in from right (GSAP)
- PR tabs: Conversation / Changes / Linked Jira
- Jira tabs: Comments / Ticket Details
- Back to inbox link returns to feed (GSAP slide back)

**Settings (`/settings`)**
- **Integrations section:** GitHub PAT + username, Jira base URL + API token + account ID
- **Tracked repos:** list of `owner/repo` with add/remove, per-repo enabled toggle
- **Jira spaces:** list of `project key` entries with add/remove, board type display (auto-detected)
- **Teammates:** GitHub usernames to watch, add/remove
- **Priority weights:** slider per item type (1–100)
- **Age multiplier:** number input (points per hour)
- **Layout:** 1/2/3 column selector
- **Poll intervals:** per-integration (seconds)
- **Auto-refresh interval:** seconds (default 20)
- **Minimum review count:** number input

### 7.3 GSAP Animations

| Trigger | Effect |
|---------|--------|
| Page load | Header entrance: y -50→0, opacity 0→1, 0.7s power3.out |
| Page load | Feed items: stagger 0.07s, y 30→0, scale 0.97→1, opacity 0→1 |
| New item | Height expand 0→auto + opacity 0→1 (0.4s), glow pulse 3x |
| Toast | y -30→0, scale 0.85→1, back.out(1.5), auto-dismiss after 4.4s |
| Detail open | Feed: opacity 0, x -30 (0.2s); Detail: opacity 0→1, x 30→0 (0.4s) |
| Back to feed | Detail: opacity 0, x +30 (0.2s); Feed: opacity 0→1, x -30→0 (0.35s) |

### 7.4 Toast Notifications

Appear at top-center when: new items arrive after sync, sync errors occur, settings saved. Colored border + glow matching item type. Frosted glass via `backdrop-filter: blur(16px)`. Up to 4 visible at once.

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
│   │   ├── item.go              # Item, ItemType, Score types
│   │   ├── integration.go       # Integration, Provider, Space types
│   │   └── settings.go          # Settings, CategoryWeight types
│   ├── store/
│   │   ├── item_store.go        # Upsert, ListRanked, Dismiss
│   │   ├── integration_store.go # CRUD for integrations and spaces
│   │   └── settings_store.go   # get/set settings, category weights
│   ├── sync/
│   │   ├── scheduler.go         # starts/stops per-integration workers
│   │   ├── worker.go            # poll loop: calls Fetcher, writes to store
│   │   ├── scorer.go            # re-scorer goroutine (runs every 60s)
│   │   ├── github/
│   │   │   └── fetcher.go       # GitHub API → []domain.Item
│   │   └── jira/
│   │       └── fetcher.go       # Jira API → []domain.Item
│   └── web/
│       ├── server.go            # HTTP server setup, route registration
│       ├── handlers/
│       │   ├── feed.go          # GET /
│       │   ├── detail.go        # GET /items/:id
│       │   ├── dismiss.go       # POST /items/:id/dismiss
│       │   ├── sync.go          # POST /sync
│       │   └── settings.go      # GET /settings, POST /settings/*
│       └── templates/
│           ├── layout.templ
│           ├── feed.templ
│           ├── detail.templ
│           ├── settings.templ
│           └── components.templ  # avatar, pill, icon, feed-item, toast
├── static/
│   ├── app.js                   # Alpine.js state + GSAP wiring
│   └── style.css                # scrollbar, global overrides, color vars
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
    Dismiss(ctx context.Context, id string) error
    Undismiss(ctx context.Context, id string) error
}
```

All dependencies injected via constructors. Fetchers depend only on `domain` types.

---

## 9. Docker & Deployment

**Multi-stage Dockerfile:**
- Stage 1 (`golang:1.23`): compile binary with CGO enabled for SQLite
- Stage 2 (`debian:bookworm-slim`): copy binary + static assets, expose port 8080

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

**Database migrations:** Run automatically on startup. Forward-only, versioned by filename.

**First-run experience:** If no integrations are configured (fresh DB), redirect to `/settings`. Settings form validates that at least one GitHub repo and one Jira space are saved before allowing navigation to the main feed.

**CI/CD:** GitHub Actions builds and pushes to Dockerhub on tagged releases (`v*`).

---

## 10. Out of Scope (v1)

- Multiple accounts per integration
- Linear, Slack, or other integrations
- Mobile layout
- Email/desktop notifications
- PR review submission from within Churndesk
- Jira ticket editing from within Churndesk
