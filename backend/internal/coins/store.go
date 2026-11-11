package coins

import (
	"context"
	"time"
)

// Keys are the documents one Store.Update reads and may write: always the
// wallet of UID, plus the hold HoldID and the purchase TransactionID when set.
type Keys struct {
	UID           string
	HoldID        string
	TransactionID string
}

// Tx is the state inside one atomic update. fn changes the fields and marks
// what to write; Entries are added to UID's ledger. Wallet is the zero
// Wallet when the document does not exist; Hold / Purchase are nil when
// absent (or not asked for).
type Tx struct {
	Wallet        Wallet
	WalletDirty   bool
	Hold          *Hold
	HoldDirty     bool
	Purchase      *Purchase
	PurchaseDirty bool
	Entries       []Entry
	// Claims reads the initial-grant claims of keys in this transaction
	// (absent keys are missing from the map). nil = the Store has none.
	Claims func(keys []string) (map[string]InitialClaim, error)
	// NewClaims are written with the rest.
	NewClaims map[string]InitialClaim
}

// HoldRef points at a hold (Sweep).
type HoldRef struct {
	UID string
	ID  string
}

// Store persists wallets, ledgers, holds and purchases. The operations
// (ops.go) are written once against Update, so Memory and Firestore behave
// the same.
type Store interface {
	// Update reads k's documents, runs fn and writes what fn marked, all in
	// one transaction (Firestore may run fn again on contention). An error
	// from fn aborts without writing and is returned as is.
	Update(ctx context.Context, k Keys, fn func(*Tx) error) error
	// Expired lists held holds whose expireAt is at or before now (oldest
	// first, at most limit).
	Expired(ctx context.Context, now time.Time, limit int) ([]HoldRef, error)
	// Ledger lists uid's entries, newest first, after cursor ("" = from the
	// newest). next is "" at the end.
	Ledger(ctx context.Context, uid string, limit int, cursor string) (entries []Entry, next string, err error)
	// PurgeUser deletes uid's wallet, ledger and holds (account deletion).
	// coin_transactions are kept for refund matching (decision #7 of the
	// design), and coin_initial_grants so the initial grant stays once per person.
	PurgeUser(ctx context.Context, uid string) (int, error)
}
