package main

import (
	"context"
	"embed"
	"io/fs"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	gogithub "github.com/google/go-github/v68/github"
	"golang.org/x/oauth2"

	"github.com/churndesk/churndesk/internal/adapter/github"
	jiradapter "github.com/churndesk/churndesk/internal/adapter/jira"
	"github.com/churndesk/churndesk/internal/adapter/sqlite"
	"github.com/churndesk/churndesk/internal/app"
	"github.com/churndesk/churndesk/internal/config"
	"github.com/churndesk/churndesk/internal/db"
	"github.com/churndesk/churndesk/internal/domain"
	"github.com/churndesk/churndesk/internal/domain/port"
	"github.com/churndesk/churndesk/internal/web"
	"github.com/churndesk/churndesk/internal/web/handlers"
)

//go:embed static
var staticFiles embed.FS

func main() {
	cfg := config.Load()

	conn, err := db.Open(cfg.DBPath)
	if err != nil {
		log.Fatalf("open db: %v", err)
	}

	itemStore := sqlite.NewItemStore(conn)
	linkStore := sqlite.NewLinkStore(conn)
	integrationStore := sqlite.NewIntegrationStore(conn)
	settingsStore := sqlite.NewSettingsStore(conn)

	ghToken, ghUsername := loadGitHubCredentials(integrationStore)
	ts := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: ghToken})
	tc := oauth2.NewClient(context.Background(), ts)
	tc.Timeout = 30 * time.Second
	ghGoClient := gogithub.NewClient(tc)
	ghClient := github.NewClient(ghGoClient, ghUsername)

	jiraBaseURL, jiraEmail, jiraToken, jiraAccountID := loadJiraCredentials(integrationStore)
	jiraClient, err := jiradapter.NewClient(jiraBaseURL, jiraEmail, jiraToken, jiraAccountID)
	if err != nil {
		log.Printf("jira client init (will be unavailable until configured): %v", err)
		jiraClient = nil
	}

	minReviewCount := loadMinReviewCount(settingsStore)
	teammates := loadTeammates(integrationStore)
	ghFetcher := github.NewFetcher(ghClient, ghUsername, teammates, minReviewCount)

	fetchers := map[domain.Provider]port.Fetcher{
		domain.ProviderGitHub: ghFetcher,
	}
	if jiraClient != nil {
		jiraFetcher := jiradapter.NewFetcher(jiraClient, jiraAccountID)
		fetchers[domain.ProviderJira] = jiraFetcher
	}

	scheduler := app.NewScheduler(itemStore, integrationStore, settingsStore, fetchers)
	scorer := app.NewScorer(itemStore, settingsStore, integrationStore)

	feedHandler := handlers.NewFeedHandler(itemStore, scheduler, settingsStore)
	prHandler := handlers.NewPRHandler(ghClient, itemStore, linkStore, integrationStore, ghUsername)

	var jiraHandler *handlers.JiraHandler
	if jiraClient != nil {
		jiraHandler = handlers.NewJiraHandler(jiraClient, itemStore, linkStore)
	} else {
		jiraHandler = handlers.NewJiraHandler(newNoopJiraClient(), itemStore, linkStore)
	}

	settingsHandler := handlers.NewSettingsHandler(integrationStore, settingsStore, itemStore, scheduler)

	gate := web.OnboardingGate(integrationStore)

	staticFS, err := fs.Sub(staticFiles, "static")
	if err != nil {
		_ = conn.Close()
		log.Fatalf("static fs: %v", err)
	}
	defer func() { _ = conn.Close() }()

	srv := web.NewServer(staticFS, feedHandler, prHandler, jiraHandler, settingsHandler, gate)
	httpServer := &http.Server{
		Addr:    ":" + cfg.Port,
		Handler: srv.Handler(),
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	scorer.Start(ctx)
	go func() {
		if err := scheduler.Start(ctx); err != nil {
			log.Printf("scheduler stopped: %v", err)
		}
	}()

	go func() {
		log.Printf("churndesk listening on :%s", cfg.Port)
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("http server: %v", err)
		}
	}()

	<-ctx.Done()
	log.Println("shutting down")
	httpServer.Shutdown(context.Background()) //nolint:errcheck
}

func loadGitHubCredentials(store port.IntegrationStore) (token, username string) {
	integrations, err := store.ListIntegrations(context.Background())
	if err != nil {
		return "", ""
	}
	for _, ig := range integrations {
		if ig.Provider == domain.ProviderGitHub && ig.Enabled {
			return ig.AccessToken, ig.Username
		}
	}
	return "", ""
}

func loadJiraCredentials(store port.IntegrationStore) (baseURL, email, token, accountID string) {
	integrations, err := store.ListIntegrations(context.Background())
	if err != nil {
		return
	}
	for _, ig := range integrations {
		if ig.Provider == domain.ProviderJira && ig.Enabled {
			return ig.BaseURL, ig.Username, ig.AccessToken, ig.AccountID
		}
	}
	return
}

func loadMinReviewCount(store port.SettingsStore) int {
	val, _ := store.Get(context.Background(), domain.SettingMinReviewCount)
	if val != "" {
		if n, err := strconv.Atoi(val); err == nil && n > 0 {
			return n
		}
	}
	return 1
}

func loadTeammates(store port.IntegrationStore) []domain.Teammate {
	integrations, _ := store.ListIntegrations(context.Background())
	out := make([]domain.Teammate, 0, len(integrations))
	for _, ig := range integrations {
		ts, _ := store.ListTeammates(context.Background(), ig.ID)
		out = append(out, ts...)
	}
	return out
}

// noopJiraClient satisfies handlers.JiraAPIClient when Jira is not configured.
type noopJiraClient struct{}

func newNoopJiraClient() *noopJiraClient { return &noopJiraClient{} }

func (n *noopJiraClient) GetIssue(_ context.Context, _ string) (*domain.JiraIssue, error) {
	return nil, nil
}
func (n *noopJiraClient) ListIssueComments(_ context.Context, _ string) ([]domain.Comment, error) {
	return nil, nil
}
func (n *noopJiraClient) PostComment(_ context.Context, _, _ string) error { return nil }
