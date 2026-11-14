package coins

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
)

// Mode is COIN_BILLING_MODE: how far coin billing is on (same steps as
// PRO_GATE_MODE).
type Mode string

// Modes.
const (
	// ModeOff (default): nothing is read or written; the AI paths keep the
	// daily limits of today.
	ModeOff Mode = "off"
	// ModeLog: wallets exist (grants, purchases); a call's would-be cost is
	// only logged (coins.would_spend) and nothing is held or charged.
	ModeLog Mode = "log"
	// ModeEnforce: calls hold and charge coins; 402 coins_insufficient.
	ModeEnforce Mode = "enforce"
)

// Ops a hold is for (COIN_RESERVE_<OP>, COIN_ESTIMATE_<OP>).
const (
	OpGenerate = "generate"
	OpEdit     = "edit"
	OpChat     = "chat"
	OpText     = "text"
	// OpCall is a block of call minutes (#447, internal/calls). It has no
	// COIN_RESERVE_<OP>: the caller passes the block as ReserveRequest.Max.
	OpCall = "call"
	// OpExt is a paid external API call (internal/extapi). Like OpCall it has
	// no COIN_RESERVE_<OP>: extapi passes COIN_RESERVE_EXT_<PROVIDER> as
	// ReserveRequest.Max, and its spend rows are spend_ext.
	OpExt = "ext"
)

// Ops lists the ops in display order.
var Ops = []string{OpGenerate, OpEdit, OpChat, OpText}

// TextKinds are the heavier /v1/text calls that may hold their own amount
// (COIN_RESERVE_TEXT_<KIND>, like the rate limit weights); unset = the TEXT
// hold. The ledger op stays "text".
var TextKinds = []string{"task", "long", "image"}

// Allowance kinds (ALLOWANCE_PRO_<KIND>, COIN_OVERFLOW_*_<KIND>): what the
// runner counts per creator per JST month (part 3, overflow.go).
const (
	KindRuns       = "runs"
	KindRecipients = "recipients"
	KindWrites     = "writes"
	KindReads      = "reads"
)

// AllowanceKinds lists the allowance kinds in display order.
var AllowanceKinds = []string{KindRuns, KindRecipients, KindWrites, KindReads}

// Block is one overflow block of a kind: Size more units of the monthly
// allowance for Coins coins. A zero Size or Coins means the kind cannot be
// bought past the allowance.
type Block struct {
	Size  int64 `json:"size"`
	Coins int64 `json:"coins"`
}

// On reports whether the block can be bought.
func (b Block) On() bool { return b.Size > 0 && b.Coins > 0 }

// Pack is one consumable product and the coins it credits.
type Pack struct {
	ProductID string `json:"productId"`
	Coins     int64  `json:"coins"`
}

// Config is the coin settings. Every number that reveals a price comes from
// env set by the owner; the defaults are 0 (= not counted / not granted).
type Config struct {
	Mode Mode
	// PerUSD converts a call's est_cost_usd to coins (0 = calls cost nothing).
	PerUSD float64
	// MinPerCall is the least a counted call costs.
	MinPerCall int64
	// Reserve is the hold per op (the op's upper cost).
	Reserve map[string]int64
	// Estimates per op shown before a call ("about N coins").
	Estimates map[string]int64
	// Packs are the consumable products, in COIN_PACKS order.
	Packs []Pack
	// InitialGrant is credited once to each signed-in account.
	InitialGrant int64
	// ProMonthly is Tailor It Pro's coins per billing period (not carried over).
	ProMonthly int64
	// HoldTTL is how long an unsettled hold lives before it is returned.
	HoldTTL time.Duration
	// Allowance is Pro's monthly server allowance per kind (0 = not enforced;
	// still counted for display).
	Allowance map[string]int64
	// Overflow is the block bought per kind past the allowance (overflow.go).
	Overflow map[string]Block
	// SlotPrice is the coins of one app slot (slots.go; 0 = cannot be bought).
	SlotPrice int64
}

// Defaults that reveal nothing about prices.
const (
	DefaultMinPerCall = 1
	DefaultHoldTTL    = 10 * time.Minute
)

// ConfigFromEnv reads:
//
//	COIN_BILLING_MODE     off (default) | log | enforce; anything else is an error
//	COIN_PER_USD          coins per USD of est_cost_usd (default 0 = not counted)
//	COIN_MIN_PER_CALL     least coins a counted call costs (default 1)
//	COIN_RESERVE_<OP>     hold per op: GENERATE, EDIT, CHAT, TEXT (default 0)
//	COIN_RESERVE_TEXT_<KIND>  hold of a /v1/text kind: TASK, LONG (aiChat too), IMAGE (unset = TEXT's)
//	COIN_ESTIMATE_<OP>    "about N coins" shown per op (default 0 = not shown)
//	COIN_PACKS            productId=coins,... consumables (default none)
//	COIN_INITIAL_GRANT    one-time coins of a signed-in account (default 0)
//	COIN_PRO_MONTHLY      Pro's coins per billing period (default 0)
//	COIN_HOLD_TTL         unsettled hold lifetime, e.g. 10m (default 10m, ≥ 1m)
//	ALLOWANCE_PRO_<KIND>  Pro's monthly allowance: RUNS, RECIPIENTS, WRITES, READS (default 0)
//	COIN_OVERFLOW_BLOCK_<KIND>  units one overflow block adds (default 0 = no overflow)
//	COIN_OVERFLOW_PRICE_<KIND>  coins of one overflow block (default 0 = no overflow)
//	COIN_PRICE_APP_SLOT   coins of one more app to hold (default 0 = slots cannot be bought)
func ConfigFromEnv(getenv func(string) string) (Config, error) {
	c := Config{
		MinPerCall: DefaultMinPerCall,
		HoldTTL:    DefaultHoldTTL,
		Reserve:    map[string]int64{},
		Estimates:  map[string]int64{},
		Allowance:  map[string]int64{},
		Overflow:   map[string]Block{},
	}
	switch m := Mode(strings.ToLower(strings.TrimSpace(getenv("COIN_BILLING_MODE")))); m {
	case "":
		c.Mode = ModeOff
	case ModeOff, ModeLog, ModeEnforce:
		c.Mode = m
	default:
		return c, fmt.Errorf("COIN_BILLING_MODE must be off|log|enforce, got %q", getenv("COIN_BILLING_MODE"))
	}
	var err error
	if v := strings.TrimSpace(getenv("COIN_PER_USD")); v != "" {
		if c.PerUSD, err = strconv.ParseFloat(v, 64); err != nil || c.PerUSD < 0 || math.IsInf(c.PerUSD, 0) || math.IsNaN(c.PerUSD) {
			return c, fmt.Errorf("COIN_PER_USD must be a number ≥ 0, got %q", v)
		}
	}
	num := func(key string, dst *int64) {
		if err == nil {
			err = nonNegative(getenv, key, dst)
		}
	}
	num("COIN_MIN_PER_CALL", &c.MinPerCall)
	num("COIN_INITIAL_GRANT", &c.InitialGrant)
	num("COIN_PRO_MONTHLY", &c.ProMonthly)
	num("COIN_PRICE_APP_SLOT", &c.SlotPrice)
	for _, op := range Ops {
		var r, e int64
		num("COIN_RESERVE_"+strings.ToUpper(op), &r)
		num("COIN_ESTIMATE_"+strings.ToUpper(op), &e)
		c.Reserve[op], c.Estimates[op] = r, e
	}
	for _, k := range TextKinds {
		key := "COIN_RESERVE_TEXT_" + strings.ToUpper(k)
		if strings.TrimSpace(getenv(key)) == "" {
			continue
		}
		var r int64
		num(key, &r)
		c.Reserve[OpText+"_"+k] = r
	}
	for _, k := range AllowanceKinds {
		var a int64
		var b Block
		num("ALLOWANCE_PRO_"+strings.ToUpper(k), &a)
		num("COIN_OVERFLOW_BLOCK_"+strings.ToUpper(k), &b.Size)
		num("COIN_OVERFLOW_PRICE_"+strings.ToUpper(k), &b.Coins)
		c.Allowance[k], c.Overflow[k] = a, b
	}
	if err != nil {
		return c, err
	}
	if v := strings.TrimSpace(getenv("COIN_HOLD_TTL")); v != "" {
		if c.HoldTTL, err = time.ParseDuration(v); err != nil || c.HoldTTL < time.Minute {
			return c, fmt.Errorf("COIN_HOLD_TTL must be a duration ≥ 1m, got %q", v)
		}
	}
	if c.Packs, err = parsePacks(getenv("COIN_PACKS")); err != nil {
		return c, err
	}
	return c, nil
}

// CheckReserves is for the process that holds coins (the ai): under enforce
// a charge is capped at the hold, so an op whose COIN_RESERVE_<OP> is left
// at 0 would never be charged. It should refuse to start rather than run
// that op free. Other processes (appconfig, runner) never hold and do not
// need the reserves.
func (c Config) CheckReserves() error {
	if c.Mode != ModeEnforce {
		return nil
	}
	var missing []string
	for _, op := range Ops {
		if c.Reserve[op] <= 0 {
			missing = append(missing, "COIN_RESERVE_"+strings.ToUpper(op))
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("COIN_BILLING_MODE=enforce needs %s > 0", strings.Join(missing, ", "))
	}
	return nil
}

func nonNegative(getenv func(string) string, key string, dst *int64) error {
	v := strings.TrimSpace(getenv(key))
	if v == "" {
		return nil
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil || n < 0 {
		return fmt.Errorf("%s must be an integer ≥ 0, got %q", key, v)
	}
	*dst = n
	return nil
}

func parsePacks(raw string) ([]Pack, error) {
	var out []Pack
	seen := map[string]bool{}
	for _, item := range strings.Split(raw, ",") {
		if item = strings.TrimSpace(item); item == "" {
			continue
		}
		id, n, ok := strings.Cut(item, "=")
		id = strings.TrimSpace(id)
		coins, err := strconv.ParseInt(strings.TrimSpace(n), 10, 64)
		if !ok || id == "" || err != nil || coins <= 0 || seen[id] {
			return nil, fmt.Errorf("COIN_PACKS must be productId=coins,... (coins > 0, ids unique), got %q", item)
		}
		seen[id] = true
		out = append(out, Pack{ProductID: id, Coins: coins})
	}
	return out, nil
}

// PackCoins returns the coins of productID.
func (c Config) PackCoins(productID string) (int64, bool) {
	for _, p := range c.Packs {
		if p.ProductID == productID {
			return p.Coins, true
		}
	}
	return 0, false
}

// PackIDs are the consumable product ids (the verifier's allow list).
func (c Config) PackIDs() []string {
	out := make([]string, len(c.Packs))
	for i, p := range c.Packs {
		out[i] = p.ProductID
	}
	return out
}

// Price converts a call's cost to coins: ceil(usd × PerUSD), at least
// MinPerCall. PerUSD = 0 means calls are not counted (0).
func (c Config) Price(usd float64) int64 {
	if c.PerUSD <= 0 {
		return 0
	}
	n := int64(math.Ceil(max(0, usd) * c.PerUSD))
	return max(n, c.MinPerCall)
}
