// Copyright 2024 The Cockroach Authors.
//
// Use of this software is governed by the CockroachDB Software License
// included in the /LICENSE file.

package connexec

import (
	"context"

	"github.com/cockroachdb/cockroach/pkg/security/username"
	"github.com/cockroachdb/cockroach/pkg/sql/sessiondata"
	"github.com/cockroachdb/errors"
)

// SessionManager manages session state for a connection executor.
// This includes session variables, prepared statements, and other
// session-scoped state.
type SessionManager struct {
	// sessionDataStack contains the user-configurable connection variables.
	sessionDataStack *sessiondata.Stack

	// preparedStatements maps statement names to prepared statements.
	preparedStatements map[string]*PreparedStatement

	// portals maps portal names to portals.
	portals map[string]*Portal
}

// PreparedStatement represents a prepared statement in the session.
type PreparedStatement struct {
	// Name is the name of the prepared statement.
	Name string

	// SQL is the original SQL text.
	SQL string

	// TypeHints contains any type hints provided during preparation.
	TypeHints []uint32
}

// Portal represents a portal (bound prepared statement) in the session.
type Portal struct {
	// Name is the name of the portal.
	Name string

	// StmtName is the name of the prepared statement this portal is bound to.
	StmtName string

	// Args contains the bound parameter values.
	Args []interface{}
}

// NewSessionManager creates a new SessionManager with the given session data.
func NewSessionManager(sessionDataStack *sessiondata.Stack) *SessionManager {
	return &SessionManager{
		sessionDataStack:   sessionDataStack,
		preparedStatements: make(map[string]*PreparedStatement),
		portals:            make(map[string]*Portal),
	}
}

// User returns the current SQL username.
func (s *SessionManager) User() username.SQLUsername {
	return s.sessionDataStack.Top().User()
}

// Database returns the current database name.
func (s *SessionManager) Database() string {
	return s.sessionDataStack.Top().Database
}

// SearchPath returns the current search path.
func (s *SessionManager) SearchPath() sessiondata.SearchPath {
	return s.sessionDataStack.Top().SearchPath
}

// ApplicationName returns the application name for this session.
func (s *SessionManager) ApplicationName() string {
	return s.sessionDataStack.Top().ApplicationName
}

// SessionData returns the full session data.
func (s *SessionManager) SessionData() *sessiondata.SessionData {
	return s.sessionDataStack.Top()
}

// SessionDataStack returns the session data stack.
func (s *SessionManager) SessionDataStack() *sessiondata.Stack {
	return s.sessionDataStack
}

// Set sets a session variable to the given value.
// Returns an error if the variable name is invalid or the value is not allowed.
func (s *SessionManager) Set(ctx context.Context, key, value string) error {
	// Setting session variables is handled by the session mutator
	// in the main connExecutor. This method serves as the module interface.
	return errors.New("Set not implemented in SessionManager; use session mutator")
}

// Get returns the value of a session variable.
func (s *SessionManager) Get(key string) (string, bool) {
	// Getting session variables requires access to the varGen map
	// in the connExecutor. This method serves as the module interface.
	return "", false
}

// Prepare creates a new prepared statement with the given name and SQL.
func (s *SessionManager) Prepare(name, sql string, typeHints []uint32) error {
	if _, exists := s.preparedStatements[name]; exists && name != "" {
		return errors.Newf("prepared statement %q already exists", name)
	}

	s.preparedStatements[name] = &PreparedStatement{
		Name:      name,
		SQL:       sql,
		TypeHints: typeHints,
	}
	return nil
}

// GetPrepared returns the prepared statement with the given name.
func (s *SessionManager) GetPrepared(name string) (*PreparedStatement, bool) {
	stmt, ok := s.preparedStatements[name]
	return stmt, ok
}

// Deallocate removes the prepared statement with the given name.
func (s *SessionManager) Deallocate(name string) error {
	if _, exists := s.preparedStatements[name]; !exists {
		return errors.Newf("prepared statement %q does not exist", name)
	}
	delete(s.preparedStatements, name)
	return nil
}

// DeallocateAll removes all prepared statements.
func (s *SessionManager) DeallocateAll() {
	s.preparedStatements = make(map[string]*PreparedStatement)
}

// BindPortal creates a new portal bound to the given prepared statement.
func (s *SessionManager) BindPortal(name, stmtName string, args []interface{}) error {
	if _, exists := s.preparedStatements[stmtName]; !exists && stmtName != "" {
		return errors.Newf("prepared statement %q does not exist", stmtName)
	}

	s.portals[name] = &Portal{
		Name:     name,
		StmtName: stmtName,
		Args:     args,
	}
	return nil
}

// GetPortal returns the portal with the given name.
func (s *SessionManager) GetPortal(name string) (*Portal, bool) {
	portal, ok := s.portals[name]
	return portal, ok
}

// ClosePortal removes the portal with the given name.
func (s *SessionManager) ClosePortal(name string) {
	delete(s.portals, name)
}

// CloseAllPortals removes all portals.
func (s *SessionManager) CloseAllPortals() {
	s.portals = make(map[string]*Portal)
}

// PreparedStatementCount returns the number of prepared statements.
func (s *SessionManager) PreparedStatementCount() int {
	return len(s.preparedStatements)
}

// PortalCount returns the number of portals.
func (s *SessionManager) PortalCount() int {
	return len(s.portals)
}

// Reset resets the session manager state.
func (s *SessionManager) Reset() {
	s.preparedStatements = make(map[string]*PreparedStatement)
	s.portals = make(map[string]*Portal)
}
