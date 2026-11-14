package store

import (
	"context"
	"errors"
	"math"
	"reflect"
)

// Unique records (#273): entity.unique names fields whose values no two
// records of the entity may share. appconfig hashes the values of a record
// into UniqueClaim.Key; the store reserves the key in the same transaction as
// the write:
//
//	{records collection}/../unique/{key}   # {recordId}
//
// next to the records collection (entities/{e}/unique). A record keeps its
// key in uniqueKey so an update or delete releases it. Records written before
// the entity had unique (or by paths that do not claim) hold no key, so a
// claim also looks for a record with the same values (UniqueClaim.Match).
// A lock whose record is gone or holds another key is stale and is taken over.

// UniqueAuthorKey is the UniqueClaim.Match key of the record's author ($author).
const UniqueAuthorKey = "$author"

// ErrDuplicate: another record already has the claimed values.
var ErrDuplicate = errors.New("duplicate record")

// UniqueClaim is the combination of values a write reserves. The zero value
// (Key "") reserves nothing: a record with an empty unique field is not compared.
type UniqueClaim struct {
	Key string
	// Match is field id (or UniqueAuthorKey) -> value, all equal on a duplicate.
	Match map[string]any
}

// UniqueRecords are record writes that keep entity.unique.
type UniqueRecords interface {
	// CreateRecordUnique is CreateRecordEmit (emit may be nil) that fails with
	// ErrDuplicate when another record holds claim. Idempotent on rec.ID like
	// CreateRecord (an existing record is returned without checking).
	CreateRecordUnique(ctx context.Context, rs RecordSet, rec Record, claim UniqueClaim, emit EmitFunc) (*Record, bool, error)
	// UpdateRecordUnique is UpdateRecordEmit that moves the record's claim to
	// claim (releasing the old one) and fails with ErrDuplicate when another
	// record holds the new one.
	UpdateRecordUnique(ctx context.Context, rs RecordSet, id string, data map[string]any, claim UniqueClaim, emit EmitFunc) (*Record, error)
}

// matches reports whether r has every value of m (r is not id).
func (m UniqueClaim) matches(r *Record) bool {
	if len(m.Match) == 0 {
		return false
	}
	for k, want := range m.Match {
		var got any
		if k == UniqueAuthorKey {
			got = r.Author
		} else {
			got = r.Data[k]
		}
		if !sameUniqueValue(got, want) {
			return false
		}
	}
	return true
}

// sameUniqueValue compares as Firestore does: numbers by value (1 == 1.0).
func sameUniqueValue(a, b any) bool {
	if x, ok := uniqueNumber(a); ok {
		y, ok := uniqueNumber(b)
		return ok && x == y
	}
	return reflect.DeepEqual(a, b)
}

func uniqueNumber(v any) (float64, bool) {
	switch n := v.(type) {
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	case float64:
		return n, !math.IsNaN(n)
	}
	return 0, false
}
