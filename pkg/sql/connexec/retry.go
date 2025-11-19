// Copyright 2024 The Cockroach Authors.
//
// Use of this software is governed by the CockroachDB Software License
// included in the /LICENSE file.

package connexec

import (
	"github.com/cockroachdb/cockroach/pkg/kv/kvpb"
	"github.com/cockroachdb/cockroach/pkg/sql/pgwire/pgcode"
	"github.com/cockroachdb/cockroach/pkg/sql/pgwire/pgerror"
	"github.com/cockroachdb/errors"
)

// RetryManager handles automatic transaction retry for a connection executor.
// CockroachDB can automatically retry transactions that fail due to serialization
// errors when certain conditions are met (results haven't been sent to client).
type RetryManager struct {
	// rewindPos is the position in the statement buffer to which we'll
	// rewind when performing automatic retries.
	rewindPos CmdPos

	// canRetry indicates whether automatic retry is currently possible.
	// This is typically true when results haven't been sent to the client.
	canRetry bool

	// retryCount tracks the number of retries performed for the current
	// transaction.
	retryCount int

	// maxRetries is the maximum number of automatic retries to attempt.
	maxRetries int

	// statements records statements for potential replay during retry.
	statements []StatementRecord
}

// CmdPos represents a position in the command buffer.
type CmdPos int

// StatementRecord holds information about a statement for retry purposes.
type StatementRecord struct {
	// SQL is the SQL text of the statement.
	SQL string

	// IsRetryable indicates whether this statement can be retried.
	IsRetryable bool
}

// NewRetryManager creates a new RetryManager with the specified maximum retries.
func NewRetryManager(maxRetries int) *RetryManager {
	return &RetryManager{
		rewindPos:  0,
		canRetry:   true,
		retryCount: 0,
		maxRetries: maxRetries,
		statements: make([]StatementRecord, 0),
	}
}

// CanAutoRetry returns true if automatic retry is currently possible.
func (r *RetryManager) CanAutoRetry() bool {
	return r.canRetry && r.retryCount < r.maxRetries
}

// SetCanRetry sets whether automatic retry is possible.
// This should be set to false when results are sent to the client.
func (r *RetryManager) SetCanRetry(canRetry bool) {
	r.canRetry = canRetry
}

// RecordStatement records a statement for potential retry.
func (r *RetryManager) RecordStatement(sql string, isRetryable bool) {
	r.statements = append(r.statements, StatementRecord{
		SQL:         sql,
		IsRetryable: isRetryable,
	})
}

// RecordForRetry implements the RetryContext interface.
func (r *RetryManager) RecordForRetry(stmt StatementContext) {
	r.RecordStatement(stmt.SQL(), true)
}

// GetRetryStatements returns the statements to replay during a retry.
func (r *RetryManager) GetRetryStatements() []StatementRecord {
	return r.statements
}

// ClearRetryState clears the retry state after a successful commit.
func (r *RetryManager) ClearRetryState() {
	r.rewindPos = 0
	r.canRetry = true
	r.retryCount = 0
	r.statements = make([]StatementRecord, 0)
}

// SetRewindPos sets the position to which we'll rewind during retry.
func (r *RetryManager) SetRewindPos(pos CmdPos) {
	r.rewindPos = pos
}

// GetRewindPos returns the current rewind position.
func (r *RetryManager) GetRewindPos() CmdPos {
	return r.rewindPos
}

// IncrementRetryCount increments the retry counter.
func (r *RetryManager) IncrementRetryCount() {
	r.retryCount++
}

// GetRetryCount returns the current retry count.
func (r *RetryManager) GetRetryCount() int {
	return r.retryCount
}

// IsRetryableError returns true if the error is retryable.
// Retryable errors include serialization failures and write intent errors.
func (r *RetryManager) IsRetryableError(err error) bool {
	if err == nil {
		return false
	}

	// Check for serialization failure
	if pgErr := pgerror.GetPGCause(err); pgErr != nil {
		if pgErr.Code == pgcode.SerializationFailure {
			return true
		}
	}

	// Check for transaction retry errors
	var retryErr *kvpb.TransactionRetryWithProtoRefreshError
	if errors.As(err, &retryErr) {
		return true
	}

	return false
}

// CanRetryError returns true if the specific error can be retried
// given the current retry state.
func (r *RetryManager) CanRetryError(err error) bool {
	if !r.CanAutoRetry() {
		return false
	}

	return r.IsRetryableError(err)
}

// PrepareForRetry prepares the manager for a retry attempt.
// Returns false if no more retries are allowed.
func (r *RetryManager) PrepareForRetry() bool {
	if !r.CanAutoRetry() {
		return false
	}

	r.IncrementRetryCount()
	return true
}

// ResetForNewTransaction resets the retry state for a new transaction.
func (r *RetryManager) ResetForNewTransaction(pos CmdPos) {
	r.rewindPos = pos
	r.canRetry = true
	r.statements = make([]StatementRecord, 0)
	// Note: retryCount is preserved across transactions within a session
}

// Reset fully resets the retry manager state.
func (r *RetryManager) Reset() {
	r.rewindPos = 0
	r.canRetry = true
	r.retryCount = 0
	r.statements = make([]StatementRecord, 0)
}

// StatementCount returns the number of recorded statements.
func (r *RetryManager) StatementCount() int {
	return len(r.statements)
}

// DisableRetry disables automatic retry for the current transaction.
// This is called when results have been sent to the client.
func (r *RetryManager) DisableRetry() {
	r.canRetry = false
}
