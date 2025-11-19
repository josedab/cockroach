// Copyright 2024 The Cockroach Authors.
//
// Use of this software is governed by the CockroachDB Software License
// included in the /LICENSE file.

package connexec

import (
	"context"
	"time"
)

// MetricsManager handles metrics collection for a connection executor.
// It tracks statement and transaction metrics including counts, latencies,
// and error rates.
type MetricsManager struct {
	// statementMetrics tracks per-statement metrics.
	statementMetrics *StatementMetrics

	// transactionMetrics tracks per-transaction metrics.
	transactionMetrics *TransactionMetrics

	// observers is a list of observers to notify of execution events.
	observers []ExecutionObserver

	// currentStmtStart is the start time of the current statement.
	currentStmtStart time.Time

	// currentTxnStart is the start time of the current transaction.
	currentTxnStart time.Time
}

// StatementMetrics contains metrics for statement execution.
type StatementMetrics struct {
	// TotalCount is the total number of statements executed.
	TotalCount int64

	// SuccessCount is the number of successfully executed statements.
	SuccessCount int64

	// ErrorCount is the number of statements that failed.
	ErrorCount int64

	// TotalLatency is the total latency of all statements.
	TotalLatency time.Duration

	// SelectCount is the number of SELECT statements.
	SelectCount int64

	// InsertCount is the number of INSERT statements.
	InsertCount int64

	// UpdateCount is the number of UPDATE statements.
	UpdateCount int64

	// DeleteCount is the number of DELETE statements.
	DeleteCount int64

	// DDLCount is the number of DDL statements.
	DDLCount int64
}

// TransactionMetrics contains metrics for transaction execution.
type TransactionMetrics struct {
	// TotalCount is the total number of transactions.
	TotalCount int64

	// CommitCount is the number of committed transactions.
	CommitCount int64

	// RollbackCount is the number of rolled back transactions.
	RollbackCount int64

	// RetryCount is the number of transaction retries.
	RetryCount int64

	// TotalLatency is the total latency of all transactions.
	TotalLatency time.Duration
}

// NewMetricsManager creates a new MetricsManager.
func NewMetricsManager() *MetricsManager {
	return &MetricsManager{
		statementMetrics:   &StatementMetrics{},
		transactionMetrics: &TransactionMetrics{},
		observers:          make([]ExecutionObserver, 0),
	}
}

// AddObserver adds an execution observer.
func (m *MetricsManager) AddObserver(obs ExecutionObserver) {
	m.observers = append(m.observers, obs)
}

// RemoveObserver removes an execution observer.
func (m *MetricsManager) RemoveObserver(obs ExecutionObserver) {
	for i, o := range m.observers {
		if o == obs {
			m.observers = append(m.observers[:i], m.observers[i+1:]...)
			return
		}
	}
}

// BeginStatement marks the start of statement execution.
func (m *MetricsManager) BeginStatement(ctx context.Context, stmt StatementContext) {
	m.currentStmtStart = time.Now()
	m.statementMetrics.TotalCount++

	// Notify observers
	for _, obs := range m.observers {
		obs.OnStatementStart(ctx, stmt)
	}
}

// EndStatement marks the end of statement execution.
func (m *MetricsManager) EndStatement(ctx context.Context, stmt StatementContext, err error) {
	latency := time.Since(m.currentStmtStart)
	m.statementMetrics.TotalLatency += latency

	if err != nil {
		m.statementMetrics.ErrorCount++
	} else {
		m.statementMetrics.SuccessCount++
	}

	// Notify observers
	for _, obs := range m.observers {
		obs.OnStatementEnd(ctx, stmt, err)
	}
}

// RecordSuccess records a successful statement execution.
func (m *MetricsManager) RecordSuccess(stmt StatementContext) {
	m.statementMetrics.SuccessCount++
}

// RecordError records a failed statement execution.
func (m *MetricsManager) RecordError(stmt StatementContext, err error) {
	m.statementMetrics.ErrorCount++
}

// RecordStatementType records the type of statement executed.
func (m *MetricsManager) RecordStatementType(stmtType string) {
	switch stmtType {
	case "SELECT":
		m.statementMetrics.SelectCount++
	case "INSERT":
		m.statementMetrics.InsertCount++
	case "UPDATE":
		m.statementMetrics.UpdateCount++
	case "DELETE":
		m.statementMetrics.DeleteCount++
	case "DDL":
		m.statementMetrics.DDLCount++
	}
}

// BeginTransaction marks the start of transaction execution.
func (m *MetricsManager) BeginTransaction(ctx context.Context, txn TransactionContext) {
	m.currentTxnStart = time.Now()
	m.transactionMetrics.TotalCount++

	// Notify observers
	for _, obs := range m.observers {
		obs.OnTransactionStart(ctx, txn)
	}
}

// EndTransaction marks the end of transaction execution.
func (m *MetricsManager) EndTransaction(ctx context.Context, txn TransactionContext, committed bool) {
	latency := time.Since(m.currentTxnStart)
	m.transactionMetrics.TotalLatency += latency

	if committed {
		m.transactionMetrics.CommitCount++
	} else {
		m.transactionMetrics.RollbackCount++
	}

	// Notify observers
	for _, obs := range m.observers {
		obs.OnTransactionEnd(ctx, txn, committed)
	}
}

// RecordRetry records a transaction retry.
func (m *MetricsManager) RecordRetry() {
	m.transactionMetrics.RetryCount++
}

// GetStatementMetrics returns the current statement metrics.
func (m *MetricsManager) GetStatementMetrics() *StatementMetrics {
	return m.statementMetrics
}

// GetTransactionMetrics returns the current transaction metrics.
func (m *MetricsManager) GetTransactionMetrics() *TransactionMetrics {
	return m.transactionMetrics
}

// Reset resets all metrics to zero.
func (m *MetricsManager) Reset() {
	m.statementMetrics = &StatementMetrics{}
	m.transactionMetrics = &TransactionMetrics{}
}

// AverageStatementLatency returns the average statement latency.
func (m *MetricsManager) AverageStatementLatency() time.Duration {
	if m.statementMetrics.TotalCount == 0 {
		return 0
	}
	return m.statementMetrics.TotalLatency / time.Duration(m.statementMetrics.TotalCount)
}

// AverageTransactionLatency returns the average transaction latency.
func (m *MetricsManager) AverageTransactionLatency() time.Duration {
	if m.transactionMetrics.TotalCount == 0 {
		return 0
	}
	return m.transactionMetrics.TotalLatency / time.Duration(m.transactionMetrics.TotalCount)
}

// StatementSuccessRate returns the statement success rate as a percentage.
func (m *MetricsManager) StatementSuccessRate() float64 {
	if m.statementMetrics.TotalCount == 0 {
		return 100.0
	}
	return float64(m.statementMetrics.SuccessCount) / float64(m.statementMetrics.TotalCount) * 100.0
}

// TransactionCommitRate returns the transaction commit rate as a percentage.
func (m *MetricsManager) TransactionCommitRate() float64 {
	if m.transactionMetrics.TotalCount == 0 {
		return 100.0
	}
	return float64(m.transactionMetrics.CommitCount) / float64(m.transactionMetrics.TotalCount) * 100.0
}
