package ratelimit

import (
	"fmt"
	"strconv"
)

// ConfigFromEnv reads RATE_LIMIT_* (unset = defaults; 0 disables a counter):
//
//	RATE_LIMIT_ANON_DAILY      units/day per anonymous uid      (30)
//	RATE_LIMIT_USER_DAILY      units/day per signed-in uid      (60)
//	RATE_LIMIT_PRO_DAILY       units/day per Pro uid            (300)
//	RATE_LIMIT_IP_DAILY        units/day per IP, anonymous only (300)
//	RATE_LIMIT_GLOBAL_DAILY    units/day for the environment    (2000)
//	RATE_LIMIT_WEIGHT_CREATE   generate, new app                (3)
//	RATE_LIMIT_WEIGHT_EDIT     generate, edit                   (2)
//	RATE_LIMIT_WEIGHT_CHAT     chat turn                        (1)
//	RATE_LIMIT_WEIGHT_TEXT     aiButton generate (/v1/text)     (1)
func ConfigFromEnv(getenv func(string) string) (Config, error) {
	cfg := Config{Limits: DefaultLimits, Weights: DefaultWeights, Env: getenv("APP_ENV")}
	fields := []struct {
		key string
		dst *int
	}{
		{"RATE_LIMIT_ANON_DAILY", &cfg.Limits.Anonymous},
		{"RATE_LIMIT_USER_DAILY", &cfg.Limits.SignedIn},
		{"RATE_LIMIT_PRO_DAILY", &cfg.Limits.Pro},
		{"RATE_LIMIT_IP_DAILY", &cfg.Limits.PerIP},
		{"RATE_LIMIT_GLOBAL_DAILY", &cfg.Limits.Global},
		{"RATE_LIMIT_WEIGHT_CREATE", &cfg.Weights.Create},
		{"RATE_LIMIT_WEIGHT_EDIT", &cfg.Weights.Edit},
		{"RATE_LIMIT_WEIGHT_CHAT", &cfg.Weights.Chat},
		{"RATE_LIMIT_WEIGHT_TEXT", &cfg.Weights.Text},
	}
	for _, f := range fields {
		raw := getenv(f.key)
		if raw == "" {
			continue
		}
		n, err := strconv.Atoi(raw)
		if err != nil || n < 0 {
			return cfg, fmt.Errorf("%s must be a non-negative integer, got %q", f.key, raw)
		}
		*f.dst = n
	}
	return cfg, nil
}
