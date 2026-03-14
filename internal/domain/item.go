// internal/domain/item.go
package domain

import "time"

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

type Item struct {
	ID               string
	Source           string
	Type             ItemType
	ExternalID       string
	Title            string
	URL              string
	Metadata         string
	BaseScore        int
	AgeBoost         float64
	TotalScore       float64
	PROwner          string
	PRRepo           string
	Dismissed        int
	PrerequisitesMet int
	Seen             int
	CreatedAt        time.Time
	UpdatedAt        time.Time
	Deleted          bool
}

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
	State        string
	Body         string
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

type Comment struct {
	ID              int64
	Author          string
	Body            string
	CreatedAt       time.Time
	IsChangeRequest bool
}

type Review struct {
	ID     int64
	Author string
	State  string
}

type CheckRun struct {
	Name       string
	Status     string
	Conclusion string
}

type PRRef struct {
	Owner  string
	Repo   string
	Title  string
	Number int
}

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

type Board struct {
	ID   int
	Name string
	Type string
}
