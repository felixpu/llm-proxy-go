//go:build !integration && !e2e
// +build !integration,!e2e

package service

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/user/llm-proxy-go/internal/models"
	"go.uber.org/zap"
)

func TestEndpointStore_GetEndpoints_ReturnsAppendSafeSnapshot(t *testing.T) {
	store := &EndpointStore{
		endpoints: []*models.Endpoint{
			{Model: &models.Model{Name: "m1"}},
		},
		logger: zap.NewNop(),
	}

	snapshot := store.GetEndpoints()
	if assert.Len(t, snapshot, 1) {
		snapshot = append(snapshot, &models.Endpoint{Model: &models.Model{Name: "m2"}})
	}

	current := store.GetEndpoints()
	assert.Len(t, current, 1, "mutating returned slice should not affect store internal slice")
	assert.Equal(t, "m1", current[0].Model.Name)
}
