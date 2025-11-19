// Copyright 2024 The Cockroach Authors.
//
// Use of this software is governed by the CockroachDB Software License
// included in the /LICENSE file.

package connexec

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestMetricsManager_StatementMetrics(t *testing.T) {
	mm := NewMetricsManager()
	ctx := context.Background()
	stmt := &Statement{SQL: "SELECT 1"}

	// Begin and end statement
	mm.BeginStatement(ctx, stmt)
	time.Sleep(time.Millisecond) // Ensure some latency
	mm.EndStatement(ctx, stmt, nil)

	metrics := mm.GetStatementMetrics()
	require.Equal(t, int64(1), metrics.TotalCount)
	require.Equal(t, int64(1), metrics.SuccessCount)
	require.Equal(t, int64(0), metrics.ErrorCount)
	require.Greater(t, metrics.TotalLatency, time.Duration(0))
}

func TestMetricsManager_StatementError(t *testing.T) {
	mm := NewMetricsManager()
	ctx := context.Background()
	stmt := &Statement{SQL: "SELECT 1"}

	mm.BeginStatement(ctx, stmt)
	mm.EndStatement(ctx, stmt, context.DeadlineExceeded)

	metrics := mm.GetStatementMetrics()
	require.Equal(t, int64(1), metrics.TotalCount)
	require.Equal(t, int64(0), metrics.SuccessCount)
	require.Equal(t, int64(1), metrics.ErrorCount)
}

func TestMetricsManager_TransactionMetrics(t *testing.T) {
	mm := NewMetricsManager()
	ctx := context.Background()
	tm := NewTransactionManager()

	// Committed transaction
	mm.BeginTransaction(ctx, tm)
	time.Sleep(time.Millisecond)
	mm.EndTransaction(ctx, tm, true)

	metrics := mm.GetTransactionMetrics()
	require.Equal(t, int64(1), metrics.TotalCount)
	require.Equal(t, int64(1), metrics.CommitCount)
	require.Equal(t, int64(0), metrics.RollbackCount)
}

func TestMetricsManager_TransactionRollback(t *testing.T) {
	mm := NewMetricsManager()
	ctx := context.Background()
	tm := NewTransactionManager()

	mm.BeginTransaction(ctx, tm)
	mm.EndTransaction(ctx, tm, false)

	metrics := mm.GetTransactionMetrics()
	require.Equal(t, int64(1), metrics.TotalCount)
	require.Equal(t, int64(0), metrics.CommitCount)
	require.Equal(t, int64(1), metrics.RollbackCount)
}

func TestMetricsManager_Retry(t *testing.T) {
	mm := NewMetricsManager()

	mm.RecordRetry()
	mm.RecordRetry()

	metrics := mm.GetTransactionMetrics()
	require.Equal(t, int64(2), metrics.RetryCount)
}

func TestMetricsManager_StatementTypes(t *testing.T) {
	mm := NewMetricsManager()

	mm.RecordStatementType("SELECT")
	mm.RecordStatementType("INSERT")
	mm.RecordStatementType("UPDATE")
	mm.RecordStatementType("DELETE")
	mm.RecordStatementType("DDL")
	mm.RecordStatementType("SELECT")

	metrics := mm.GetStatementMetrics()
	require.Equal(t, int64(2), metrics.SelectCount)
	require.Equal(t, int64(1), metrics.InsertCount)
	require.Equal(t, int64(1), metrics.UpdateCount)
	require.Equal(t, int64(1), metrics.DeleteCount)
	require.Equal(t, int64(1), metrics.DDLCount)
}

func TestMetricsManager_Reset(t *testing.T) {
	mm := NewMetricsManager()
	ctx := context.Background()
	stmt := &Statement{SQL: "SELECT 1"}

	mm.BeginStatement(ctx, stmt)
	mm.EndStatement(ctx, stmt, nil)
	mm.RecordRetry()

	mm.Reset()

	stmtMetrics := mm.GetStatementMetrics()
	require.Equal(t, int64(0), stmtMetrics.TotalCount)

	txnMetrics := mm.GetTransactionMetrics()
	require.Equal(t, int64(0), txnMetrics.RetryCount)
}

func TestMetricsManager_AverageLatency(t *testing.T) {
	mm := NewMetricsManager()
	ctx := context.Background()

	// No statements yet
	require.Equal(t, time.Duration(0), mm.AverageStatementLatency())
	require.Equal(t, time.Duration(0), mm.AverageTransactionLatency())

	// Execute some statements
	stmt := &Statement{SQL: "SELECT 1"}
	mm.BeginStatement(ctx, stmt)
	time.Sleep(time.Millisecond)
	mm.EndStatement(ctx, stmt, nil)

	require.Greater(t, mm.AverageStatementLatency(), time.Duration(0))
}

func TestMetricsManager_SuccessRate(t *testing.T) {
	mm := NewMetricsManager()
	ctx := context.Background()
	stmt := &Statement{SQL: "SELECT 1"}

	// No statements yet
	require.Equal(t, 100.0, mm.StatementSuccessRate())
	require.Equal(t, 100.0, mm.TransactionCommitRate())

	// One success, one failure
	mm.BeginStatement(ctx, stmt)
	mm.EndStatement(ctx, stmt, nil)
	mm.BeginStatement(ctx, stmt)
	mm.EndStatement(ctx, stmt, context.DeadlineExceeded)

	require.Equal(t, 50.0, mm.StatementSuccessRate())
}

func TestMetricsManager_Observer(t *testing.T) {
	mm := NewMetricsManager()
	ctx := context.Background()
	stmt := &Statement{SQL: "SELECT 1"}
	tm := NewTransactionManager()

	// Create mock observer
	obs := &mockObserver{}
	mm.AddObserver(obs)

	// Execute statement
	mm.BeginStatement(ctx, stmt)
	require.True(t, obs.stmtStarted)

	mm.EndStatement(ctx, stmt, nil)
	require.True(t, obs.stmtEnded)

	// Execute transaction
	mm.BeginTransaction(ctx, tm)
	require.True(t, obs.txnStarted)

	mm.EndTransaction(ctx, tm, true)
	require.True(t, obs.txnEnded)

	// Remove observer
	mm.RemoveObserver(obs)
	obs.stmtStarted = false

	mm.BeginStatement(ctx, stmt)
	require.False(t, obs.stmtStarted)
}

type mockObserver struct {
	stmtStarted bool
	stmtEnded   bool
	txnStarted  bool
	txnEnded    bool
}

func (m *mockObserver) OnStatementStart(ctx context.Context, stmt StatementContext) {
	m.stmtStarted = true
}

func (m *mockObserver) OnStatementEnd(ctx context.Context, stmt StatementContext, err error) {
	m.stmtEnded = true
}

func (m *mockObserver) OnTransactionStart(ctx context.Context, txn TransactionContext) {
	m.txnStarted = true
}

func (m *mockObserver) OnTransactionEnd(ctx context.Context, txn TransactionContext, committed bool) {
	m.txnEnded = true
}
