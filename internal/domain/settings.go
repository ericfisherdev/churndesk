// internal/domain/settings.go
package domain

type SettingKey string

const (
	SettingAutoRefreshInterval SettingKey = "auto_refresh_interval"
	SettingAgeMultiplier       SettingKey = "age_multiplier"
	SettingMaxAgeBoost         SettingKey = "max_age_boost"
	SettingFeedColumns         SettingKey = "feed_columns"
	SettingMinReviewCount      SettingKey = "min_review_count"
)

type CategoryWeight struct {
	ItemType ItemType
	Weight   int
}
