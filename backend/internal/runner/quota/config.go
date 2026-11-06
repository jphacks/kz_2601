package quota

import (
	"fmt"
	"strconv"
)

// Limits are the daily counters (units per JST day) and the per-run shape
// caps. 0 disables that counter or cap. Values: docs/decisions/338 「上限(案)」.
type Limits struct {
	AppExecFree, AppExecPro         int // executions per app
	CreatorExecFree, CreatorExecPro int // executions per creator, all apps
	AppWritesFree, AppWritesPro     int // records written per app
	AppNotifyFree, AppNotifyPro     int // notification recipients per app
	RecipientPerApp                 int // notifications one person gets from one app
	RecipientTotal                  int // notifications one person gets from all apps
	ActorPerApp                     int // executions one actor triggers in one app
	Global                          int // executions in the environment (cost breaker)

	RunRecipientsFree, RunRecipientsPro   int // recipients of one run
	RunWritesFree, RunWritesPro           int // writes of one run
	ScheduledRulesFree, ScheduledRulesPro int // scheduled rules of one app
}

// DefaultLimits is the starting table of #338 (to be tuned from runner.usage).
var DefaultLimits = Limits{
	AppExecFree: 100, AppExecPro: 2000,
	CreatorExecFree: 300, CreatorExecPro: 3000,
	AppWritesFree: 100, AppWritesPro: 2000,
	AppNotifyFree: 50, AppNotifyPro: 1000,
	RecipientPerApp: 20,
	RecipientTotal:  100,
	ActorPerApp:     200,
	Global:          10000,

	RunRecipientsFree: 1, RunRecipientsPro: 200,
	RunWritesFree: 1, RunWritesPro: 10,
	ScheduledRulesFree: 1, ScheduledRulesPro: 5,
}

// ConfigFromEnv reads RUNNER_LIMIT_* and RUNNER_GLOBAL_DAILY (unset =
// DefaultLimits; 0 disables) and APP_ENV, like ratelimit.ConfigFromEnv.
func ConfigFromEnv(getenv func(string) string) (Config, error) {
	cfg := Config{Limits: DefaultLimits, Env: getenv("APP_ENV")}
	l := &cfg.Limits
	fields := []struct {
		key string
		dst *int
	}{
		{"RUNNER_LIMIT_APP_EXEC_FREE", &l.AppExecFree},
		{"RUNNER_LIMIT_APP_EXEC_PRO", &l.AppExecPro},
		{"RUNNER_LIMIT_CREATOR_EXEC_FREE", &l.CreatorExecFree},
		{"RUNNER_LIMIT_CREATOR_EXEC_PRO", &l.CreatorExecPro},
		{"RUNNER_LIMIT_APP_WRITES_FREE", &l.AppWritesFree},
		{"RUNNER_LIMIT_APP_WRITES_PRO", &l.AppWritesPro},
		{"RUNNER_LIMIT_APP_NOTIFY_FREE", &l.AppNotifyFree},
		{"RUNNER_LIMIT_APP_NOTIFY_PRO", &l.AppNotifyPro},
		{"RUNNER_LIMIT_RECIPIENT_PER_APP", &l.RecipientPerApp},
		{"RUNNER_LIMIT_RECIPIENT_TOTAL", &l.RecipientTotal},
		{"RUNNER_LIMIT_ACTOR_PER_APP", &l.ActorPerApp},
		{"RUNNER_GLOBAL_DAILY", &l.Global},
		{"RUNNER_LIMIT_RUN_RECIPIENTS_FREE", &l.RunRecipientsFree},
		{"RUNNER_LIMIT_RUN_RECIPIENTS_PRO", &l.RunRecipientsPro},
		{"RUNNER_LIMIT_RUN_WRITES_FREE", &l.RunWritesFree},
		{"RUNNER_LIMIT_RUN_WRITES_PRO", &l.RunWritesPro},
		{"RUNNER_LIMIT_SCHEDULED_RULES_FREE", &l.ScheduledRulesFree},
		{"RUNNER_LIMIT_SCHEDULED_RULES_PRO", &l.ScheduledRulesPro},
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

// pick returns the free or pro value of a pair.
func pick(p Plan, free, pro int) int {
	if p == PlanPro {
		return pro
	}
	return free
}
