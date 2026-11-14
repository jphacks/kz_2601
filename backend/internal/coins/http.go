package coins

import (
	"errors"
	"net/http"

	"github.com/jphacks/kz_2601/backend/internal/httpx"
)

// Response headers of a charged call (part 2; iOS shows the balance).
const (
	HeaderCharged = "X-Coins-Charged"
	HeaderBalance = "X-Coins-Balance"
)

// ErrAllowanceExhausted is 402 allowance_exhausted: the payer's server
// allowance is used up and overflow paid with coins is off (part 3).
var ErrAllowanceExhausted = errors.New("server allowance exhausted")

// OverflowCapError is 402 overflow_cap_reached: overflow paid with coins
// reached the payer's monthly cap (part 3).
type OverflowCapError struct {
	Cap   int64
	Spent int64
}

func (e *OverflowCapError) Error() string { return "overflow cap reached" }

// WriteError writes the 402 of a coin error and reports whether err was one.
// Callers return these only in enforce mode (Reserve never fails with them
// in off / log).
func WriteError(w http.ResponseWriter, err error) bool {
	var ins *InsufficientError
	var capErr *OverflowCapError
	switch {
	case errors.As(err, &ins):
		payer := ins.Payer
		if payer == "" {
			payer = "self"
		}
		httpx.ErrorWithDetails(w, http.StatusPaymentRequired, httpx.CodeCoinsInsufficient, "not enough coins",
			map[string]any{"required": ins.Required, "balance": ins.Balance, "payer": payer})
	case errors.Is(err, ErrAllowanceExhausted):
		httpx.Error(w, http.StatusPaymentRequired, httpx.CodeAllowanceExhausted, "the monthly server allowance is used up")
	case errors.As(err, &capErr):
		httpx.ErrorWithDetails(w, http.StatusPaymentRequired, httpx.CodeOverflowCapReached, "the monthly coin cap for overflow is reached",
			map[string]any{"cap": capErr.Cap, "spent": capErr.Spent})
	default:
		return false
	}
	return true
}
