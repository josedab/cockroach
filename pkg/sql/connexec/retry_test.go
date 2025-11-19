// Copyright 2024 The Cockroach Authors.
//
// Use of this software is governed by the CockroachDB Software License
// included in the /LICENSE file.

package connexec

import (
	"testing"

	"github.com/cockroachdb/cockroach/pkg/sql/pgwire/pgcode"
	"github.com/cockroachdb/cockroach/pkg/sql/pgwire/pgerror"
	"github.com/cockroachdb/errors"
	"github.com/stretchr/testify/require"
)

func TestRetryManager_BasicState(t *testing.T) {
	rm := NewRetryManager(3)

	require.True(t, rm.CanAutoRetry())
	require.Equal(t, 0, rm.GetRetryCount())
	require.Equal(t, CmdPos(0), rm.GetRewindPos())
}

func TestRetryManager_RecordStatements(t *testing.T) {
	rm := NewRetryManager(3)

	rm.RecordStatement("SELECT 1", true)
	rm.RecordStatement("INSERT INTO t VALUES (1)", true)

	stmts := rm.GetRetryStatements()
	require.Len(t, stmts, 2)
	require.Equal(t, "SELECT 1", stmts[0].SQL)
	require.Equal(t, "INSERT INTO t VALUES (1)", stmts[1].SQL)
}

func TestRetryManager_RewindPos(t *testing.T) {
	rm := NewRetryManager(3)

	rm.SetRewindPos(5)
	require.Equal(t, CmdPos(5), rm.GetRewindPos())
}

func TestRetryManager_RetryCount(t *testing.T) {
	rm := NewRetryManager(3)

	// First retry
	require.True(t, rm.PrepareForRetry())
	require.Equal(t, 1, rm.GetRetryCount())
	require.True(t, rm.CanAutoRetry())

	// Second retry
	require.True(t, rm.PrepareForRetry())
	require.Equal(t, 2, rm.GetRetryCount())
	require.True(t, rm.CanAutoRetry())

	// Third retry (last one)
	require.True(t, rm.PrepareForRetry())
	require.Equal(t, 3, rm.GetRetryCount())
	require.False(t, rm.CanAutoRetry())

	// No more retries
	require.False(t, rm.PrepareForRetry())
}

func TestRetryManager_DisableRetry(t *testing.T) {
	rm := NewRetryManager(3)

	require.True(t, rm.CanAutoRetry())

	rm.DisableRetry()
	require.False(t, rm.CanAutoRetry())

	// Cannot retry even if max not reached
	require.False(t, rm.PrepareForRetry())
}

func TestRetryManager_ClearState(t *testing.T) {
	rm := NewRetryManager(3)

	// Set up some state
	rm.SetRewindPos(10)
	rm.RecordStatement("SELECT 1", true)
	rm.PrepareForRetry()
	rm.DisableRetry()

	// Clear state
	rm.ClearRetryState()

	require.Equal(t, CmdPos(0), rm.GetRewindPos())
	require.True(t, rm.CanAutoRetry())
	require.Equal(t, 0, rm.GetRetryCount())
	require.Equal(t, 0, rm.StatementCount())
}

func TestRetryManager_Reset(t *testing.T) {
	rm := NewRetryManager(3)

	// Set up some state
	rm.SetRewindPos(10)
	rm.RecordStatement("SELECT 1", true)
	rm.PrepareForRetry()

	// Reset
	rm.Reset()

	require.Equal(t, CmdPos(0), rm.GetRewindPos())
	require.True(t, rm.CanAutoRetry())
	require.Equal(t, 0, rm.GetRetryCount())
	require.Equal(t, 0, rm.StatementCount())
}

func TestRetryManager_ResetForNewTransaction(t *testing.T) {
	rm := NewRetryManager(3)

	// Set up some state
	rm.RecordStatement("SELECT 1", true)
	rm.DisableRetry()

	// Reset for new transaction
	rm.ResetForNewTransaction(15)

	require.Equal(t, CmdPos(15), rm.GetRewindPos())
	require.True(t, rm.CanAutoRetry())
	require.Equal(t, 0, rm.StatementCount())
}

func TestRetryManager_IsRetryableError(t *testing.T) {
	rm := NewRetryManager(3)

	// Nil error is not retryable
	require.False(t, rm.IsRetryableError(nil))

	// Regular error is not retryable
	require.False(t, rm.IsRetryableError(errors.New("regular error")))

	// Serialization failure is retryable
	serErr := pgerror.New(pgcode.SerializationFailure, "serialization failure")
	require.True(t, rm.IsRetryableError(serErr))
}

func TestRetryManager_CanRetryError(t *testing.T) {
	rm := NewRetryManager(3)

	serErr := pgerror.New(pgcode.SerializationFailure, "serialization failure")

	// Can retry when retry is enabled
	require.True(t, rm.CanRetryError(serErr))

	// Cannot retry when disabled
	rm.DisableRetry()
	require.False(t, rm.CanRetryError(serErr))
}

func TestRetryManager_SetCanRetry(t *testing.T) {
	rm := NewRetryManager(3)

	rm.SetCanRetry(false)
	require.False(t, rm.CanAutoRetry())

	rm.SetCanRetry(true)
	require.True(t, rm.CanAutoRetry())
}
