package extapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

// Google Tasks (#1677): free (one `runs` a call), OAuth with PKCE.
//
//	lists(limit?)                              tasks.readonly
//	tasks(listId?, due?, status?, limit?)      tasks.readonly
//	addTask(title, notes?, due?, listId?)      tasks
//	complete(taskId, listId?)                  tasks
//
// Google has no scope narrower than tasks for a write. Both scopes are
// sensitive: Google's verification is a human step. The OAuth client is
// Google Calendar's unless EXT_GOOGLE_TASKS_CLIENT_ID / _CLIENT_SECRET are
// set (ClientFrom: one consent screen).
//
// Env: EXT_GOOGLE_TASKS_ENABLED=1. Off by default; not in the review build
// (NeedsOAuth).
const GoogleTasks = "googleTasks"

const (
	gtasksScopeRead  = "https://www.googleapis.com/auth/tasks.readonly"
	gtasksScopeWrite = "https://www.googleapis.com/auth/tasks"
)

var googleTasksWire = gcalWire{
	AuthURL:   "https://accounts.google.com/o/oauth2/v2/auth",
	TokenURL:  "https://oauth2.googleapis.com/token",
	RevokeURL: "https://oauth2.googleapis.com/revoke",
	APIBase:   "https://tasks.googleapis.com/tasks/v1",
}

var googleTasksHosts = []string{"https://oauth2.googleapis.com", "https://tasks.googleapis.com"}

func init() {
	register(Provider{ID: GoogleTasks, Billing: BillingFree, NeedsOAuth: true, NameJa: "Google ToDo リスト", NameEn: "Google Tasks",
		Ops: []OpSpec{
			{Name: "lists", Scopes: []string{gtasksScopeRead}, Args: []string{"limit"}, Check: checkGtasksLists},
			{Name: "tasks", Scopes: []string{gtasksScopeRead}, Args: []string{"listId", "due", "status", "limit"}, Check: checkGtasksTasks},
			{Name: "addTask", Write: true, Scopes: []string{gtasksScopeWrite}, Args: []string{"title", "notes", "due", "listId"}, Check: checkGtasksAdd},
			{Name: "complete", Write: true, Scopes: []string{gtasksScopeWrite}, Args: []string{"taskId", "listId"}, Check: checkGtasksComplete},
		},
		Hosts:      googleTasksHosts,
		ClientFrom: GoogleCalendar,
		New: func(sec Secrets) (map[string]Operation, error) {
			t, err := newGoogleTasks(sec, googleTasksWire)
			if err != nil {
				return nil, err
			}
			return map[string]Operation{"lists": OperationFunc(t.lists), "tasks": OperationFunc(t.tasks),
				"addTask": OperationFunc(t.addTask), "complete": OperationFunc(t.complete)}, nil
		},
		NewOAuth: func(sec Secrets) (OAuthClient, error) {
			t, err := newGoogleTasks(sec, googleTasksWire)
			if err != nil {
				return nil, err
			}
			return t.g, nil
		},
	})
}

// googleTasks is the provider's ops; the OAuth client and do are Google
// Calendar's code on the Tasks wire.
type googleTasks struct {
	g *googleCalendar
}

func newGoogleTasks(sec Secrets, w gcalWire) (*googleTasks, error) {
	g, err := newGoogleClient(GoogleTasks, sec, w, googleTasksHosts)
	if err != nil {
		return nil, err
	}
	return &googleTasks{g: g}, nil
}

// --- arguments ---

const (
	gtasksDefaultLimit = 20
	gtasksMaxLimit     = 50
	gtasksDefaultList  = "@default"
)

var gtasksIDRe = regexp.MustCompile(`^[A-Za-z0-9_-]{1,128}$`)

func gtasksLimit(v string) (int, error) {
	if v == "" {
		return gtasksDefaultLimit, nil
	}
	x, err := strconv.Atoi(v)
	if err != nil || x < 1 || x > gtasksMaxLimit {
		return 0, fmt.Errorf("%w: limit must be 1-%d", ErrUnknown, gtasksMaxLimit)
	}
	return x, nil
}

// gtasksListID: a task list id, or the default list when empty.
func gtasksListID(v string) (string, error) {
	switch {
	case v == "" || v == gtasksDefaultList:
		return gtasksDefaultList, nil
	case gtasksIDRe.MatchString(v):
		return v, nil
	}
	return "", fmt.Errorf("%w: listId must be a Google Tasks list id", ErrUnknown)
}

func gtasksTaskID(v string) (string, error) {
	if !gtasksIDRe.MatchString(v) {
		return "", fmt.Errorf("%w: taskId must be a Google Tasks task id", ErrUnknown)
	}
	return v, nil
}

// gtasksDate reads YYYY-MM-DD. Google keeps only the date of a due
// (midnight UTC), so the date is sent as that.
func gtasksDate(name, v string) (time.Time, error) {
	t, err := time.ParseInLocation(time.DateOnly, v, time.UTC)
	if err != nil {
		return time.Time{}, fmt.Errorf("%w: %s must be YYYY-MM-DD", ErrUnknown, name)
	}
	return t, nil
}

// gtasksDueRange turns tasks' due into Google's [dueMin, dueMax): today,
// overdue (before today), week (today and the next 6 days) or one date.
// Today is the date in JST. Zero times are no bound.
func gtasksDueRange(v string, now time.Time) (time.Time, time.Time, error) {
	j := now.In(gcalJST)
	today := time.Date(j.Year(), j.Month(), j.Day(), 0, 0, 0, 0, time.UTC)
	switch v {
	case "":
		return time.Time{}, time.Time{}, nil
	case "today":
		return today, today.AddDate(0, 0, 1), nil
	case "overdue":
		return time.Time{}, today, nil
	case "week":
		return today, today.AddDate(0, 0, 7), nil
	}
	d, err := gtasksDate("due", v)
	if err != nil {
		return time.Time{}, time.Time{}, fmt.Errorf("%w: due must be today, overdue, week or YYYY-MM-DD", ErrUnknown)
	}
	return d, d.AddDate(0, 0, 1), nil
}

func gtasksStatus(v string) (string, error) {
	switch v {
	case "":
		return "open", nil
	case "open", "completed", "all":
		return v, nil
	}
	return "", fmt.Errorf("%w: status must be open, completed or all", ErrUnknown)
}

func checkGtasksLists(a map[string]string) error {
	_, err := gtasksLimit(a["limit"])
	return err
}

func checkGtasksTasks(a map[string]string) error {
	_, _, err := gtasksTasksQuery(a, time.Now())
	return err
}

func checkGtasksAdd(a map[string]string) error {
	_, _, err := gtasksAddBody(a)
	return err
}

func checkGtasksComplete(a map[string]string) error {
	if _, err := gtasksTaskID(a["taskId"]); err != nil {
		return err
	}
	_, err := gtasksListID(a["listId"])
	return err
}

// gtasksCanRead / gtasksCanWrite: the granted scopes cover the op (unknown
// scopes pass; Google then answers 403 and the person connects again).
func gtasksCanRead(t *Token) bool {
	return t == nil || len(t.Scopes) == 0 || slices.Contains(t.Scopes, gtasksScopeRead) || slices.Contains(t.Scopes, gtasksScopeWrite)
}

func gtasksCanWrite(t *Token) bool {
	return t == nil || len(t.Scopes) == 0 || slices.Contains(t.Scopes, gtasksScopeWrite)
}

// --- answers ---

const (
	gtasksNoTitle  = "(タイトルなし)"
	gtasksDone     = "完了"
	gtasksNotesMax = 120
)

type gtasksTask struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Notes       string `json:"notes"`
	Status      string `json:"status"`
	Due         string `json:"due"`
	WebViewLink string `json:"webViewLink"`
}

// row: title, the notes' first line (完了 first when done), the due date,
// the task on tasks.google.com.
func (t gtasksTask) row() Row {
	r := Row{Title: strings.TrimSpace(t.Title)}
	if r.Title == "" {
		r.Title = gtasksNoTitle
	}
	note, _, _ := strings.Cut(strings.TrimSpace(t.Notes), "\n")
	if utf8.RuneCountInString(note) > gtasksNotesMax {
		note = string([]rune(note)[:gtasksNotesMax]) + "…"
	}
	if t.Status == "completed" {
		note = strings.TrimSuffix(gtasksDone+"・"+note, "・")
	}
	r.Subtitle = note
	if len(t.Due) >= 10 {
		if _, err := time.Parse(time.DateOnly, t.Due[:10]); err == nil {
			r.Value = t.Due[:10]
		}
	}
	if u, err := url.Parse(t.WebViewLink); err == nil && u.Scheme == "https" && u.Host == "tasks.google.com" {
		r.URL = t.WebViewLink
	}
	return r
}

// --- ops ---

// lists lists the person's task lists, one page of limit (default 20):
// title = the name, value = the id (tasks / addTask / complete's listId).
func (t *googleTasks) lists(ctx context.Context, in Input) (Output, error) {
	if !gtasksCanRead(in.Token) {
		return Output{}, ErrNotConnected
	}
	n, err := gtasksLimit(in.Args["limit"])
	if err != nil {
		return Output{}, err
	}
	v := url.Values{"maxResults": {strconv.Itoa(n)}, "fields": {"items(id,title)"}}
	var r struct {
		Items []struct {
			ID    string `json:"id"`
			Title string `json:"title"`
		} `json:"items"`
	}
	if err := t.do(ctx, in.Token, http.MethodGet, "/users/@me/lists?"+v.Encode(), nil, &r); err != nil {
		return Output{}, err
	}
	out := Output{Rows: make([]Row, 0, len(r.Items))}
	for _, l := range r.Items {
		row := Row{Title: l.Title}
		if row.Title == "" {
			row.Title = gtasksNoTitle
		}
		if gtasksIDRe.MatchString(l.ID) {
			row.Value = l.ID
		}
		out.Rows = append(out.Rows, row)
	}
	return out, nil
}

// gtasksTasksQuery turns tasks' arguments into the path and query.
func gtasksTasksQuery(a map[string]string, now time.Time) (string, url.Values, error) {
	list, err := gtasksListID(a["listId"])
	if err != nil {
		return "", nil, err
	}
	from, to, err := gtasksDueRange(a["due"], now)
	if err != nil {
		return "", nil, err
	}
	status, err := gtasksStatus(a["status"])
	if err != nil {
		return "", nil, err
	}
	n, err := gtasksLimit(a["limit"])
	if err != nil {
		return "", nil, err
	}
	v := url.Values{
		"maxResults": {strconv.Itoa(n)},
		"fields":     {"items(id,title,notes,status,due,webViewLink)"},
	}
	switch status {
	case "open":
		v.Set("showCompleted", "false")
	case "completed":
		// completedMin leaves out the open tasks; hidden = done in Google's apps
		v.Set("showCompleted", "true")
		v.Set("showHidden", "true")
		v.Set("completedMin", "1970-01-01T00:00:00Z")
	case "all":
		v.Set("showCompleted", "true")
		v.Set("showHidden", "true")
	}
	if !from.IsZero() {
		v.Set("dueMin", from.Format(time.RFC3339))
	}
	if !to.IsZero() {
		v.Set("dueMax", to.Format(time.RFC3339))
	}
	return "/lists/" + url.PathEscape(list) + "/tasks", v, nil
}

// tasks lists one list's tasks (default: the default list, open ones),
// narrowed by due date and status, one page of limit (default 20).
func (t *googleTasks) tasks(ctx context.Context, in Input) (Output, error) {
	if !gtasksCanRead(in.Token) {
		return Output{}, ErrNotConnected
	}
	path, v, err := gtasksTasksQuery(in.Args, t.g.now())
	if err != nil {
		return Output{}, err
	}
	var r struct {
		Items []gtasksTask `json:"items"`
	}
	if err := t.do(ctx, in.Token, http.MethodGet, path+"?"+v.Encode(), nil, &r); err != nil {
		return Output{}, err
	}
	completedOnly := v.Has("completedMin")
	out := Output{Rows: make([]Row, 0, len(r.Items))}
	for _, task := range r.Items {
		if completedOnly && task.Status != "completed" {
			continue
		}
		out.Rows = append(out.Rows, task.row())
	}
	return out, nil
}

// gtasksAddBody checks addTask's arguments and builds the path and body.
func gtasksAddBody(a map[string]string) (string, []byte, error) {
	title, err := gcalText("title", a["title"], 1024, true)
	if err != nil {
		return "", nil, err
	}
	notes, err := gcalText("notes", a["notes"], 8192, false)
	if err != nil {
		return "", nil, err
	}
	var due string
	if v := a["due"]; v != "" {
		d, err := gtasksDate("due", v)
		if err != nil {
			return "", nil, err
		}
		due = d.Format(time.RFC3339)
	}
	list, err := gtasksListID(a["listId"])
	if err != nil {
		return "", nil, err
	}
	b, err := json.Marshal(struct {
		Title string `json:"title"`
		Notes string `json:"notes,omitempty"`
		Due   string `json:"due,omitempty"`
	}{title, notes, due})
	if err != nil {
		return "", nil, err
	}
	return "/lists/" + url.PathEscape(list) + "/tasks", b, nil
}

// addTask adds one task at the top of a list (default: the default list).
// Google takes no request id, so the service's never-retried write is the
// only guard against a second task.
func (t *googleTasks) addTask(ctx context.Context, in Input) (Output, error) {
	if !gtasksCanWrite(in.Token) {
		return Output{}, ErrNotConnected
	}
	path, body, err := gtasksAddBody(in.Args)
	if err != nil {
		return Output{}, err
	}
	var task gtasksTask
	if err := t.do(ctx, in.Token, http.MethodPost, path, body, &task); err != nil {
		return Output{}, err
	}
	row := task.row()
	return Output{ResultID: task.ID, Item: &row}, nil
}

// complete marks one task done. Doing it again changes nothing, so a
// retried request is harmless. A task or list the person does not have is
// 400 bad_request.
func (t *googleTasks) complete(ctx context.Context, in Input) (Output, error) {
	if !gtasksCanWrite(in.Token) {
		return Output{}, ErrNotConnected
	}
	if err := checkGtasksComplete(in.Args); err != nil {
		return Output{}, err
	}
	list, _ := gtasksListID(in.Args["listId"])
	id := in.Args["taskId"]
	var task gtasksTask
	if err := t.do(ctx, in.Token, http.MethodPatch, "/lists/"+url.PathEscape(list)+"/tasks/"+url.PathEscape(id),
		[]byte(`{"status":"completed"}`), &task); err != nil {
		return Output{}, err
	}
	row := task.row()
	return Output{ResultID: id, Item: &row}, nil
}

// do is Google Calendar's do (401 → ErrInvalidGrant, a missing scope →
// ErrNotConnected, 429 / 5xx / quota 403 → retryable) plus: an API not
// enabled stays upstream_unavailable; any other 403, 400 and 404 are a list
// or task the person cannot use (bad_request: connecting again would not
// help).
func (t *googleTasks) do(ctx context.Context, tok *Token, method, path string, body []byte, out any) error {
	ctype := ""
	if body != nil {
		ctype = "application/json"
	}
	err := t.g.do(ctx, tok, method, path, body, ctype, out)
	var pe *ProviderError
	if errors.As(err, &pe) && !pe.Retryable {
		switch pe.Status {
		case http.StatusForbidden:
			if errors.Is(err, errGoogleAPIDisabled) {
				return err
			}
			return &ProviderError{Status: pe.Status, Err: fmt.Errorf("%w: googleTasks: this person has no access to this list or task", ErrUnknown)}
		case http.StatusBadRequest, http.StatusNotFound:
			return &ProviderError{Status: pe.Status, Err: fmt.Errorf("%w: googleTasks has no such list or task", ErrUnknown)}
		}
	}
	return err
}
