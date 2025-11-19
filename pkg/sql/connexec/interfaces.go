// Copyright 2024 The Cockroach Authors.
//
// Use of this software is governed by the CockroachDB Software License
// included in the /LICENSE file.

// Package connexec provides modular components for connection executor
// functionality. This package breaks down the monolithic connExecutor
// into smaller, focused modules that can be understood, tested, and
// maintained independently.
package connexec

import (
	"context"

	"github.com/cockroachdb/cockroach/pkg/kv"
	"github.com/cockroachdb/cockroach/pkg/security/username"
	"github.com/cockroachdb/cockroach/pkg/sql/sem/tree"
	"github.com/cockroachdb/cockroach/pkg/sql/sessiondata"
	"github.com/cockroachdb/cockroach/pkg/util/mon"
)

// TransactionContext provides transaction information to other modules.
// This interface allows modules to access transaction state without
// direct coupling to the implementation.
type TransactionContext interface {
	// Txn returns the underlying KV transaction.
	Txn() *kv.Txn

	// IsolationLevel returns the transaction's isolation level.
	IsolationLevel() tree.IsolationLevel

	// ReadOnly returns true if the transaction is read-only.
	ReadOnly() bool

	// IsImplicit returns true if this is an implicit transaction.
	IsImplicit() bool

	// Priority returns the transaction priority.
	Priority() tree.UserPriority
}

// SessionContext provides session information to other modules.
// This interface allows modules to access session state without
// direct coupling to the implementation.
type SessionContext interface {
	// User returns the current SQL username.
	User() username.SQLUsername

	// Database returns the current database name.
	Database() string

	// SearchPath returns the current search path.
	SearchPath() sessiondata.SearchPath

	// ApplicationName returns the application name for this session.
	ApplicationName() string

	// SessionData returns the full session data.
	SessionData() *sessiondata.SessionData
}

// MemoryContext provides memory accounting information.
// This interface allows modules to track and manage memory usage.
type MemoryContext interface {
	// SessionMonitor returns the session-level memory monitor.
	SessionMonitor() *mon.BytesMonitor

	// TxnMonitor returns the transaction-level memory monitor.
	TxnMonitor() *mon.BytesMonitor

	// StmtMonitor returns the statement-level memory monitor.
	StmtMonitor() *mon.BytesMonitor
}

// StatementContext provides information about the current statement
// being executed.
type StatementContext interface {
	// SQL returns the SQL text of the statement.
	SQL() string

	// AST returns the parsed AST of the statement.
	AST() tree.Statement

	// IsInternal returns true if this is an internal statement.
	IsInternal() bool
}

// ExecutionObserver allows modules to observe statement execution events.
// This is used for metrics collection and logging.
type ExecutionObserver interface {
	// OnStatementStart is called when a statement begins execution.
	OnStatementStart(ctx context.Context, stmt StatementContext)

	// OnStatementEnd is called when a statement completes execution.
	OnStatementEnd(ctx context.Context, stmt StatementContext, err error)

	// OnTransactionStart is called when a transaction begins.
	OnTransactionStart(ctx context.Context, txn TransactionContext)

	// OnTransactionEnd is called when a transaction completes.
	OnTransactionEnd(ctx context.Context, txn TransactionContext, committed bool)
}

// RetryContext provides information about automatic retry state.
type RetryContext interface {
	// CanAutoRetry returns true if automatic retry is currently possible.
	CanAutoRetry() bool

	// RecordForRetry records a statement for potential retry.
	RecordForRetry(stmt StatementContext)

	// ClearRetryState clears the retry state after a successful commit.
	ClearRetryState()
}
