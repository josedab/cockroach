// Copyright 2024 The Cockroach Authors.
//
// Use of this software is governed by the CockroachDB Software License
// included in the /LICENSE file.

package connexec

import (
	"context"

	"github.com/cockroachdb/cockroach/pkg/sql/sem/tree"
)

// StatementExecutor handles statement parsing and execution coordination.
// It delegates the actual execution to the appropriate execution engine
// while managing the execution context and coordinating with other modules.
type StatementExecutor struct {
	// memMgr handles memory accounting during execution.
	memMgr *MemoryManager

	// metricsMgr handles metrics collection during execution.
	metricsMgr *MetricsManager

	// retryMgr handles retry state during execution.
	retryMgr *RetryManager
}

// NewStatementExecutor creates a new StatementExecutor with the given managers.
func NewStatementExecutor(
	memMgr *MemoryManager,
	metricsMgr *MetricsManager,
	retryMgr *RetryManager,
) *StatementExecutor {
	return &StatementExecutor{
		memMgr:     memMgr,
		metricsMgr: metricsMgr,
		retryMgr:   retryMgr,
	}
}

// Statement represents a statement to be executed.
type Statement struct {
	// SQL is the SQL text of the statement.
	SQL string

	// AST is the parsed AST of the statement.
	AST tree.Statement

	// IsInternal indicates whether this is an internal statement.
	IsInternal bool

	// ExpectedTypes contains expected result column types, if any.
	ExpectedTypes []uint32
}

// SQL implements StatementContext.
func (s *Statement) SQL() string {
	return s.SQL
}

// AST implements StatementContext.
func (s *Statement) AST() tree.Statement {
	return s.AST
}

// IsInternal implements StatementContext.
func (s *Statement) IsInternal() bool {
	return s.IsInternal
}

// ExecuteResult contains the result of statement execution.
type ExecuteResult struct {
	// RowsAffected is the number of rows affected by the statement.
	RowsAffected int64

	// Err is any error that occurred during execution.
	Err error
}

// BeforeExecute prepares for statement execution.
// This handles memory accounting and metrics setup.
func (s *StatementExecutor) BeforeExecute(ctx context.Context, stmt *Statement) {
	// Begin memory accounting for this statement
	if s.memMgr != nil {
		s.memMgr.BeginStatement(ctx)
	}

	// Begin metrics collection for this statement
	if s.metricsMgr != nil {
		s.metricsMgr.BeginStatement(ctx, stmt)
	}

	// Record statement for potential retry
	if s.retryMgr != nil && !stmt.IsInternal {
		s.retryMgr.RecordForRetry(stmt)
	}
}

// AfterExecute finalizes statement execution.
// This handles memory accounting cleanup and metrics recording.
func (s *StatementExecutor) AfterExecute(ctx context.Context, stmt *Statement, err error) {
	// End memory accounting for this statement
	if s.memMgr != nil {
		s.memMgr.EndStatement(ctx)
	}

	// End metrics collection for this statement
	if s.metricsMgr != nil {
		s.metricsMgr.EndStatement(ctx, stmt, err)
	}
}

// RecordStatementType records the type of statement for metrics.
func (s *StatementExecutor) RecordStatementType(stmt *Statement) {
	if s.metricsMgr == nil || stmt.AST == nil {
		return
	}

	// Categorize the statement type
	stmtType := categorizeStatement(stmt.AST)
	s.metricsMgr.RecordStatementType(stmtType)
}

// categorizeStatement returns the category of the given statement.
func categorizeStatement(ast tree.Statement) string {
	switch ast.(type) {
	case *tree.Select:
		return "SELECT"
	case *tree.Insert:
		return "INSERT"
	case *tree.Update:
		return "UPDATE"
	case *tree.Delete:
		return "DELETE"
	case *tree.CreateTable, *tree.CreateIndex, *tree.CreateDatabase,
		*tree.AlterTable, *tree.DropTable, *tree.DropIndex, *tree.DropDatabase:
		return "DDL"
	default:
		return "OTHER"
	}
}

// CanRetry returns true if the executor can retry after the given error.
func (s *StatementExecutor) CanRetry(err error) bool {
	if s.retryMgr == nil {
		return false
	}
	return s.retryMgr.CanRetryError(err)
}

// PrepareRetry prepares for a retry attempt.
// Returns false if retry is not possible.
func (s *StatementExecutor) PrepareRetry() bool {
	if s.retryMgr == nil {
		return false
	}
	return s.retryMgr.PrepareForRetry()
}

// GetRewindPos returns the position to rewind to for retry.
func (s *StatementExecutor) GetRewindPos() CmdPos {
	if s.retryMgr == nil {
		return 0
	}
	return s.retryMgr.GetRewindPos()
}

// ClearRetryState clears the retry state after a successful commit.
func (s *StatementExecutor) ClearRetryState() {
	if s.retryMgr != nil {
		s.retryMgr.ClearRetryState()
	}
}

// Reset resets the statement executor state.
func (s *StatementExecutor) Reset() {
	// Nothing to reset in the executor itself;
	// individual managers handle their own reset.
}

// IsTransactionControlStatement returns true if the statement is a
// transaction control statement (BEGIN, COMMIT, ROLLBACK, SAVEPOINT).
func IsTransactionControlStatement(ast tree.Statement) bool {
	switch ast.(type) {
	case *tree.BeginTransaction, *tree.CommitTransaction,
		*tree.RollbackTransaction, *tree.Savepoint,
		*tree.ReleaseSavepoint, *tree.RollbackToSavepoint:
		return true
	default:
		return false
	}
}

// IsDDLStatement returns true if the statement is a DDL statement.
func IsDDLStatement(ast tree.Statement) bool {
	switch ast.(type) {
	case *tree.CreateTable, *tree.CreateIndex, *tree.CreateDatabase,
		*tree.CreateSchema, *tree.CreateSequence, *tree.CreateView,
		*tree.AlterTable, *tree.AlterIndex, *tree.AlterDatabase,
		*tree.AlterSchema, *tree.AlterSequence,
		*tree.DropTable, *tree.DropIndex, *tree.DropDatabase,
		*tree.DropSchema, *tree.DropSequence, *tree.DropView:
		return true
	default:
		return false
	}
}

// IsReadOnlyStatement returns true if the statement is read-only.
func IsReadOnlyStatement(ast tree.Statement) bool {
	switch ast.(type) {
	case *tree.Select, *tree.Explain, *tree.ExplainAnalyze,
		*tree.ShowVar, *tree.ShowCreate, *tree.ShowDatabases,
		*tree.ShowSchemas, *tree.ShowTables, *tree.ShowColumns,
		*tree.ShowIndexes, *tree.ShowConstraints:
		return true
	default:
		return false
	}
}
