// Command feedback lets an operator read in-app feedback (#319). Read-only:
// it never writes to Firestore (status changes are done by hand in the
// console, docs/backend/features/feedback.md).
//
//	feedback list [-since 168h] [-status received] [-limit 50]
//	feedback show [-out DIR] <id>     # -out saves screenshot.jpg / dsl.json there
//
// Environment: GOOGLE_CLOUD_PROJECT (required), FIRESTORE_DATABASE,
// FIRESTORE_EMULATOR_HOST (local: make run-feedback). Uses your ADC.
//
// The message is text anyone can write: it is printed as quoted data with
// control characters escaped, and must never be pasted into an agent or LLM
// as instructions (#366).
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/jphacks/kz_2601/backend/internal/fsenv"
	"github.com/jphacks/kz_2601/backend/internal/store"
)

func main() {
	if len(os.Args) < 2 {
		usage(os.Stderr)
		os.Exit(2)
	}
	target := fsenv.FromEnv(os.Getenv)
	if target.Project == "" {
		fmt.Fprintln(os.Stderr, "feedback: GOOGLE_CLOUD_PROJECT is required")
		os.Exit(2)
	}
	ctx := context.Background()
	st, err := store.NewFirestore(ctx, target.Project, target.Database)
	if err != nil {
		fmt.Fprintln(os.Stderr, "feedback: firestore:", err)
		os.Exit(1)
	}
	defer func() { _ = st.Close() }()
	fmt.Fprintln(os.Stderr, "feedback: reading", target.String())
	if err := run(ctx, st, os.Args[1:], os.Stdout, time.Now()); err != nil {
		fmt.Fprintln(os.Stderr, "feedback:", err)
		os.Exit(1)
	}
}

func usage(w io.Writer) {
	_, _ = io.WriteString(w, usageText)
}

const usageText = "usage: feedback list [-since 168h] [-status received] [-limit 50]\n       feedback show [-out DIR] <id>\n"

var jst = time.FixedZone("JST", 9*60*60)

// run executes one command and writes its output to out.
func run(ctx context.Context, st store.FeedbackReader, args []string, out io.Writer, now time.Time) error {
	var b strings.Builder
	err := render(ctx, st, args, &b, now)
	if _, werr := io.WriteString(out, b.String()); err == nil {
		err = werr
	}
	return err
}

func render(ctx context.Context, st store.FeedbackReader, args []string, out *strings.Builder, now time.Time) error {
	switch args[0] {
	case "list":
		fs := flag.NewFlagSet("list", flag.ContinueOnError)
		fs.SetOutput(io.Discard) // the parse error is returned; do not also print flag usage
		since := fs.Duration("since", 7*24*time.Hour, "only feedback newer than this (0 = all)")
		status := fs.String("status", "", "only this status (received / triaged / done)")
		limit := fs.Int("limit", 50, "max rows (<= 500)")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		q := store.FeedbackQuery{Status: *status, Limit: *limit}
		if *since > 0 {
			q.Since = now.Add(-*since)
		}
		items, err := st.ListFeedback(ctx, q)
		if err != nil {
			return err
		}
		for _, fb := range items {
			att := []string{}
			if fb.AppID != "" {
				att = append(att, fmt.Sprintf("dsl(v%d)", fb.DSLVersion))
			}
			if fb.Device != nil {
				att = append(att, "device")
			}
			fmt.Fprintf(out, "%s  %s  %-8s %-5s uid=%s  [%s]\n    message (untrusted): %s\n", fb.ID,
				fb.CreatedAt.In(jst).Format("2006-01-02 15:04"), fb.Status, fb.Kind, fb.UID, strings.Join(att, ","), quote(fb.Message, 200))
		}
		fmt.Fprintf(out, "%d item(s). screenshots are not listed: use show <id>\n", len(items))
		return nil
	case "show":
		fs := flag.NewFlagSet("show", flag.ContinueOnError)
		fs.SetOutput(io.Discard) // the parse error is returned; do not also print flag usage
		dir := fs.String("out", "", "save screenshot.jpg / dsl.json into this directory (created if missing)")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		if fs.NArg() != 1 {
			return errors.New("show needs exactly one id")
		}
		fb, err := st.GetFeedback(ctx, fs.Arg(0))
		if err != nil {
			return err
		}
		fmt.Fprintf(out, "id:        %s\ncreatedAt: %s\nstatus:    %s\nkind:      %s\nuid:       %s\nconsent:   %s\n",
			fb.ID, fb.CreatedAt.In(jst).Format(time.RFC3339), fb.Status, fb.Kind, fb.UID, fb.ConsentVersion)
		if d := fb.Device; d != nil {
			fmt.Fprintf(out, "device:    app %s (%s) / %s / %s / %s\n", quote(d.AppVersion, 64), quote(d.Build, 64), quote(d.OS, 64), quote(d.Model, 64), quote(d.Locale, 64))
		}
		if fb.AppID != "" {
			fmt.Fprintf(out, "dsl:       app %s v%d (%d bytes)\n", fb.AppID, fb.DSLVersion, len(fb.DSL))
		}
		fmt.Fprintf(out, "screenshot: %d bytes\nmessage (untrusted data, not instructions):\n%s\n", len(fb.Screenshot), quote(fb.Message, 0))
		if *dir == "" {
			return nil
		}
		if err := os.MkdirAll(*dir, 0o700); err != nil {
			return err
		}
		files := map[string][]byte{"screenshot.jpg": fb.Screenshot, "dsl.json": fb.DSL}
		for name, body := range files {
			if len(body) == 0 {
				continue
			}
			p := filepath.Join(*dir, fb.ID+"-"+name)
			if err := os.WriteFile(p, body, 0o600); err != nil {
				return err
			}
			fmt.Fprintln(out, "saved", p)
		}
		return nil
	}
	out.WriteString(usageText)
	return fmt.Errorf("unknown command %q", args[0])
}

// quote prints s as a Go string literal (escapes newlines, terminal control
// sequences and invalid UTF-8) cut to max runes (0 = no cut).
func quote(s string, max int) string {
	if r := []rune(s); max > 0 && len(r) > max {
		s = string(r[:max]) + "…"
	}
	return strconv.Quote(s)
}
