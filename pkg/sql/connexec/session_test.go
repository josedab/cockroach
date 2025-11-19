// Copyright 2024 The Cockroach Authors.
//
// Use of this software is governed by the CockroachDB Software License
// included in the /LICENSE file.

package connexec

import (
	"testing"

	"github.com/cockroachdb/cockroach/pkg/sql/sessiondata"
	"github.com/stretchr/testify/require"
)

func TestSessionManager_PreparedStatements(t *testing.T) {
	stack := sessiondata.NewStack(&sessiondata.SessionData{})
	sm := NewSessionManager(stack)

	// Prepare a statement
	err := sm.Prepare("stmt1", "SELECT 1", nil)
	require.NoError(t, err)
	require.Equal(t, 1, sm.PreparedStatementCount())

	// Get prepared statement
	stmt, ok := sm.GetPrepared("stmt1")
	require.True(t, ok)
	require.Equal(t, "stmt1", stmt.Name)
	require.Equal(t, "SELECT 1", stmt.SQL)

	// Cannot prepare with same name
	err = sm.Prepare("stmt1", "SELECT 2", nil)
	require.Error(t, err)

	// Deallocate statement
	err = sm.Deallocate("stmt1")
	require.NoError(t, err)
	require.Equal(t, 0, sm.PreparedStatementCount())

	// Cannot deallocate non-existent statement
	err = sm.Deallocate("nonexistent")
	require.Error(t, err)
}

func TestSessionManager_Portals(t *testing.T) {
	stack := sessiondata.NewStack(&sessiondata.SessionData{})
	sm := NewSessionManager(stack)

	// Prepare a statement first
	err := sm.Prepare("stmt1", "SELECT $1", nil)
	require.NoError(t, err)

	// Bind portal
	err = sm.BindPortal("portal1", "stmt1", []interface{}{1})
	require.NoError(t, err)
	require.Equal(t, 1, sm.PortalCount())

	// Get portal
	portal, ok := sm.GetPortal("portal1")
	require.True(t, ok)
	require.Equal(t, "portal1", portal.Name)
	require.Equal(t, "stmt1", portal.StmtName)
	require.Equal(t, []interface{}{1}, portal.Args)

	// Close portal
	sm.ClosePortal("portal1")
	require.Equal(t, 0, sm.PortalCount())

	// Get non-existent portal
	_, ok = sm.GetPortal("nonexistent")
	require.False(t, ok)
}

func TestSessionManager_DeallocateAll(t *testing.T) {
	stack := sessiondata.NewStack(&sessiondata.SessionData{})
	sm := NewSessionManager(stack)

	// Prepare multiple statements
	err := sm.Prepare("stmt1", "SELECT 1", nil)
	require.NoError(t, err)
	err = sm.Prepare("stmt2", "SELECT 2", nil)
	require.NoError(t, err)
	require.Equal(t, 2, sm.PreparedStatementCount())

	// Deallocate all
	sm.DeallocateAll()
	require.Equal(t, 0, sm.PreparedStatementCount())
}

func TestSessionManager_CloseAllPortals(t *testing.T) {
	stack := sessiondata.NewStack(&sessiondata.SessionData{})
	sm := NewSessionManager(stack)

	// Create multiple portals
	err := sm.BindPortal("portal1", "", nil)
	require.NoError(t, err)
	err = sm.BindPortal("portal2", "", nil)
	require.NoError(t, err)
	require.Equal(t, 2, sm.PortalCount())

	// Close all portals
	sm.CloseAllPortals()
	require.Equal(t, 0, sm.PortalCount())
}

func TestSessionManager_Reset(t *testing.T) {
	stack := sessiondata.NewStack(&sessiondata.SessionData{})
	sm := NewSessionManager(stack)

	// Add prepared statement and portal
	err := sm.Prepare("stmt1", "SELECT 1", nil)
	require.NoError(t, err)
	err = sm.BindPortal("portal1", "", nil)
	require.NoError(t, err)

	// Reset
	sm.Reset()
	require.Equal(t, 0, sm.PreparedStatementCount())
	require.Equal(t, 0, sm.PortalCount())
}

func TestSessionManager_UnnamedPreparedStatement(t *testing.T) {
	stack := sessiondata.NewStack(&sessiondata.SessionData{})
	sm := NewSessionManager(stack)

	// Can prepare unnamed statement multiple times
	err := sm.Prepare("", "SELECT 1", nil)
	require.NoError(t, err)
	err = sm.Prepare("", "SELECT 2", nil)
	require.NoError(t, err)
}

func TestSessionManager_TypeHints(t *testing.T) {
	stack := sessiondata.NewStack(&sessiondata.SessionData{})
	sm := NewSessionManager(stack)

	// Prepare with type hints
	typeHints := []uint32{1, 2, 3}
	err := sm.Prepare("stmt1", "SELECT $1, $2, $3", typeHints)
	require.NoError(t, err)

	stmt, ok := sm.GetPrepared("stmt1")
	require.True(t, ok)
	require.Equal(t, typeHints, stmt.TypeHints)
}
