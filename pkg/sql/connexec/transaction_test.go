// Copyright 2024 The Cockroach Authors.
//
// Use of this software is governed by the CockroachDB Software License
// included in the /LICENSE file.

package connexec

import (
	"context"
	"testing"

	"github.com/cockroachdb/cockroach/pkg/sql/sem/tree"
	"github.com/stretchr/testify/require"
)

func TestTransactionManager_BasicLifecycle(t *testing.T) {
	ctx := context.Background()
	tm := NewTransactionManager()

	// Initial state
	require.Equal(t, TxnStateNoTxn, tm.State())
	require.False(t, tm.InTransaction())

	// Begin transaction
	err := tm.Begin(ctx, nil, false, tree.SerializableIsolation, tree.NormalUserPriority, false)
	require.NoError(t, err)
	require.Equal(t, TxnStateOpen, tm.State())
	require.True(t, tm.InTransaction())
	require.False(t, tm.IsImplicit())

	// Commit transaction
	err = tm.Commit(ctx)
	require.NoError(t, err)
	require.Equal(t, TxnStateCommitWait, tm.State())

	// Finish commit
	tm.FinishCommit(ctx)
	require.Equal(t, TxnStateNoTxn, tm.State())
	require.False(t, tm.InTransaction())
}

func TestTransactionManager_ImplicitTransaction(t *testing.T) {
	ctx := context.Background()
	tm := NewTransactionManager()

	err := tm.Begin(ctx, nil, true, tree.SerializableIsolation, tree.NormalUserPriority, false)
	require.NoError(t, err)
	require.True(t, tm.IsImplicit())
}

func TestTransactionManager_Rollback(t *testing.T) {
	ctx := context.Background()
	tm := NewTransactionManager()

	// Begin and rollback
	err := tm.Begin(ctx, nil, false, tree.SerializableIsolation, tree.NormalUserPriority, false)
	require.NoError(t, err)

	err = tm.Rollback(ctx)
	require.NoError(t, err)
	require.Equal(t, TxnStateNoTxn, tm.State())

	// Cannot rollback when no transaction
	err = tm.Rollback(ctx)
	require.Error(t, err)
}

func TestTransactionManager_Abort(t *testing.T) {
	ctx := context.Background()
	tm := NewTransactionManager()

	// Begin and abort
	err := tm.Begin(ctx, nil, false, tree.SerializableIsolation, tree.NormalUserPriority, false)
	require.NoError(t, err)

	tm.Abort(ctx)
	require.Equal(t, TxnStateAborted, tm.State())
}

func TestTransactionManager_Savepoints(t *testing.T) {
	ctx := context.Background()
	tm := NewTransactionManager()

	// Begin transaction
	err := tm.Begin(ctx, nil, false, tree.SerializableIsolation, tree.NormalUserPriority, false)
	require.NoError(t, err)

	// Create savepoints
	err = tm.Savepoint("sp1")
	require.NoError(t, err)
	require.Equal(t, 1, tm.SavepointCount())
	require.True(t, tm.HasSavepoint("sp1"))

	err = tm.Savepoint("sp2")
	require.NoError(t, err)
	require.Equal(t, 2, tm.SavepointCount())

	// Rollback to savepoint
	err = tm.RollbackToSavepoint("sp1")
	require.NoError(t, err)
	require.Equal(t, 1, tm.SavepointCount())
	require.False(t, tm.HasSavepoint("sp2"))

	// Release savepoint
	err = tm.ReleaseSavepoint("sp1")
	require.NoError(t, err)
	require.Equal(t, 0, tm.SavepointCount())
}

func TestTransactionManager_SavepointErrors(t *testing.T) {
	ctx := context.Background()
	tm := NewTransactionManager()

	// Cannot create savepoint without transaction
	err := tm.Savepoint("sp1")
	require.Error(t, err)

	// Begin transaction
	err = tm.Begin(ctx, nil, false, tree.SerializableIsolation, tree.NormalUserPriority, false)
	require.NoError(t, err)

	// Cannot rollback to non-existent savepoint
	err = tm.RollbackToSavepoint("nonexistent")
	require.Error(t, err)

	// Cannot release non-existent savepoint
	err = tm.ReleaseSavepoint("nonexistent")
	require.Error(t, err)
}

func TestTransactionManager_RollbackToSavepointFromAborted(t *testing.T) {
	ctx := context.Background()
	tm := NewTransactionManager()

	// Begin transaction and create savepoint
	err := tm.Begin(ctx, nil, false, tree.SerializableIsolation, tree.NormalUserPriority, false)
	require.NoError(t, err)

	err = tm.Savepoint("sp1")
	require.NoError(t, err)

	// Abort and then rollback to savepoint
	tm.Abort(ctx)
	require.Equal(t, TxnStateAborted, tm.State())

	err = tm.RollbackToSavepoint("sp1")
	require.NoError(t, err)
	require.Equal(t, TxnStateOpen, tm.State())
}

func TestTransactionManager_IsolationLevel(t *testing.T) {
	ctx := context.Background()
	tm := NewTransactionManager()

	err := tm.Begin(ctx, nil, false, tree.ReadCommittedIsolation, tree.NormalUserPriority, false)
	require.NoError(t, err)
	require.Equal(t, tree.ReadCommittedIsolation, tm.IsolationLevel())

	tm.SetIsolationLevel(tree.SerializableIsolation)
	require.Equal(t, tree.SerializableIsolation, tm.IsolationLevel())
}

func TestTransactionManager_Priority(t *testing.T) {
	ctx := context.Background()
	tm := NewTransactionManager()

	err := tm.Begin(ctx, nil, false, tree.SerializableIsolation, tree.High, false)
	require.NoError(t, err)
	require.Equal(t, tree.High, tm.Priority())

	tm.SetPriority(tree.Low)
	require.Equal(t, tree.Low, tm.Priority())
}

func TestTransactionManager_ReadOnly(t *testing.T) {
	ctx := context.Background()
	tm := NewTransactionManager()

	err := tm.Begin(ctx, nil, false, tree.SerializableIsolation, tree.NormalUserPriority, true)
	require.NoError(t, err)
	require.True(t, tm.ReadOnly())

	tm.SetReadOnly(false)
	require.False(t, tm.ReadOnly())
}

func TestTransactionManager_Reset(t *testing.T) {
	ctx := context.Background()
	tm := NewTransactionManager()

	// Begin transaction with non-default values
	err := tm.Begin(ctx, nil, true, tree.ReadCommittedIsolation, tree.High, true)
	require.NoError(t, err)
	err = tm.Savepoint("sp1")
	require.NoError(t, err)

	// Reset
	tm.Reset()
	require.Equal(t, TxnStateNoTxn, tm.State())
	require.False(t, tm.IsImplicit())
	require.Equal(t, tree.SerializableIsolation, tm.IsolationLevel())
	require.Equal(t, tree.UnspecifiedUserPriority, tm.Priority())
	require.False(t, tm.ReadOnly())
	require.Equal(t, 0, tm.SavepointCount())
}

func TestTransactionManager_DoubleBegin(t *testing.T) {
	ctx := context.Background()
	tm := NewTransactionManager()

	err := tm.Begin(ctx, nil, false, tree.SerializableIsolation, tree.NormalUserPriority, false)
	require.NoError(t, err)

	// Cannot begin when transaction already in progress
	err = tm.Begin(ctx, nil, false, tree.SerializableIsolation, tree.NormalUserPriority, false)
	require.Error(t, err)
}

func TestTxnState_String(t *testing.T) {
	require.Equal(t, "NoTxn", TxnStateNoTxn.String())
	require.Equal(t, "Open", TxnStateOpen.String())
	require.Equal(t, "Aborted", TxnStateAborted.String())
	require.Equal(t, "CommitWait", TxnStateCommitWait.String())
}
