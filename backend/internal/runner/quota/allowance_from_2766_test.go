package quota

import (
	"reflect"
	"testing"

	"github.com/jphacks/kz_2601/backend/internal/coins"
)

func coinsConfig2766(mode coins.Mode) coins.Config {
	return coins.Config{
		Mode:      mode,
		Allowance: map[string]int64{coins.KindRuns: 100, coins.KindWrites: 0},
		Overflow:  map[string]coins.Block{coins.KindRuns: {Size: 50, Coins: 30}},
	}
}

func TestAllowanceFromCopiesModeLimitsAndBlocks(t *testing.T) {
	c := coinsConfig2766(coins.ModeEnforce)
	got := AllowanceFrom(c)
	if got.Mode != coins.ModeEnforce {
		t.Errorf("Mode = %q, want %q", got.Mode, coins.ModeEnforce)
	}
	if !reflect.DeepEqual(got.Limits, c.Allowance) {
		t.Errorf("Limits = %#v, want %#v", got.Limits, c.Allowance)
	}
	if !reflect.DeepEqual(got.Blocks, c.Overflow) {
		t.Errorf("Blocks = %#v, want %#v", got.Blocks, c.Overflow)
	}
}

func TestAllowanceFromKeepsEmptySettingsEmpty(t *testing.T) {
	got := AllowanceFrom(coins.Config{})
	if got.Mode != "" {
		t.Errorf("Mode = %q, want 空", got.Mode)
	}
	if got.Limits != nil {
		t.Errorf("Limits = %#v, want nil", got.Limits)
	}
	if got.Blocks != nil {
		t.Errorf("Blocks = %#v, want nil", got.Blocks)
	}
}

func TestAllowanceFromOnFollowsMode(t *testing.T) {
	for _, c := range []struct {
		mode coins.Mode
		want bool
	}{
		{coins.ModeOff, false},
		{coins.ModeLog, true},
		{coins.ModeEnforce, true},
		{coins.Mode(""), false},
	} {
		if got := AllowanceFrom(coinsConfig2766(c.mode)).on(); got != c.want {
			t.Errorf("mode %q: on() = %v, want %v", c.mode, got, c.want)
		}
	}
}
