// Copyright 2024 The Cockroach Authors.
//
// Use of this software is governed by the CockroachDB Software License
// included in the /LICENSE file.

package connexec

import (
	"context"

	"github.com/cockroachdb/cockroach/pkg/kv"
	"github.com/cockroachdb/cockroach/pkg/roachpb"
	"github.com/cockroachdb/cockroach/pkg/sql/sem/tree"
	"github.com/cockroachdb/errors"
)

// TxnState represents the state of a transaction.
type TxnState int

const (
	// TxnStateNoTxn indicates no transaction is active.
	TxnStateNoTxn TxnState = iota
	// TxnStateOpen indicates a transaction is active and accepting commands.
	TxnStateOpen
	// TxnStateAborted indicates the transaction has been aborted and is
	// waiting for a ROLLBACK.
	TxnStateAborted
	// TxnStateCommitWait indicates the transaction has committed and is
	// waiting for the client to acknowledge.
	TxnStateCommitWait
)

// String returns a string representation of the transaction state.
func (s TxnState) String() string {
	switch s {
	case TxnStateNoTxn:
		return "NoTxn"
	case TxnStateOpen:
		return "Open"
	case TxnStateAborted:
		return "Aborted"
	case TxnStateCommitWait:
		return "CommitWait"
	default:
		return "Unknown"
	}
}

// TransactionManager manages transaction lifecycle for a connection executor.
// It handles starting, committing, and rolling back transactions, as well as
// savepoint management.
type TransactionManager struct {
	// txn is the current KV transaction, or nil if no transaction is active.
	txn *kv.Txn

	// state is the current transaction state.
	state TxnState

	// implicit indicates whether this is an implicit (auto-commit) transaction.
	implicit bool

	// isolationLevel is the isolation level for the current transaction.
	isolationLevel tree.IsolationLevel

	// priority is the priority for the current transaction.
	priority tree.UserPriority

	// readOnly indicates whether the transaction is read-only.
	readOnly bool

	// savepoints is the stack of active savepoints.
	savepoints []Savepoint
}

// Savepoint represents a savepoint within a transaction.
type Savepoint struct {
	// Name is the name of the savepoint.
	Name string

	// Active indicates whether the savepoint is still active.
	Active bool
}

// NewTransactionManager creates a new TransactionManager.
func NewTransactionManager() *TransactionManager {
	return &TransactionManager{
		state:          TxnStateNoTxn,
		isolationLevel: tree.SerializableIsolation,
		priority:       tree.UnspecifiedUserPriority,
		savepoints:     make([]Savepoint, 0),
	}
}

// Txn returns the underlying KV transaction.
func (t *TransactionManager) Txn() *kv.Txn {
	return t.txn
}

// State returns the current transaction state.
func (t *TransactionManager) State() TxnState {
	return t.state
}

// IsolationLevel returns the transaction's isolation level.
func (t *TransactionManager) IsolationLevel() tree.IsolationLevel {
	return t.isolationLevel
}

// ReadOnly returns true if the transaction is read-only.
func (t *TransactionManager) ReadOnly() bool {
	return t.readOnly
}

// IsImplicit returns true if this is an implicit transaction.
func (t *TransactionManager) IsImplicit() bool {
	return t.implicit
}

// Priority returns the transaction priority.
func (t *TransactionManager) Priority() tree.UserPriority {
	return t.priority
}

// InTransaction returns true if a transaction is currently active.
func (t *TransactionManager) InTransaction() bool {
	return t.state != TxnStateNoTxn
}

// Begin starts a new transaction with the given options.
// This method sets up the transaction state but does not create the KV
// transaction - that is handled by the connExecutor.
func (t *TransactionManager) Begin(
	ctx context.Context,
	txn *kv.Txn,
	implicit bool,
	isolationLevel tree.IsolationLevel,
	priority tree.UserPriority,
	readOnly bool,
) error {
	if t.state != TxnStateNoTxn {
		return errors.New("cannot begin transaction: transaction already in progress")
	}

	t.txn = txn
	t.state = TxnStateOpen
	t.implicit = implicit
	t.isolationLevel = isolationLevel
	t.priority = priority
	t.readOnly = readOnly
	t.savepoints = make([]Savepoint, 0)

	return nil
}

// Commit commits the current transaction.
// This method updates the transaction state but does not commit the KV
// transaction - that is handled by the connExecutor.
func (t *TransactionManager) Commit(ctx context.Context) error {
	if t.state != TxnStateOpen {
		return errors.Newf("cannot commit: transaction in %s state", t.state)
	}

	t.state = TxnStateCommitWait
	return nil
}

// FinishCommit finalizes the commit, transitioning to no transaction state.
func (t *TransactionManager) FinishCommit(ctx context.Context) {
	t.txn = nil
	t.state = TxnStateNoTxn
	t.savepoints = make([]Savepoint, 0)
}

// Rollback rolls back the current transaction.
// This method updates the transaction state but does not rollback the KV
// transaction - that is handled by the connExecutor.
func (t *TransactionManager) Rollback(ctx context.Context) error {
	if t.state == TxnStateNoTxn {
		return errors.New("cannot rollback: no transaction in progress")
	}

	t.txn = nil
	t.state = TxnStateNoTxn
	t.savepoints = make([]Savepoint, 0)

	return nil
}

// Abort marks the transaction as aborted.
func (t *TransactionManager) Abort(ctx context.Context) {
	if t.state == TxnStateOpen {
		t.state = TxnStateAborted
	}
}

// Savepoint creates a new savepoint with the given name.
func (t *TransactionManager) Savepoint(name string) error {
	if t.state != TxnStateOpen {
		return errors.Newf("cannot create savepoint: transaction in %s state", t.state)
	}

	t.savepoints = append(t.savepoints, Savepoint{
		Name:   name,
		Active: true,
	})

	return nil
}

// RollbackToSavepoint rolls back to the savepoint with the given name.
func (t *TransactionManager) RollbackToSavepoint(name string) error {
	if t.state != TxnStateOpen && t.state != TxnStateAborted {
		return errors.Newf("cannot rollback to savepoint: transaction in %s state", t.state)
	}

	// Find the savepoint
	found := false
	for i := len(t.savepoints) - 1; i >= 0; i-- {
		if t.savepoints[i].Name == name {
			// Deactivate all savepoints after this one
			t.savepoints = t.savepoints[:i+1]
			found = true

			// If we were aborted, rolling back to a savepoint allows
			// the transaction to continue.
			if t.state == TxnStateAborted {
				t.state = TxnStateOpen
			}
			break
		}
	}

	if !found {
		return errors.Newf("savepoint %q does not exist", name)
	}

	return nil
}

// ReleaseSavepoint releases the savepoint with the given name.
func (t *TransactionManager) ReleaseSavepoint(name string) error {
	if t.state != TxnStateOpen {
		return errors.Newf("cannot release savepoint: transaction in %s state", t.state)
	}

	// Find and remove the savepoint
	found := false
	for i := len(t.savepoints) - 1; i >= 0; i-- {
		if t.savepoints[i].Name == name {
			// Remove this savepoint and all after it
			t.savepoints = t.savepoints[:i]
			found = true
			break
		}
	}

	if !found {
		return errors.Newf("savepoint %q does not exist", name)
	}

	return nil
}

// SavepointCount returns the number of active savepoints.
func (t *TransactionManager) SavepointCount() int {
	return len(t.savepoints)
}

// HasSavepoint returns true if a savepoint with the given name exists.
func (t *TransactionManager) HasSavepoint(name string) bool {
	for _, sp := range t.savepoints {
		if sp.Name == name {
			return true
		}
	}
	return false
}

// SetPriority sets the transaction priority.
func (t *TransactionManager) SetPriority(priority tree.UserPriority) {
	t.priority = priority
}

// SetReadOnly sets whether the transaction is read-only.
func (t *TransactionManager) SetReadOnly(readOnly bool) {
	t.readOnly = readOnly
}

// SetIsolationLevel sets the transaction isolation level.
func (t *TransactionManager) SetIsolationLevel(level tree.IsolationLevel) {
	t.isolationLevel = level
}

// Reset resets the transaction manager to its initial state.
func (t *TransactionManager) Reset() {
	t.txn = nil
	t.state = TxnStateNoTxn
	t.implicit = false
	t.isolationLevel = tree.SerializableIsolation
	t.priority = tree.UnspecifiedUserPriority
	t.readOnly = false
	t.savepoints = make([]Savepoint, 0)
}

// KVPriority converts the SQL priority to a KV priority.
func (t *TransactionManager) KVPriority() roachpb.UserPriority {
	switch t.priority {
	case tree.Low:
		return roachpb.MinUserPriority
	case tree.Normal, tree.UnspecifiedUserPriority:
		return roachpb.NormalUserPriority
	case tree.High:
		return roachpb.MaxUserPriority
	default:
		return roachpb.NormalUserPriority
	}
}
