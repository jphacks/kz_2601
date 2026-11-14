package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"regexp"
	"time"

	"github.com/jphacks/kz_2601/backend/internal/store"
)

// defaultNotifyPause is pause-notify's -for (the automatic pause's length).
const defaultNotifyPause = 72 * time.Hour

// maxNotifyPause bounds -for: a longer stop is stop-app.
const maxNotifyPause = 30 * 24 * time.Hour

var (
	envRe = regexp.MustCompile(`^[a-z0-9-]{1,32}$`)
	// appconfig's idRe (appIds and rule ids)
	idRe = regexp.MustCompile(`^[A-Za-z0-9_.:-]{1,128}$`)
)

var runnerCommands = map[string]bool{
	"stops": true, "stop-log": true, "stop-app": true, "resume-app": true, "stop-all": true, "resume-all": true,
	"stop-rule": true, "resume-rule": true, "pause-notify": true, "resume-notify": true,
}

type stopArgs struct {
	env      string
	reason   string
	pauseFor time.Duration
	forSet   bool // -for was given (stop-app / stop-all: a pause that ends by itself)
	force    bool
	operator string
	limit    int
	yes      bool
}

// now is the clock of the stop commands (tests set it).
var now = func() time.Time { return store.Now() }

// runStops runs the runner stop commands (#346). Every write is a dry run
// unless -yes; the stop document is the record of the operation (who: the
// operator; when: createdAt / liftedAt; why: reason).
func runStops(ctx context.Context, st store.RunnerStops, cmd string, pos []string, a stopArgs, out io.Writer) error {
	ids := func(n int) error {
		if len(pos) != n {
			return errUsage
		}
		for _, p := range pos {
			if !idRe.MatchString(p) {
				return fmt.Errorf("%w: bad id %q", errUsage, p)
			}
		}
		return nil
	}
	write := func(desc string, fn func() error) error {
		if !a.yes {
			_, _ = fmt.Fprintf(out, "dry run: would %s in runner/%s (add -yes)\n", desc, a.env)
			return nil
		}
		if err := fn(); err != nil {
			return fmt.Errorf("%s: %w", desc, err)
		}
		_, _ = fmt.Fprintf(out, "done: %s in runner/%s\n", desc, a.env)
		return nil
	}
	needReason := func() error {
		if a.reason == "" {
			return errors.New("-reason is required (it is kept with the stop and shown to the creator)")
		}
		return nil
	}
	needOperator := func() error {
		if a.operator == "" {
			return errors.New("-operator is required (it is kept in the stop log)")
		}
		return nil
	}
	put := func(desc string, s store.Stop) error {
		if err := needReason(); err != nil {
			return err
		}
		if err := needOperator(); err != nil {
			return err
		}
		s.By, s.Reason, s.Operator, s.CreatedAt = store.StopByOperator, a.reason, a.operator, now()
		return write(fmt.Sprintf("%s (reason %q)", desc, a.reason), func() error { return st.PutStop(ctx, a.env, s) })
	}
	// hardStop is stop-app / stop-all: -for makes it a pause, -force also
	// refuses the rule-triggering writes in appconfig.
	hardStop := func(desc string, s store.Stop) error {
		if a.forSet {
			if a.pauseFor < time.Minute || a.pauseFor > maxNotifyPause {
				return fmt.Errorf("-for must be between 1m and %s (leave it out to stop until resumed)", maxNotifyPause)
			}
			s.Until = now().Add(a.pauseFor)
			desc += " for " + a.pauseFor.String()
		}
		if a.force {
			s.Force = true
			desc += ", refusing the writes that trigger its rules"
		}
		return put(desc, s)
	}
	lift := func(desc, id string) error {
		if err := needOperator(); err != nil {
			return err
		}
		return write(desc, func() error {
			ok, err := st.LiftStop(ctx, a.env, id, now(), a.operator)
			if err == nil && !ok {
				err = errors.New("no active stop")
			}
			return err
		})
	}

	switch cmd {
	case "stops":
		if len(pos) != 0 {
			return errUsage
		}
		all, err := st.ListStops(ctx, a.env)
		if err != nil {
			return err
		}
		t := now()
		for _, s := range all {
			state := "active"
			if !s.Active(t) {
				state = "ended"
			}
			until := "-"
			if !s.Until.IsZero() {
				until = s.Until.Format(time.RFC3339)
			}
			force := ""
			if s.Force {
				force = "\tforce"
			}
			_, _ = fmt.Fprintf(out, "%s\t%s\t%s\t%s\t%s\t%s\tuntil=%s\t%s%s\n", s.CreatedAt.Format(time.RFC3339), state, s.Level, s.AppID, s.RuleID, s.By, until, s.Reason, force)
		}
		return nil
	case "stop-log":
		if len(pos) > 1 {
			return errUsage
		}
		app := ""
		if len(pos) == 1 {
			if err := ids(1); err != nil {
				return err
			}
			app = pos[0]
		}
		log, err := st.StopLog(ctx, a.env, app, a.limit)
		if err != nil {
			return err
		}
		for _, e := range log {
			until := "-"
			if !e.Until.IsZero() {
				until = e.Until.Format(time.RFC3339)
			}
			_, _ = fmt.Fprintf(out, "%s\t%s\t%s\t%s\t%s\t%s\toperator=%s\tforce=%t\tuntil=%s\t%s\n", e.At.Format(time.RFC3339), e.Action, e.Level, e.AppID, e.RuleID, e.By, e.Operator, e.Force, until, e.Reason)
		}
		return nil
	case "stop-app":
		if err := ids(1); err != nil {
			return err
		}
		return hardStop("stop app "+pos[0], store.Stop{ID: store.AppStopID(pos[0]), Level: store.StopApp, AppID: pos[0]})
	case "resume-app":
		if err := ids(1); err != nil {
			return err
		}
		return lift("resume app "+pos[0], store.AppStopID(pos[0]))
	case "stop-all":
		if err := ids(0); err != nil {
			return err
		}
		return hardStop("stop every app", store.Stop{ID: store.GlobalStopID, Level: store.StopGlobal})
	case "resume-all":
		if err := ids(0); err != nil {
			return err
		}
		return lift("resume every app", store.GlobalStopID)
	case "stop-rule":
		if err := ids(2); err != nil {
			return err
		}
		return put(fmt.Sprintf("stop rule %s of app %s", pos[1], pos[0]),
			store.Stop{ID: store.RuleStopID(pos[0], pos[1]), Level: store.StopRule, AppID: pos[0], RuleID: pos[1]})
	case "resume-rule":
		if err := ids(2); err != nil {
			return err
		}
		return lift(fmt.Sprintf("resume rule %s of app %s", pos[1], pos[0]), store.RuleStopID(pos[0], pos[1]))
	case "pause-notify":
		if err := ids(1); err != nil {
			return err
		}
		if a.pauseFor < time.Minute || a.pauseFor > maxNotifyPause {
			return fmt.Errorf("-for must be between 1m and %s (a longer stop is stop-app)", maxNotifyPause)
		}
		return put(fmt.Sprintf("pause the notifications of app %s for %s", pos[0], a.pauseFor),
			store.Stop{ID: store.NotifyStopID(pos[0]), Level: store.StopNotify, AppID: pos[0], Until: now().Add(a.pauseFor)})
	case "resume-notify":
		if err := ids(1); err != nil {
			return err
		}
		return lift("resume the notifications of app "+pos[0], store.NotifyStopID(pos[0]))
	}
	return errUsage
}
