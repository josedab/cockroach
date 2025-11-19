// Copyright 2024 The Cockroach Authors.
//
// Use of this software is governed by the CockroachDB Software License
// included in the /LICENSE file.

package connexec

import (
	"context"
)

// Coordinator orchestrates the various modules that make up the connection
// executor. This provides the interface that the main connExecutor can use
// to delegate to the modular components.
//
// This follows the RFC design where connExecutor becomes a coordinator
// rather than containing all the implementation directly.
type Coordinator struct {
	// stmtExec handles statement execution coordination.
	stmtExec *StatementExecutor

	// txnMgr manages transaction lifecycle.
	txnMgr *TransactionManager

	// sessionMgr manages session state.
	sessionMgr *SessionManager

	// memMgr handles memory accounting.
	memMgr *MemoryManager

	// metricsMgr handles metrics collection.
	metricsMgr *MetricsManager

	// cursorMgr manages SQL cursors.
	cursorMgr *CursorManager

	// retryMgr handles automatic retry.
	retryMgr *RetryManager
}

// CoordinatorConfig contains configuration for creating a Coordinator.
type CoordinatorConfig struct {
	// MaxRetries is the maximum number of automatic retries.
	MaxRetries int
}

// NewCoordinator creates a new Coordinator with all modules initialized.
func NewCoordinator(cfg CoordinatorConfig) *Coordinator {
	// Create the managers
	memMgr := NewMemoryManager(nil, nil, nil, nil)
	retryMgr := NewRetryManager(cfg.MaxRetries)
	metricsMgr := NewMetricsManager()
	cursorMgr := NewCursorManager()
	txnMgr := NewTransactionManager()
	sessionMgr := NewSessionManager(nil)

	// Create the statement executor with references to the managers
	stmtExec := NewStatementExecutor(memMgr, metricsMgr, retryMgr)

	return &Coordinator{
		stmtExec:   stmtExec,
		txnMgr:     txnMgr,
		sessionMgr: sessionMgr,
		memMgr:     memMgr,
		metricsMgr: metricsMgr,
		cursorMgr:  cursorMgr,
		retryMgr:   retryMgr,
	}
}

// ExecStmt executes a statement using the modular components.
// This demonstrates the simplified execution flow described in the RFC.
func (c *Coordinator) ExecStmt(ctx context.Context, stmt *Statement) error {
	// Memory accounting
	c.memMgr.BeginStatement(ctx)
	defer c.memMgr.EndStatement(ctx)

	// Execute the statement with coordinated metrics and retry
	c.stmtExec.BeforeExecute(ctx, stmt)

	// Actual execution would be delegated here
	// For now, we just demonstrate the coordination pattern
	var err error

	c.stmtExec.AfterExecute(ctx, stmt, err)

	if err != nil {
		if c.retryMgr.CanRetryError(err) {
			return c.retryTransaction(ctx)
		}
		return err
	}

	c.metricsMgr.RecordSuccess(stmt)
	return nil
}

// retryTransaction handles automatic transaction retry.
func (c *Coordinator) retryTransaction(ctx context.Context) error {
	if !c.retryMgr.PrepareForRetry() {
		return nil // Cannot retry
	}

	c.metricsMgr.RecordRetry()

	// Rewind to the saved position and replay statements
	// This would be implemented in the main connExecutor

	return nil
}

// TransactionManager returns the transaction manager.
func (c *Coordinator) TransactionManager() *TransactionManager {
	return c.txnMgr
}

// SessionManager returns the session manager.
func (c *Coordinator) SessionManager() *SessionManager {
	return c.sessionMgr
}

// MemoryManager returns the memory manager.
func (c *Coordinator) MemoryManager() *MemoryManager {
	return c.memMgr
}

// MetricsManager returns the metrics manager.
func (c *Coordinator) MetricsManager() *MetricsManager {
	return c.metricsMgr
}

// CursorManager returns the cursor manager.
func (c *Coordinator) CursorManager() *CursorManager {
	return c.cursorMgr
}

// RetryManager returns the retry manager.
func (c *Coordinator) RetryManager() *RetryManager {
	return c.retryMgr
}

// StatementExecutor returns the statement executor.
func (c *Coordinator) StatementExecutor() *StatementExecutor {
	return c.stmtExec
}

// Reset resets all modules to their initial state.
func (c *Coordinator) Reset(ctx context.Context) {
	c.txnMgr.Reset()
	c.sessionMgr.Reset()
	c.memMgr.Reset(ctx)
	c.metricsMgr.Reset()
	c.cursorMgr.Reset()
	c.retryMgr.Reset()
}

// OnTransactionCommit should be called when a transaction commits successfully.
func (c *Coordinator) OnTransactionCommit(ctx context.Context) {
	c.txnMgr.FinishCommit(ctx)
	c.cursorMgr.CloseTransactionCursors()
	c.retryMgr.ClearRetryState()
	c.metricsMgr.EndTransaction(ctx, c.txnMgr, true)
}

// OnTransactionRollback should be called when a transaction rolls back.
func (c *Coordinator) OnTransactionRollback(ctx context.Context) {
	c.cursorMgr.CloseTransactionCursors()
	c.retryMgr.ClearRetryState()
	c.metricsMgr.EndTransaction(ctx, c.txnMgr, false)
}
