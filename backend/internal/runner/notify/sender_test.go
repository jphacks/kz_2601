package notify

import (
	"bytes"
	"context"
	"log/slog"
	"reflect"
	"slices"
	"strings"
	"testing"
)

func TestDispatch(t *testing.T) {
	tokens := MemoryTokens{"a": {"ta1", "ta2"}, "b": {"tb"}, "c": {"tc"}, "d": {"td1", "td2"}}
	fs := &FakeSender{Results: map[string]Result{
		"ta2": {Status: 410, Unregistered: true}, // a still has ta1
		"tb":  {Status: 410, Unregistered: true},
		"tc":  {Status: 503, Retryable: true},
		"td1": {Status: 400, Reason: "BadDeviceToken"},
		"td2": {Status: 410, Unregistered: true},
	}}
	d := &Dispatcher{Tokens: tokens, Sender: fs}
	res := Resolution{Action: 2, To: []string{"a", "b", "c", "d", "e"}, Skipped: []Skip{{UID: "z", Reason: ReasonBlocked}}}
	c := Content{Key: "outbox/1", AppID: "app1", AppName: "部活", Title: "題", Body: "本文"}
	out, err := d.Dispatch(context.Background(), res, c)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(out.Sent, []string{"a"}) {
		t.Errorf("sent %v", out.Sent)
	}
	wantSkip := []Skip{{UID: "z", Reason: ReasonBlocked}, {UID: "b", Reason: ReasonUnregistered},
		{UID: "c", Reason: ReasonSendFailed}, {UID: "d", Reason: ReasonSendFailed}, {UID: "e", Reason: ReasonNoDevice}}
	if !reflect.DeepEqual(out.Skipped, wantSkip) {
		t.Errorf("skipped %v", out.Skipped)
	}
	if !out.Retryable {
		t.Error("want retryable (c got 503)")
	}
	wantUnreg := []DeviceToken{{"a", "ta2"}, {"b", "tb"}, {"d", "td2"}}
	if !reflect.DeepEqual(out.Unregistered, wantUnreg) {
		t.Errorf("unregistered %v", out.Unregistered)
	}
	ps := fs.Pushes()
	if len(ps) != 6 {
		t.Fatalf("%d pushes", len(ps))
	}
	for _, p := range ps {
		if p.CollapseID != CollapseID("outbox/1", 2) || p.AppID != "app1" || p.AppName != "部活" || p.Title != "題" || p.Body != "本文" {
			t.Errorf("push %+v", p)
		}
	}
	s := out.Summary()
	if s.Sent != 1 || s.Skipped[ReasonSendFailed] != 2 || s.Skipped[ReasonNoDevice] != 1 || s.Action != 2 {
		t.Errorf("summary %+v", s)
	}
}

func TestDispatchRefused(t *testing.T) {
	fs := &FakeSender{}
	d := &Dispatcher{Tokens: MemoryTokens{}, Sender: fs}
	out, err := d.Dispatch(context.Background(), Resolution{Refused: ReasonScope}, Content{})
	if err != nil || out.Refused != ReasonScope || len(fs.Pushes()) != 0 {
		t.Errorf("got %+v %v", out, err)
	}
	if _, err := (&Dispatcher{}).Dispatch(context.Background(), Resolution{}, Content{}); err == nil {
		t.Error("want error without Tokens/Sender")
	}
}

func TestCollapseID(t *testing.T) {
	a := CollapseID("k", 0)
	if len(a) != 64 || a != CollapseID("k", 0) || a == CollapseID("k", 1) || a == CollapseID("k2", 0) {
		t.Errorf("collapse id %q", a)
	}
}

func TestLogSenderHidesContent(t *testing.T) {
	var buf bytes.Buffer
	s := LogSender{Logger: slog.New(slog.NewJSONHandler(&buf, nil))}
	if _, err := s.Send(context.Background(), Push{Token: "devtoken123", AppID: "app1", Title: "秘密の題", Body: "秘密の本文"}); err != nil {
		t.Fatal(err)
	}
	log := buf.String()
	for _, bad := range []string{"devtoken123", "秘密"} {
		if strings.Contains(log, bad) {
			t.Errorf("log contains %q: %s", bad, log)
		}
	}
	if !strings.Contains(log, "app1") {
		t.Errorf("log lacks app id: %s", log)
	}
}
