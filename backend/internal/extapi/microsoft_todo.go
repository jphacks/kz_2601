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
)

// Microsoft To Do (Microsoft Graph v1.0): free (one `runs` a call), OAuth
// with PKCE on the common tenant.
//
//	lists(limit?)                            Tasks.Read
//	tasks(listId, limit?)                    Tasks.Read
//	addTask(listId, title, dueDate?, body?)  Tasks.ReadWrite
//
// The OAuth client is Outlook calendar's unless EXT_MICROSOFT_TODO_CLIENT_ID
// / _CLIENT_SECRET are set (ClientFrom: one app registration). offline_access
// is always asked for. Graph To Do has no idempotency key, so the service's
// never-retried write is the only guard against a second task. Microsoft has
// no revoke endpoint: disconnecting deletes the stored token.
//
// Env: EXT_MICROSOFT_TODO_ENABLED=1. Off by default; not in the review build
// (NeedsOAuth).
const MicrosoftTodo = "microsoftTodo"

const (
	mstodoScopeRead  = "https://graph.microsoft.com/Tasks.Read"
	mstodoScopeWrite = "https://graph.microsoft.com/Tasks.ReadWrite"
)

var microsoftTodoWire = outlookWire{
	AuthURL:  "https://login.microsoftonline.com/common/oauth2/v2.0/authorize",
	TokenURL: "https://login.microsoftonline.com/common/oauth2/v2.0/token",
	APIBase:  "https://graph.microsoft.com/v1.0",
}

var microsoftTodoHosts = []string{"https://login.microsoftonline.com", "https://graph.microsoft.com"}

func init() {
	register(Provider{ID: MicrosoftTodo, Billing: BillingFree, NeedsOAuth: true, NameJa: "Microsoft To Do", NameEn: "Microsoft To Do",
		Ops: []OpSpec{
			{Name: "lists", Scopes: []string{mstodoScopeRead}, Args: []string{"limit"}, Check: checkMstodoLists},
			{Name: "tasks", Scopes: []string{mstodoScopeRead}, Args: []string{"listId", "limit"}, Check: checkMstodoTasks},
			{Name: "addTask", Write: true, Scopes: []string{mstodoScopeWrite}, Args: []string{"listId", "title", "dueDate", "body"}, Check: checkMstodoAdd},
		},
		Hosts:      microsoftTodoHosts,
		ClientFrom: OutlookCalendar,
		New: func(sec Secrets) (map[string]Operation, error) {
			m, err := newMicrosoftTodo(sec, microsoftTodoWire)
			if err != nil {
				return nil, err
			}
			return map[string]Operation{"lists": OperationFunc(m.lists), "tasks": OperationFunc(m.tasks), "addTask": OperationFunc(m.addTask)}, nil
		},
		NewOAuth: func(sec Secrets) (OAuthClient, error) {
			m, err := newMicrosoftTodo(sec, microsoftTodoWire)
			if err != nil {
				return nil, err
			}
			return m.o, nil
		},
	})
}

// microsoftTodo is the provider's ops; the OAuth client and do are Outlook
// calendar's code on the To Do wire.
type microsoftTodo struct {
	o *outlookCalendar
}

func newMicrosoftTodo(sec Secrets, w outlookWire) (*microsoftTodo, error) {
	o, err := newMicrosoftClient(MicrosoftTodo, sec, w, microsoftTodoHosts)
	if err != nil {
		return nil, err
	}
	return &microsoftTodo{o: o}, nil
}

// --- arguments ---

const (
	mstodoDefaultLimit = 20
	mstodoMaxLimit     = 50
	mstodoNoTitle      = "(タイトルなし)"
	mstodoDefaultList  = "既定のリスト"
)

// mstodoIDRe: a Graph To Do list id (base64url-ish, "=" padding).
var mstodoIDRe = regexp.MustCompile(`^[A-Za-z0-9_=-]{1,256}$`)

func mstodoLimit(v string) (int, error) {
	if v == "" {
		return mstodoDefaultLimit, nil
	}
	x, err := strconv.Atoi(v)
	if err != nil || x < 1 || x > mstodoMaxLimit {
		return 0, fmt.Errorf("%w: limit must be 1-%d", ErrBadArgs, mstodoMaxLimit)
	}
	return x, nil
}

func mstodoListID(v string) (string, error) {
	if !mstodoIDRe.MatchString(v) {
		return "", fmt.Errorf("%w: listId must be a Microsoft To Do list id", ErrBadArgs)
	}
	return v, nil
}

func checkMstodoLists(a map[string]string) error {
	_, err := mstodoLimit(a["limit"])
	return err
}

func checkMstodoTasks(a map[string]string) error {
	if _, err := mstodoListID(a["listId"]); err != nil {
		return err
	}
	_, err := mstodoLimit(a["limit"])
	return err
}

func checkMstodoAdd(a map[string]string) error {
	_, _, err := mstodoAddBody(a)
	return err
}

// mstodoCanRead / mstodoCanWrite: the granted scopes cover the op (unknown
// scopes pass; Graph then answers 403 and the person connects again).
func mstodoCanRead(t *Token) bool {
	return t == nil || len(t.Scopes) == 0 || mstodoHas(t.Scopes, mstodoScopeRead) || mstodoHas(t.Scopes, mstodoScopeWrite)
}

func mstodoCanWrite(t *Token) bool {
	return t == nil || len(t.Scopes) == 0 || mstodoHas(t.Scopes, mstodoScopeWrite)
}

func mstodoHas(scopes []string, want string) bool {
	return slices.ContainsFunc(scopes, func(s string) bool { return strings.EqualFold(s, want) })
}

// --- answers ---

// mstodoStatus is Graph's status in Japanese (unknown ones as they are).
func mstodoStatus(s string) string {
	switch s {
	case "notStarted":
		return "未着手"
	case "inProgress":
		return "進行中"
	case "completed":
		return "完了"
	case "waitingOnOthers":
		return "待ち"
	case "deferred":
		return "延期"
	}
	return s
}

type mstodoTask struct {
	ID          string       `json:"id"`
	Title       string       `json:"title"`
	Status      string       `json:"status"`
	DueDateTime *outlookWhen `json:"dueDateTime"`
}

// row: title, the due date (JST: Prefer header), the status.
func (t mstodoTask) row() Row {
	r := Row{Title: strings.TrimSpace(t.Title), Value: mstodoStatus(t.Status)}
	if r.Title == "" {
		r.Title = mstodoNoTitle
	}
	if t.DueDateTime != nil && len(t.DueDateTime.DateTime) >= 10 {
		if _, err := time.Parse(time.DateOnly, t.DueDateTime.DateTime[:10]); err == nil {
			r.Subtitle = t.DueDateTime.DateTime[:10]
		}
	}
	return r
}

// --- ops ---

// lists lists the person's To Do lists, one page of limit (default 20):
// title = the name, value = the id (tasks / addTask's listId).
func (m *microsoftTodo) lists(ctx context.Context, in Input) (Output, error) {
	if !mstodoCanRead(in.Token) {
		return Output{}, ErrNotConnected
	}
	n, err := mstodoLimit(in.Args["limit"])
	if err != nil {
		return Output{}, err
	}
	v := url.Values{"$top": {strconv.Itoa(n)}, "$select": {"id,displayName,wellknownListName"}}
	var r struct {
		Value []struct {
			ID          string `json:"id"`
			DisplayName string `json:"displayName"`
			WellKnown   string `json:"wellknownListName"`
		} `json:"value"`
	}
	if err := m.do(ctx, in.Token, http.MethodGet, "/me/todo/lists?"+v.Encode(), nil, &r); err != nil {
		return Output{}, err
	}
	out := Output{Rows: make([]Row, 0, len(r.Value))}
	for _, l := range r.Value {
		row := Row{Title: strings.TrimSpace(l.DisplayName)}
		if row.Title == "" {
			row.Title = mstodoNoTitle
		}
		if l.WellKnown == "defaultList" {
			row.Subtitle = mstodoDefaultList
		}
		if mstodoIDRe.MatchString(l.ID) {
			row.Value = l.ID
		}
		out.Rows = append(out.Rows, row)
	}
	return out, nil
}

// tasks lists one list's tasks, one page of limit (default 20).
func (m *microsoftTodo) tasks(ctx context.Context, in Input) (Output, error) {
	if !mstodoCanRead(in.Token) {
		return Output{}, ErrNotConnected
	}
	list, err := mstodoListID(in.Args["listId"])
	if err != nil {
		return Output{}, err
	}
	n, err := mstodoLimit(in.Args["limit"])
	if err != nil {
		return Output{}, err
	}
	v := url.Values{"$top": {strconv.Itoa(n)}, "$select": {"id,title,status,dueDateTime,importance"}}
	var r struct {
		Value []mstodoTask `json:"value"`
	}
	if err := m.do(ctx, in.Token, http.MethodGet, "/me/todo/lists/"+url.PathEscape(list)+"/tasks?"+v.Encode(), nil, &r); err != nil {
		return Output{}, err
	}
	out := Output{Rows: make([]Row, 0, len(r.Value))}
	for _, t := range r.Value {
		out.Rows = append(out.Rows, t.row())
	}
	return out, nil
}

// mstodoAddBody checks addTask's arguments and builds the path and body.
func mstodoAddBody(a map[string]string) (string, []byte, error) {
	list, err := mstodoListID(a["listId"])
	if err != nil {
		return "", nil, err
	}
	title, err := gcalText("title", a["title"], 255, true)
	if err != nil {
		return "", nil, err
	}
	text, err := gcalText("body", a["body"], 8000, false)
	if err != nil {
		return "", nil, err
	}
	type content struct {
		Content     string `json:"content"`
		ContentType string `json:"contentType"`
	}
	body := struct {
		Title string       `json:"title"`
		Due   *outlookWhen `json:"dueDateTime,omitempty"`
		Body  *content     `json:"body,omitempty"`
	}{Title: title}
	if v := a["dueDate"]; v != "" {
		d, err := time.ParseInLocation(time.DateOnly, v, gcalJST)
		if err != nil {
			return "", nil, fmt.Errorf("%w: dueDate must be YYYY-MM-DD", ErrBadArgs)
		}
		body.Due = &outlookWhen{DateTime: d.Format("2006-01-02T15:04:05"), TimeZone: outlookTZ}
	}
	if text != "" {
		body.Body = &content{Content: text, ContentType: "text"}
	}
	b, err := json.Marshal(body)
	if err != nil {
		return "", nil, err
	}
	return "/me/todo/lists/" + url.PathEscape(list) + "/tasks", b, nil
}

// addTask adds one task to a list. Graph takes no request id for To Do, so
// the service's never-retried write is the only guard against a second task.
func (m *microsoftTodo) addTask(ctx context.Context, in Input) (Output, error) {
	if !mstodoCanWrite(in.Token) {
		return Output{}, ErrNotConnected
	}
	path, body, err := mstodoAddBody(in.Args)
	if err != nil {
		return Output{}, err
	}
	var task mstodoTask
	if err := m.do(ctx, in.Token, http.MethodPost, path, body, &task); err != nil {
		return Output{}, err
	}
	row := task.row()
	return Output{ResultID: task.ID, Item: &row}, nil
}

// do is Outlook calendar's do (401 → ErrInvalidGrant, access denied →
// ErrNotConnected, 429 / 5xx → retryable) plus: a 400 / 404 is a list or
// task the person does not have (bad_request: connecting again would not
// help).
func (m *microsoftTodo) do(ctx context.Context, tok *Token, method, path string, body []byte, out any) error {
	err := m.o.do(ctx, tok, method, path, body, out)
	var pe *ProviderError
	if errors.As(err, &pe) && !pe.Retryable && (pe.Status == http.StatusBadRequest || pe.Status == http.StatusNotFound) {
		return &ProviderError{Status: pe.Status, Err: fmt.Errorf("%w: microsoftTodo has no such list or task", ErrUnknown)}
	}
	return err
}
