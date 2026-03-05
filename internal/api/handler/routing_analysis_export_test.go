package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/user/llm-proxy-go/internal/models"
	"github.com/user/llm-proxy-go/internal/repository"
	"github.com/user/llm-proxy-go/internal/service"
	"github.com/user/llm-proxy-go/internal/testutil"
)

type exportTestLogStore struct {
	allLogs        []*models.RequestLog
	inaccurateLogs []*models.RequestLog
}

func (s *exportTestLogStore) GetByID(ctx context.Context, id int64) (*models.RequestLog, error) {
	return nil, nil
}

func (s *exportTestLogStore) List(ctx context.Context, limit, offset int, userID *int64, modelName, endpointName *string, startTime, endTime *time.Time, success *bool) ([]*models.RequestLog, int64, error) {
	return s.allLogs, int64(len(s.allLogs)), nil
}

func (s *exportTestLogStore) MarkInaccurate(ctx context.Context, id int64, inaccurate bool) error {
	return nil
}

func (s *exportTestLogStore) GetRoutingAggregation(ctx context.Context, startTime, endTime *time.Time) (*repository.RoutingAggregation, error) {
	return &repository.RoutingAggregation{}, nil
}

func (s *exportTestLogStore) ListInaccurate(ctx context.Context, limit, offset int) ([]*models.RequestLog, int64, error) {
	return s.inaccurateLogs, int64(len(s.inaccurateLogs)), nil
}

func TestRoutingAnalysisHandler_ExportRoutingData_Meta(t *testing.T) {
	store := &exportTestLogStore{
		allLogs: []*models.RequestLog{
			{ID: 1, MessagePreview: "all-log", TaskType: "default", IsInaccurate: false},
		},
		inaccurateLogs: []*models.RequestLog{
			{ID: 2, MessagePreview: "bad-log", TaskType: "complex", IsInaccurate: true},
		},
	}
	handler := NewRoutingAnalysisHandler(store, nil, testutil.NewTestLogger())

	t.Run("inaccurate_only true", func(t *testing.T) {
		c, w := testutil.NewTestContext()
		c.Request = httptest.NewRequest("GET", "/api/routing/analysis/export?inaccurate_only=true&limit=10", nil)
		c.Set("current_user", &service.CurrentUser{UserID: 1, Username: "admin", Role: "admin"})

		handler.ExportRoutingData(c)
		assert.Equal(t, http.StatusOK, w.Code)

		var resp map[string]any
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.Equal(t, float64(1), resp["count"])

		meta, ok := resp["meta"].(map[string]any)
		require.True(t, ok)
		assert.Equal(t, true, meta["inaccurate_only"])
		assert.Contains(t, meta["scope_note"], "仅包含已标记不准确日志")
	})

	t.Run("inaccurate_only false", func(t *testing.T) {
		c, w := testutil.NewTestContext()
		c.Request = httptest.NewRequest("GET", "/api/routing/analysis/export?inaccurate_only=false&limit=10", nil)
		c.Set("current_user", &service.CurrentUser{UserID: 1, Username: "admin", Role: "admin"})

		handler.ExportRoutingData(c)
		assert.Equal(t, http.StatusOK, w.Code)

		var resp map[string]any
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.Equal(t, float64(1), resp["count"])

		meta, ok := resp["meta"].(map[string]any)
		require.True(t, ok)
		assert.Equal(t, false, meta["inaccurate_only"])
		assert.Contains(t, meta["scope_note"], "不代表全量分布")
	})
}
