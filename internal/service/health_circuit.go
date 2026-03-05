package service

import (
	"time"

	"github.com/user/llm-proxy-go/internal/config"
	"github.com/user/llm-proxy-go/internal/models"
	"go.uber.org/zap"
)

func applyRequestResultToCircuitState(
	state *EndpointState,
	result RequestResult,
	cfg config.CircuitBreakerConfig,
	logger *zap.Logger,
) {
	if !cfg.Enabled {
		state.CircuitState = CircuitClosed
		state.CircuitOpenedAt = nil
		state.HalfOpenSuccesses = 0
		state.HalfOpenFailures = 0
		return
	}

	switch state.CircuitState {
	case CircuitClosed:
		handleClosedState(state, result, cfg, logger)

	case CircuitHalfOpen:
		handleHalfOpenState(state, result, cfg, logger)

	case CircuitOpen:
		handleOpenState(result, logger)
	}
}

func handleClosedState(
	state *EndpointState,
	result RequestResult,
	cfg config.CircuitBreakerConfig,
	logger *zap.Logger,
) {
	if result.Success {
		state.ConsecutiveFailures = 0
		state.ConsecutivePermanent = 0
		return
	}

	state.ConsecutiveFailures++
	now := time.Now()
	state.LastFailureTime = &now

	var errorType ErrorType
	if result.Err != nil {
		errorType = classifyError(result.Err)
	} else {
		errorType = classifyHTTPError(result.StatusCode, result.ResponseBody)
	}
	if errorType == ErrorTypePermanent {
		state.ConsecutivePermanent++
	}

	errorRecord := ErrorRecord{
		Timestamp:  now,
		ErrorType:  errorType,
		StatusCode: result.StatusCode,
		Message:    extractErrorMessage(result.Err, result.ResponseBody),
	}
	state.RecentErrors = append(state.RecentErrors, errorRecord)
	if len(state.RecentErrors) > 20 {
		trimmed := make([]ErrorRecord, 20)
		copy(trimmed, state.RecentErrors[len(state.RecentErrors)-20:])
		state.RecentErrors = trimmed
	}

	if shouldOpen, reason := shouldTransitionToOpen(state, cfg); shouldOpen {
		state.CircuitState = CircuitOpen
		openedAt := time.Now()
		state.CircuitOpenedAt = &openedAt
		state.LastError = reason
		state.Status = models.EndpointUnhealthy
		logger.Warn("Circuit breaker opened",
			zap.String("endpoint", result.EndpointName),
			zap.String("reason", reason),
			zap.Int("consecutive_failures", state.ConsecutiveFailures),
			zap.Int("consecutive_permanent", state.ConsecutivePermanent),
		)
	}
}

func handleHalfOpenState(
	state *EndpointState,
	result RequestResult,
	cfg config.CircuitBreakerConfig,
	logger *zap.Logger,
) {
	if result.Success {
		state.HalfOpenSuccesses++
		state.ConsecutiveFailures = 0
		state.ConsecutivePermanent = 0

		if state.HalfOpenSuccesses >= cfg.HalfOpenMaxRequests {
			state.CircuitState = CircuitClosed
			state.HalfOpenSuccesses = 0
			state.HalfOpenFailures = 0
			state.Status = models.EndpointHealthy
			logger.Info("Circuit breaker closed after successful recovery",
				zap.String("endpoint", result.EndpointName),
			)
		}
		return
	}

	state.HalfOpenFailures++
	state.CircuitState = CircuitOpen
	now := time.Now()
	state.CircuitOpenedAt = &now
	state.HalfOpenSuccesses = 0
	state.HalfOpenFailures = 0
	state.Status = models.EndpointUnhealthy
	logger.Warn("Circuit breaker reopened after failed test request",
		zap.String("endpoint", result.EndpointName),
	)
}

func handleOpenState(result RequestResult, logger *zap.Logger) {
	logger.Debug("Request recorded in open circuit state",
		zap.String("endpoint", result.EndpointName),
		zap.Bool("success", result.Success),
	)
}
