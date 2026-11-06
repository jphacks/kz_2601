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
	"stops": true, "stop-app": true, "resume-app": true, "stop-rule": true, "resume-rule": true,
	"pause-notify": true, "resume-notify": true,
}

type stopArgs struct {
	env      string
	reason   string
	pauseFor time.Duration
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
	put := func(desc string, s store.Stop) error {
		if err := needReason(); err != nil {
			return err
		}
		s.By, s.Reason, s.CreatedAt = store.StopByOperator, a.reason, now()
		return write(fmt.Sprintf("%s (reason %q)", desc, a.reason), func() error { return st.PutStop(ctx, a.env, s) })
	}
	lift := func(desc, id string) error {
		return write(desc, func() error {
			ok, err := st.LiftStop(ctx, a.env, id, now())
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
			_, _ = fmt.Fprintf(out, "%s\t%s\t%s\t%s\t%s\t%s\tuntil=%s\t%s\n", s.CreatedAt.Format(time.RFC3339), state, s.Level, s.AppID, s.RuleID, s.By, until, s.Reason)
		}
		return nil
	case "stop-app":
		if err := ids(1); err != nil {
			return err
		}
		return put("stop app "+pos[0], store.Stop{ID: store.AppStopID(pos[0]), Level: store.StopApp, AppID: pos[0]})
	case "resume-app":
		if err := ids(1); err != nil {
			return err
		}
		return lift("resume app "+pos[0], store.AppStopID(pos[0]))
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
