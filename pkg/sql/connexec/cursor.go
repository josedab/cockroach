// Copyright 2024 The Cockroach Authors.
//
// Use of this software is governed by the CockroachDB Software License
// included in the /LICENSE file.

package connexec

import (
	"github.com/cockroachdb/errors"
)

// CursorManager manages SQL cursors for a connection executor.
// Cursors can be declared with HOLD (surviving beyond their transaction)
// or without HOLD (bound to their transaction).
type CursorManager struct {
	// cursors maps cursor names to cursor entries.
	cursors map[string]*Cursor
}

// Cursor represents a SQL cursor.
type Cursor struct {
	// Name is the name of the cursor.
	Name string

	// SQL is the query associated with the cursor.
	SQL string

	// WithHold indicates whether the cursor survives beyond its transaction.
	WithHold bool

	// IsOpen indicates whether the cursor is currently open.
	IsOpen bool

	// Position is the current position in the result set.
	Position int64

	// RowCount is the total number of rows in the result set, if known.
	RowCount int64
}

// NewCursorManager creates a new CursorManager.
func NewCursorManager() *CursorManager {
	return &CursorManager{
		cursors: make(map[string]*Cursor),
	}
}

// Declare declares a new cursor with the given name and query.
func (c *CursorManager) Declare(name, sql string, withHold bool) error {
	if _, exists := c.cursors[name]; exists {
		return errors.Newf("cursor %q already exists", name)
	}

	c.cursors[name] = &Cursor{
		Name:     name,
		SQL:      sql,
		WithHold: withHold,
		IsOpen:   true,
		Position: 0,
	}

	return nil
}

// Open opens a cursor that was previously declared.
func (c *CursorManager) Open(name string) error {
	cursor, exists := c.cursors[name]
	if !exists {
		return errors.Newf("cursor %q does not exist", name)
	}

	if cursor.IsOpen {
		return errors.Newf("cursor %q is already open", name)
	}

	cursor.IsOpen = true
	cursor.Position = 0
	return nil
}

// Close closes the cursor with the given name.
func (c *CursorManager) Close(name string) error {
	cursor, exists := c.cursors[name]
	if !exists {
		return errors.Newf("cursor %q does not exist", name)
	}

	cursor.IsOpen = false
	delete(c.cursors, name)
	return nil
}

// CloseAll closes all cursors.
func (c *CursorManager) CloseAll() {
	c.cursors = make(map[string]*Cursor)
}

// CloseTransactionCursors closes all cursors that are not declared WITH HOLD.
// This should be called when a transaction ends.
func (c *CursorManager) CloseTransactionCursors() {
	for name, cursor := range c.cursors {
		if !cursor.WithHold {
			delete(c.cursors, name)
		}
	}
}

// Get returns the cursor with the given name.
func (c *CursorManager) Get(name string) (*Cursor, bool) {
	cursor, exists := c.cursors[name]
	return cursor, exists
}

// Exists returns true if a cursor with the given name exists.
func (c *CursorManager) Exists(name string) bool {
	_, exists := c.cursors[name]
	return exists
}

// IsOpen returns true if the cursor with the given name is open.
func (c *CursorManager) IsOpen(name string) bool {
	cursor, exists := c.cursors[name]
	if !exists {
		return false
	}
	return cursor.IsOpen
}

// SetPosition sets the position of the cursor.
func (c *CursorManager) SetPosition(name string, pos int64) error {
	cursor, exists := c.cursors[name]
	if !exists {
		return errors.Newf("cursor %q does not exist", name)
	}
	cursor.Position = pos
	return nil
}

// GetPosition returns the current position of the cursor.
func (c *CursorManager) GetPosition(name string) (int64, error) {
	cursor, exists := c.cursors[name]
	if !exists {
		return 0, errors.Newf("cursor %q does not exist", name)
	}
	return cursor.Position, nil
}

// IncrementPosition increments the cursor position by the given amount.
func (c *CursorManager) IncrementPosition(name string, delta int64) error {
	cursor, exists := c.cursors[name]
	if !exists {
		return errors.Newf("cursor %q does not exist", name)
	}
	cursor.Position += delta
	return nil
}

// SetRowCount sets the total row count for the cursor.
func (c *CursorManager) SetRowCount(name string, count int64) error {
	cursor, exists := c.cursors[name]
	if !exists {
		return errors.Newf("cursor %q does not exist", name)
	}
	cursor.RowCount = count
	return nil
}

// Count returns the number of cursors.
func (c *CursorManager) Count() int {
	return len(c.cursors)
}

// Names returns the names of all cursors.
func (c *CursorManager) Names() []string {
	names := make([]string, 0, len(c.cursors))
	for name := range c.cursors {
		names = append(names, name)
	}
	return names
}

// Reset resets the cursor manager, closing all cursors.
func (c *CursorManager) Reset() {
	c.cursors = make(map[string]*Cursor)
}

// HoldCursors returns the cursors declared WITH HOLD.
func (c *CursorManager) HoldCursors() []*Cursor {
	result := make([]*Cursor, 0)
	for _, cursor := range c.cursors {
		if cursor.WithHold {
			result = append(result, cursor)
		}
	}
	return result
}

// TransactionCursors returns the cursors not declared WITH HOLD.
func (c *CursorManager) TransactionCursors() []*Cursor {
	result := make([]*Cursor, 0)
	for _, cursor := range c.cursors {
		if !cursor.WithHold {
			result = append(result, cursor)
		}
	}
	return result
}
