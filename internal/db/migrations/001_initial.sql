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
