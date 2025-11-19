// Copyright 2024 The Cockroach Authors.
//
// Use of this software is governed by the CockroachDB Software License
// included in the /LICENSE file.

package connexec

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCursorManager_BasicOperations(t *testing.T) {
	cm := NewCursorManager()

	// Declare cursor
	err := cm.Declare("cursor1", "SELECT * FROM t", false)
	require.NoError(t, err)
	require.Equal(t, 1, cm.Count())
	require.True(t, cm.Exists("cursor1"))
	require.True(t, cm.IsOpen("cursor1"))

	// Get cursor
	cursor, ok := cm.Get("cursor1")
	require.True(t, ok)
	require.Equal(t, "cursor1", cursor.Name)
	require.Equal(t, "SELECT * FROM t", cursor.SQL)
	require.False(t, cursor.WithHold)

	// Close cursor
	err = cm.Close("cursor1")
	require.NoError(t, err)
	require.Equal(t, 0, cm.Count())
	require.False(t, cm.Exists("cursor1"))
}

func TestCursorManager_WithHold(t *testing.T) {
	cm := NewCursorManager()

	// Declare WITH HOLD cursor
	err := cm.Declare("cursor1", "SELECT * FROM t", true)
	require.NoError(t, err)

	cursor, ok := cm.Get("cursor1")
	require.True(t, ok)
	require.True(t, cursor.WithHold)
}

func TestCursorManager_DuplicateCursor(t *testing.T) {
	cm := NewCursorManager()

	err := cm.Declare("cursor1", "SELECT 1", false)
	require.NoError(t, err)

	// Cannot declare with same name
	err = cm.Declare("cursor1", "SELECT 2", false)
	require.Error(t, err)
}

func TestCursorManager_CloseTransactionCursors(t *testing.T) {
	cm := NewCursorManager()

	// Declare one WITH HOLD and one without
	err := cm.Declare("hold_cursor", "SELECT 1", true)
	require.NoError(t, err)
	err = cm.Declare("txn_cursor", "SELECT 2", false)
	require.NoError(t, err)
	require.Equal(t, 2, cm.Count())

	// Close transaction cursors
	cm.CloseTransactionCursors()
	require.Equal(t, 1, cm.Count())
	require.True(t, cm.Exists("hold_cursor"))
	require.False(t, cm.Exists("txn_cursor"))
}

func TestCursorManager_Position(t *testing.T) {
	cm := NewCursorManager()

	err := cm.Declare("cursor1", "SELECT * FROM t", false)
	require.NoError(t, err)

	// Get initial position
	pos, err := cm.GetPosition("cursor1")
	require.NoError(t, err)
	require.Equal(t, int64(0), pos)

	// Set position
	err = cm.SetPosition("cursor1", 5)
	require.NoError(t, err)
	pos, err = cm.GetPosition("cursor1")
	require.NoError(t, err)
	require.Equal(t, int64(5), pos)

	// Increment position
	err = cm.IncrementPosition("cursor1", 3)
	require.NoError(t, err)
	pos, err = cm.GetPosition("cursor1")
	require.NoError(t, err)
	require.Equal(t, int64(8), pos)
}

func TestCursorManager_RowCount(t *testing.T) {
	cm := NewCursorManager()

	err := cm.Declare("cursor1", "SELECT * FROM t", false)
	require.NoError(t, err)

	err = cm.SetRowCount("cursor1", 100)
	require.NoError(t, err)

	cursor, ok := cm.Get("cursor1")
	require.True(t, ok)
	require.Equal(t, int64(100), cursor.RowCount)
}

func TestCursorManager_Names(t *testing.T) {
	cm := NewCursorManager()

	err := cm.Declare("cursor1", "SELECT 1", false)
	require.NoError(t, err)
	err = cm.Declare("cursor2", "SELECT 2", false)
	require.NoError(t, err)

	names := cm.Names()
	require.Len(t, names, 2)
	require.Contains(t, names, "cursor1")
	require.Contains(t, names, "cursor2")
}

func TestCursorManager_CloseAll(t *testing.T) {
	cm := NewCursorManager()

	err := cm.Declare("cursor1", "SELECT 1", false)
	require.NoError(t, err)
	err = cm.Declare("cursor2", "SELECT 2", true)
	require.NoError(t, err)

	cm.CloseAll()
	require.Equal(t, 0, cm.Count())
}

func TestCursorManager_Reset(t *testing.T) {
	cm := NewCursorManager()

	err := cm.Declare("cursor1", "SELECT 1", false)
	require.NoError(t, err)

	cm.Reset()
	require.Equal(t, 0, cm.Count())
}

func TestCursorManager_HoldAndTransactionCursors(t *testing.T) {
	cm := NewCursorManager()

	err := cm.Declare("hold1", "SELECT 1", true)
	require.NoError(t, err)
	err = cm.Declare("hold2", "SELECT 2", true)
	require.NoError(t, err)
	err = cm.Declare("txn1", "SELECT 3", false)
	require.NoError(t, err)

	holdCursors := cm.HoldCursors()
	require.Len(t, holdCursors, 2)

	txnCursors := cm.TransactionCursors()
	require.Len(t, txnCursors, 1)
}

func TestCursorManager_CloseNonExistent(t *testing.T) {
	cm := NewCursorManager()

	err := cm.Close("nonexistent")
	require.Error(t, err)
}

func TestCursorManager_PositionNonExistent(t *testing.T) {
	cm := NewCursorManager()

	_, err := cm.GetPosition("nonexistent")
	require.Error(t, err)

	err = cm.SetPosition("nonexistent", 5)
	require.Error(t, err)

	err = cm.IncrementPosition("nonexistent", 1)
	require.Error(t, err)
}

func TestCursorManager_IsOpenNonExistent(t *testing.T) {
	cm := NewCursorManager()

	require.False(t, cm.IsOpen("nonexistent"))
}
