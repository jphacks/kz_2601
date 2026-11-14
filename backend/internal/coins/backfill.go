package coins

import (
	"context"
	"errors"
	"time"
)

var errNoClaims = errors.New("coins: the store keeps no initial-grant claims")

// The one-time backfill of coin_initial_grants (cmd/coin-initial-backfill):
// accounts that had the initial grant before the claims were written get
// the claims of their sign-in subjects, as claimInitial would have written.

// BackfillOutcome is what the backfill does with one account.
type BackfillOutcome int

const (
	BackfillNoSubject  BackfillOutcome = iota // no Apple / Google sign-in: nothing to key
	BackfillNotGranted                        // no wallet, or the initial grant not given
	BackfillRecorded                          // every subject is claimed by this account
	BackfillOtherOwner                        // a subject is claimed by another account: left as is
	BackfillWrite                             // claims to write
)

// backfillClaims decides the claims of one account: granted is
// Wallet.InitialGranted, keys its InitialKey values, have the existing
// claims of keys. Like claimInitial, one key of another account means none
// are written.
func backfillClaims(granted bool, owner string, keys []string, have map[string]InitialClaim) (BackfillOutcome, []string) {
	switch {
	case len(keys) == 0:
		return BackfillNoSubject, nil
	case !granted:
		return BackfillNotGranted, nil
	}
	var missing []string
	for _, k := range keys {
		c, ok := have[k]
		switch {
		case ok && c.Owner != owner:
			return BackfillOtherOwner, nil
		case !ok:
			missing = append(missing, k)
		}
	}
	if len(missing) == 0 {
		return BackfillRecorded, nil
	}
	return BackfillWrite, missing
}

// BackfillInitial records the initial-grant claims of uid (subjects are its
// auth.Identity.Subjects) in one Store.Update, and returns the outcome and
// how many claims it wrote (apply) or would write. It never changes the
// wallet, and a second run finds the claims already there.
func BackfillInitial(ctx context.Context, st Store, uid string, subjects []string, apply bool, now time.Time) (BackfillOutcome, int, error) {
	keys := make([]string, 0, len(subjects))
	for _, s := range subjects {
		keys = append(keys, InitialKey(s))
	}
	if len(keys) == 0 {
		return BackfillNoSubject, 0, nil
	}
	now = storedTime(now)
	owner := OwnerHash(uid)
	var out BackfillOutcome
	var n int
	err := st.Update(ctx, Keys{UID: uid}, func(tx *Tx) error {
		out, n = BackfillNotGranted, 0
		if !tx.Wallet.InitialGranted {
			return nil
		}
		if tx.Claims == nil {
			return errNoClaims
		}
		have, err := tx.Claims(keys)
		if err != nil {
			return err
		}
		var missing []string
		out, missing = backfillClaims(true, owner, keys, have)
		n = len(missing)
		if !apply || out != BackfillWrite {
			return nil
		}
		tx.NewClaims = map[string]InitialClaim{}
		for _, k := range missing {
			tx.NewClaims[k] = InitialClaim{Owner: owner, At: now}
		}
		return nil
	})
	return out, n, err
}
