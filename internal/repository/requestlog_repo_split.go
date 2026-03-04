package repository

import (
	"context"
	"time"

	"github.com/user/llm-proxy-go/internal/models"
)

type RequestLogWriteRepo struct {
	impl *RequestLogRepositoryImpl
}

func NewRequestLogWriteRepository(impl *RequestLogRepositoryImpl) *RequestLogWriteRepo {
	return &RequestLogWriteRepo{impl: impl}
}

func (r *RequestLogWriteRepo) Insert(ctx context.Context, entry *models.RequestLogEntry) (int64, error) {
	return r.impl.Insert(ctx, entry)
}

func (r *RequestLogWriteRepo) MarkInaccurate(ctx context.Context, id int64, inaccurate bool) error {
	return r.impl.MarkInaccurate(ctx, id, inaccurate)
}

type RequestLogQueryRepo struct {
	impl *RequestLogRepositoryImpl
}

func NewRequestLogQueryRepository(impl *RequestLogRepositoryImpl) *RequestLogQueryRepo {
	return &RequestLogQueryRepo{impl: impl}
}

func (r *RequestLogQueryRepo) GetByID(ctx context.Context, id int64) (*models.RequestLog, error) {
	return r.impl.GetByID(ctx, id)
}

func (r *RequestLogQueryRepo) List(ctx context.Context, limit, offset int, userID *int64, modelName, endpointName *string, startTime, endTime *time.Time, success *bool) ([]*models.RequestLog, int64, error) {
	return r.impl.List(ctx, limit, offset, userID, modelName, endpointName, startTime, endTime, success)
}

func (r *RequestLogQueryRepo) GetStatistics(ctx context.Context, startTime, endTime *time.Time, userID *int64, modelName, endpointName *string, success *bool) (*LogStatistics, error) {
	return r.impl.GetStatistics(ctx, startTime, endTime, userID, modelName, endpointName, success)
}

func (r *RequestLogQueryRepo) Count(ctx context.Context, modelName, endpointName *string, startTime, endTime *time.Time) (int64, error) {
	return r.impl.Count(ctx, modelName, endpointName, startTime, endTime)
}

func (r *RequestLogQueryRepo) Delete(ctx context.Context, modelName, endpointName *string, startTime, endTime *time.Time) (int64, error) {
	return r.impl.Delete(ctx, modelName, endpointName, startTime, endTime)
}

type RequestLogAnalyticsRepo struct {
	impl *RequestLogRepositoryImpl
}

func NewRequestLogAnalyticsRepository(impl *RequestLogRepositoryImpl) *RequestLogAnalyticsRepo {
	return &RequestLogAnalyticsRepo{impl: impl}
}

func (r *RequestLogAnalyticsRepo) GetRoutingAggregation(ctx context.Context, startTime, endTime *time.Time) (*RoutingAggregation, error) {
	return r.impl.GetRoutingAggregation(ctx, startTime, endTime)
}

func (r *RequestLogAnalyticsRepo) ListInaccurate(ctx context.Context, limit, offset int) ([]*models.RequestLog, int64, error) {
	return r.impl.ListInaccurate(ctx, limit, offset)
}

func (r *RequestLogAnalyticsRepo) ListForAnalysis(ctx context.Context, startTime, endTime *time.Time, maxResults int) ([]*models.RequestLog, error) {
	return r.impl.ListForAnalysis(ctx, startTime, endTime, maxResults)
}

func (r *RequestLogAnalyticsRepo) CountForAnalysis(ctx context.Context, startTime, endTime *time.Time) (int, error) {
	return r.impl.CountForAnalysis(ctx, startTime, endTime)
}

func (r *RequestLogAnalyticsRepo) GetEndpointModelStats(ctx context.Context) (map[string]*EndpointModelStats, error) {
	return r.impl.GetEndpointModelStats(ctx)
}
