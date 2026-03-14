# Churndesk Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a locally-run Docker-based developer task aggregator that consolidates GitHub PR and Jira issue actionable items into a single priority-ranked feed.

**Architecture:** Hexagonal Architecture (Ports & Adapters) with DDD. The domain core (`internal/domain/`) holds entities, value objects, and port interfaces — it imports nothing outside stdlib. Outbound adapters (`internal/adapter/`) implement those ports. Application services (`internal/app/`) orchestrate domain logic using injected ports. The HTTP layer (`internal/web/`) is an inbound adapter. All dependencies point inward — adapters depend on the domain, never the reverse.

**Tech Stack:** Go 1.23, Templ, HTMX, Alpine.js, GSAP, SQLite (`mattn/go-sqlite3` via CGO), `go-github/v68`, `go-atlassian`, `goldmark`+`chroma/v2`+`bluemonday` for markdown.

---

## DDD + Hexagonal Architecture

```
┌─────────────────────────────────────────────────────┐
│                    PRIMARY ADAPTERS                  │
│   internal/web/handlers/  (HTTP, HTMX, Templ)       │
└──────────────────────┬──────────────────────────────┘
                       │ calls
┌──────────────────────▼──────────────────────────────┐
│              APPLICATION SERVICES                    │
│   internal/app/  (worker, scorer, scheduler)        │
└──────────────────────┬──────────────────────────────┘
                       │ uses ports (interfaces)
┌──────────────────────▼──────────────────────────────┐
│                 DOMAIN CORE                          │
│   internal/domain/         entities, value objects  │
│   internal/domain/port/    port interfaces          │
│   (zero external imports)                           │
└──────────────────────┬──────────────────────────────┘
                       │ implemented by
┌──────────────────────▼──────────────────────────────┐
│              SECONDARY ADAPTERS (outbound)           │
│   internal/adapter/sqlite/  (repositories)          │
│   internal/adapter/github/  (GitHubClient, Fetcher) │
│   internal/adapter/jira/    (JiraClient, Fetcher)   │
└─────────────────────────────────────────────────────┘
```

**Dependency rule:** Domain has no external imports. Adapters import domain. App imports domain/port. Web imports app and domain/port. Nothing imports adapters except `main.go` (the composition root).

---

## File Structure

```
churndesk/
├── cmd/churndesk/
│   ├── main.go                   ← composition root: wire all dependencies
│   └── static/                   ← embedded via //go:embed static (no ".." in embed paths)
│       ├── style.css
│       └── app.js
├── internal/
│   ├── config/
│   │   └── config.go             ← env-based config (port, db path)
│   ├── db/
│   │   ├── db.go                 ← SQLite connection, WAL, migration runner
│   │   └── migrations/
│   │       └── 001_initial.sql
│   ├── domain/
│   │   ├── item.go               ← Item entity, ItemType value object, PR/Jira types
│   │   ├── integration.go        ← Integration aggregate, Space, Teammate, ReviewPrerequisite
│   │   ├── settings.go           ← SettingKey value object, CategoryWeight
│   │   └── port/
│   │       ├── store.go          ← ItemStore, LinkStore, IntegrationStore, SettingsStore interfaces
│   │       └── sync.go           ← Fetcher, GitHubClient, JiraClient interfaces
│   ├── adapter/                  ← secondary (outbound) adapters
│   │   ├── sqlite/
│   │   │   ├── item_store.go     ← implements port.ItemStore
│   │   │   ├── link_store.go     ← implements port.LinkStore
│   │   │   ├── integration_store.go ← implements port.IntegrationStore
│   │   │   └── settings_store.go    ← implements port.SettingsStore
│   │   ├── github/
│   │   │   ├── client.go         ← implements port.GitHubClient (wraps go-github/v68)
│   │   │   └── fetcher.go        ← implements port.Fetcher for GitHub
│   │   └── jira/
│   │       ├── client.go         ← implements port.JiraClient (wraps go-atlassian)
│   │       └── fetcher.go        ← implements port.Fetcher for Jira
│   ├── app/                      ← application services (orchestration, no HTTP)
│   │   ├── worker.go             ← poll loop: calls Fetcher, writes to store
│   │   ├── scorer.go             ← re-scorer goroutine (runs every 60s)
│   │   └── scheduler.go          ← starts/stops per-integration workers
│   ├── markdown/
│   │   └── render.go             ← goldmark + chroma + bluemonday
│   └── web/                      ← primary (inbound) adapter
│       ├── server.go             ← HTTP server, route registration, embed.FS
│       ├── middleware.go         ← onboarding gate (intentional SRP split)
│       └── handlers/
│           ├── feed.go           ← GET /, GET /feed
│           ├── dismiss.go        ← POST /items/:id/dismiss
│           ├── seen.go           ← POST /items/:id/seen
│           ├── sync.go           ← POST /sync
│           ├── pr.go             ← GET+POST /prs/:owner/:repo/:number/...
│           ├── jira.go           ← GET+POST /jira/:key/...
│           └── settings.go       ← GET /settings, POST /settings/*
│       └── templates/
│           ├── layout.templ
│           ├── feed.templ
│           ├── pr.templ
│           ├── jira.templ
│           ├── settings.templ
│           └── components.templ
├── Dockerfile
├── docker-compose.yml
└── go.mod
```

**Build prerequisites:** CGO is required for SQLite. All `go test` and `go build` commands need `CGO_ENABLED=1`. Templ templates must be compiled before building: `templ generate` (or `go generate ./...`) then `go build`.

---

## Chunks (execute in order)

| File | Contents | Tasks |
|------|----------|-------|
| [chunk-1-foundation.md](chunk-1-foundation.md) | Config, DB, Domain entities, Port interfaces | 1–9 |
| [chunk-2-sqlite.md](chunk-2-sqlite.md) | SQLite adapters (ItemStore, LinkStore, IntegrationStore, SettingsStore) | 10–13 (old numbering: 6–9) |
| [chunk-3-sync.md](chunk-3-sync.md) | GitHub/Jira client+fetcher adapters, App Worker, Scorer, Scheduler | 10–15 |
| [chunk-4-web-foundation.md](chunk-4-web-foundation.md) | Markdown renderer, static assets, HTTP server, middleware, feed handlers+templates | 16–20 |
| [chunk-5-pr-jira-views.md](chunk-5-pr-jira-views.md) | PR handler+templates, Jira handler+templates, shared components | 21–24 |
| [chunk-6-settings-deploy.md](chunk-6-settings-deploy.md) | Settings handler+templates, main.go wiring, Dockerfile, docker-compose, CI | 25–29 |

---
