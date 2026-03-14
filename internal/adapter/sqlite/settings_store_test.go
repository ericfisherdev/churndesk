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
	assert.Equal(t, "20", val)

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
