// Copyright 2024 The Cockroach Authors.
//
// Use of this software is governed by the CockroachDB Software License
// included in the /LICENSE file.

package connexec

import (
	"context"

	"github.com/cockroachdb/cockroach/pkg/util/mon"
)

// MemoryManager handles memory accounting for a connection executor.
// It manages three levels of memory monitors:
// - Session: tracks session-bound objects like prepared statements
// - Transaction: tracks transaction-scoped allocations
// - Statement: tracks individual statement allocations
type MemoryManager struct {
	// sessionMon tracks session-level memory usage.
	sessionMon *mon.BytesMonitor

	// txnMon tracks transaction-level memory usage.
	txnMon *mon.BytesMonitor

	// stmtMon tracks statement-level memory usage.
	stmtMon *mon.BytesMonitor

	// memMetrics contains memory metrics for this connection.
	memMetrics *MemoryMetrics

	// currentStmtAccount tracks the current statement's memory account.
	currentStmtAccount mon.BoundAccount
}

// MemoryMetrics contains metrics related to memory usage.
type MemoryMetrics struct {
	// CurrentBytesCount is the current bytes allocated.
	CurrentBytesCount int64

	// MaxBytesHist is the maximum bytes ever allocated.
	MaxBytesHist int64
}

// NewMemoryManager creates a new MemoryManager with the given monitors.
func NewMemoryManager(
	sessionMon *mon.BytesMonitor,
	txnMon *mon.BytesMonitor,
	stmtMon *mon.BytesMonitor,
	memMetrics *MemoryMetrics,
) *MemoryManager {
	return &MemoryManager{
		sessionMon: sessionMon,
		txnMon:     txnMon,
		stmtMon:    stmtMon,
		memMetrics: memMetrics,
	}
}

// SessionMonitor returns the session-level memory monitor.
func (m *MemoryManager) SessionMonitor() *mon.BytesMonitor {
	return m.sessionMon
}

// TxnMonitor returns the transaction-level memory monitor.
func (m *MemoryManager) TxnMonitor() *mon.BytesMonitor {
	return m.txnMon
}

// StmtMonitor returns the statement-level memory monitor.
func (m *MemoryManager) StmtMonitor() *mon.BytesMonitor {
	return m.stmtMon
}

// BeginStatement prepares memory accounting for a new statement.
// This should be called at the start of statement execution.
func (m *MemoryManager) BeginStatement(ctx context.Context) {
	if m.stmtMon != nil {
		m.currentStmtAccount = m.stmtMon.MakeBoundAccount()
	}
}

// EndStatement finalizes memory accounting for the current statement.
// This should be called at the end of statement execution.
func (m *MemoryManager) EndStatement(ctx context.Context) {
	m.currentStmtAccount.Close(ctx)
}

// Allocate allocates the specified number of bytes from the current
// statement's memory account. Returns an error if the allocation
// would exceed memory limits.
func (m *MemoryManager) Allocate(ctx context.Context, bytes int64) error {
	return m.currentStmtAccount.Grow(ctx, bytes)
}

// Release releases the specified number of bytes from the current
// statement's memory account.
func (m *MemoryManager) Release(ctx context.Context, bytes int64) {
	m.currentStmtAccount.Shrink(ctx, bytes)
}

// BeginTransaction prepares memory accounting for a new transaction.
// This should be called at the start of transaction execution.
func (m *MemoryManager) BeginTransaction(ctx context.Context) {
	// Transaction monitor is typically set up by the connExecutor
	// when creating the transaction state.
}

// EndTransaction finalizes memory accounting for the current transaction.
// This should be called at the end of transaction execution.
func (m *MemoryManager) EndTransaction(ctx context.Context) {
	// Transaction monitor cleanup is handled by the connExecutor
	// through the transaction state.
}

// Reset resets the memory manager state, typically called when
// closing the connection executor.
func (m *MemoryManager) Reset(ctx context.Context) {
	m.currentStmtAccount.Close(ctx)
}

// GetCurrentUsage returns the current memory usage for the session.
func (m *MemoryManager) GetCurrentUsage() int64 {
	if m.sessionMon != nil {
		return m.sessionMon.AllocBytes()
	}
	return 0
}

// GetMetrics returns the memory metrics for this manager.
func (m *MemoryManager) GetMetrics() *MemoryMetrics {
	return m.memMetrics
}
