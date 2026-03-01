package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/user/llm-proxy-go/internal/models"
	"go.uber.org/zap"
)

// AnalysisReportRepository provides access to routing analysis reports.
type AnalysisReportRepository struct {
	db     *sql.DB
	readDB *sql.DB
	logger *zap.Logger
}

// NewAnalysisReportRepository creates a new AnalysisReportRepository.
func NewAnalysisReportRepository(db *sql.DB, logger *zap.Logger, readDB ...*sql.DB) *AnalysisReportRepository {
	r := &AnalysisReportRepository{db: db, readDB: db, logger: logger}
	if len(readDB) > 0 && readDB[0] != nil {
		r.readDB = readDB[0]
	}
	return r
}

// reportJSON is the JSON structure stored in the report column.
type reportJSON struct {
	Summary         *models.AnalysisSummary          `json:"summary"`
	Issues          []models.AnalysisIssue           `json:"issues"`
	Recommendations []models.AnalysisRecommendation  `json:"recommendations"`
	Conclusion      string                           `json:"conclusion"`
}

// Save persists an analysis report and returns its ID.
func (r *AnalysisReportRepository) Save(ctx context.Context, report *models.AnalysisReport) (int64, error) {
	rj := reportJSON{
		Summary:         report.Summary,
		Issues:          report.Issues,
		Recommendations: report.Recommendations,
		Conclusion:      report.Conclusion,
	}
	reportBytes, err := json.Marshal(rj)
	if err != nil {
		return 0, fmt.Errorf("marshal report: %w", err)
	}

	var startStr, endStr *string
	if report.TimeRangeStart != nil {
		s := report.TimeRangeStart.UTC().Format("2006-01-02 15:04:05")
		startStr = &s
	}
	if report.TimeRangeEnd != nil {
		s := report.TimeRangeEnd.UTC().Format("2006-01-02 15:04:05")
		endStr = &s
	}

	result, err := r.db.ExecContext(ctx,
		`INSERT INTO routing_analysis_reports (model_used, time_range_start, time_range_end, total_logs, analyzed_logs, report)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		report.ModelUsed, startStr, endStr, report.TotalLogs, report.AnalyzedLogs, string(reportBytes))
	if err != nil {
		return 0, fmt.Errorf("insert analysis report: %w", err)
	}
	return result.LastInsertId()
}

// List returns paginated analysis reports (newest first).
func (r *AnalysisReportRepository) List(ctx context.Context, limit, offset int) ([]*models.AnalysisReport, int, error) {
	var total int
	if err := r.readDB.QueryRowContext(ctx, `SELECT COUNT(*) FROM routing_analysis_reports`).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count reports: %w", err)
	}

	rows, err := r.readDB.QueryContext(ctx,
		`SELECT id, model_used, time_range_start, time_range_end, total_logs, analyzed_logs, report, created_at
		 FROM routing_analysis_reports ORDER BY created_at DESC LIMIT ? OFFSET ?`, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("query reports: %w", err)
	}
	defer rows.Close()

	var reports []*models.AnalysisReport
	for rows.Next() {
		rpt, err := r.scanReport(rows)
		if err != nil {
			return nil, 0, err
		}
		reports = append(reports, rpt)
	}
	return reports, total, rows.Err()
}

// GetByID returns a single analysis report.
func (r *AnalysisReportRepository) GetByID(ctx context.Context, id int64) (*models.AnalysisReport, error) {
	rows, err := r.readDB.QueryContext(ctx,
		`SELECT id, model_used, time_range_start, time_range_end, total_logs, analyzed_logs, report, created_at
		 FROM routing_analysis_reports WHERE id = ?`, id)
	if err != nil {
		return nil, fmt.Errorf("query report: %w", err)
	}
	defer rows.Close()

	if !rows.Next() {
		return nil, fmt.Errorf("report not found: %d", id)
	}
	return r.scanReport(rows)
}

func (r *AnalysisReportRepository) scanReport(rows *sql.Rows) (*models.AnalysisReport, error) {
	var rpt models.AnalysisReport
	var startStr, endStr sql.NullString
	var reportStr string
	var createdAt string

	if err := rows.Scan(&rpt.ID, &rpt.ModelUsed, &startStr, &endStr,
		&rpt.TotalLogs, &rpt.AnalyzedLogs, &reportStr, &createdAt); err != nil {
		return nil, fmt.Errorf("scan report: %w", err)
	}

	if startStr.Valid {
		t := parseFlexibleTime(startStr.String)
		rpt.TimeRangeStart = &t
	}
	if endStr.Valid {
		t := parseFlexibleTime(endStr.String)
		rpt.TimeRangeEnd = &t
	}
	rpt.CreatedAt = parseFlexibleTime(createdAt)

	var rj reportJSON
	if err := json.Unmarshal([]byte(reportStr), &rj); err != nil {
		r.logger.Warn("failed to parse report JSON", zap.Error(err))
	} else {
		rpt.Summary = rj.Summary
		rpt.Issues = rj.Issues
		rpt.Recommendations = rj.Recommendations
		rpt.Conclusion = rj.Conclusion
	}

	return &rpt, nil
}
