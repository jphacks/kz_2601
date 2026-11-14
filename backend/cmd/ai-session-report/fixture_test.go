package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/jphacks/kz_2601/backend/internal/httpx"
	"github.com/jphacks/kz_2601/backend/internal/metrics"
)

// Fake data shared by the unit and emulator tests. Everything here is made up.

const (
	fxTranscript1 = "家計簿を作って。食費と日用品を分けたい"
	fxTranscript2 = "カテゴリを足して"
	fxTranscript3 = "グラフを付けて"
	fxOtherText   = "OTHER-USER-SECRET-TEXT"
	fxFeedback    = "グラフが思ってたのと違う"
)

const fxDSL = `{"schemaVersion":1,"app":{"name":"家計簿"},"initialScreen":"home",
"entities":[{"id":"expense","name":"支出","fields":[{"id":"amount"},{"id":"category"}]}],
"screens":[{"id":"home","title":"ホーム","components":[{"type":"list","entity":"expense"},{"type":"button","label":"追加"}]}]}`

type fxApp struct {
	uid, id, name string
	versions      []versionData
	records       int
}

type fixture struct {
	until    time.Time
	alice    string // the reported user
	other    string // not reported
	apps     []fxApp
	feedback []struct {
		uid, appID, kind, msg string
		at                    time.Time
	}
	logLines  []string
	goneAppID string
}

// newFixture: alice has one app with two sessions (the first goes wrong:
// slow regenerated create, quick re-request, quick undo, no-op edit, a 500)
// and one deleted app; she also chatted once without generating. other's
// data must never show up.
func newFixture(until time.Time, alice, other string) *fixture {
	T := until.Add(-48 * time.Hour).Truncate(time.Second)
	f := &fixture{until: until, alice: alice, other: other, goneAppID: "app-gone-" + alice}
	app1 := "app1-" + alice
	f.apps = []fxApp{
		{uid: alice, id: app1, name: "家計簿", records: 3, versions: []versionData{
			{N: 1, CreatedAt: T, Kind: "system", Mode: "full", DSL: json.RawMessage(`{}`)},
			{N: 2, CreatedAt: T.Add(2 * time.Minute), Kind: "ai", Mode: "full", Transcript: fxTranscript1, Summary: "家計簿を作りました", DSL: json.RawMessage(fxDSL)},
			{N: 3, CreatedAt: T.Add(4 * time.Minute), Kind: "ai", Mode: "patch", Transcript: fxTranscript2, Summary: "カテゴリを追加", DSL: json.RawMessage(fxDSL)},
			{N: 4, CreatedAt: T.Add(6 * time.Minute), Kind: "restore", Mode: "full", RestoredFrom: 2, DSL: json.RawMessage(fxDSL)},
			{N: 5, CreatedAt: T.Add(3 * time.Hour), Kind: "ai", Mode: "patch", Transcript: fxTranscript3, Summary: "グラフを追加", DSL: json.RawMessage(fxDSL)},
		}},
		{uid: other, id: "app-other-" + other, name: "other", records: 0, versions: []versionData{
			{N: 2, CreatedAt: T.Add(time.Minute), Kind: "ai", Mode: "full", Transcript: fxOtherText, DSL: json.RawMessage(fxDSL)},
		}},
	}
	f.feedback = append(f.feedback, struct {
		uid, appID, kind, msg string
		at                    time.Time
	}{alice, app1, "bug", fxFeedback, T.Add(10 * time.Minute)})
	f.feedback = append(f.feedback, struct {
		uid, appID, kind, msg string
		at                    time.Time
	}{other, "x", "bug", fxOtherText, T.Add(10 * time.Minute)})

	ah, oh := httpx.UIDHash(alice), httpx.UIDHash(other)
	a1, gone := metrics.AppHash(app1), metrics.AppHash(f.goneAppID)
	line := func(at time.Time, payload string) {
		f.logLines = append(f.logLines, fmt.Sprintf(`{"timestamp":%q,"jsonPayload":%s}`, at.UTC().Format(time.RFC3339Nano), payload))
	}
	chat := func(at time.Time, uh string) {
		line(at, fmt.Sprintf(`{"event":"ai.usage","operation":"chat","attempt":1,"latency_ms":900,"uid_hash":%q}`, uh))
	}
	chat(T.Add(30*time.Second), ah)
	chat(T.Add(60*time.Second), ah)
	line(T.Add(61*time.Second), fmt.Sprintf(`{"event":"ai.usage","operation":"chat","attempt":2,"uid_hash":%q}`, ah)) // retry: same turn
	line(T.Add(2*time.Minute+5*time.Second), fmt.Sprintf(`{"event":"ai.generate","mode":"create","outcome":"regenerated","regenerations":1,"duration_ms":70000,"uid_hash":%q,"app_hash":%q}`, ah, a1))
	line(T.Add(4*time.Minute+3*time.Second), fmt.Sprintf(`{"event":"ai.generate","mode":"edit","outcome":"validated","duration_ms":9000,"uid_hash":%q,"app_hash":%q}`, ah, a1))
	line(T.Add(6*time.Minute+1*time.Second), fmt.Sprintf(`{"event":"ai.generate","mode":"undo","outcome":"restored","uid_hash":%q,"app_hash":%q}`, ah, a1))
	line(T.Add(7*time.Minute), fmt.Sprintf(`{"event":"http.request","path":"/v1/generate","status":500,"duration":1500000000,"uid_hash":%q}`, ah))
	line(T.Add(8*time.Minute), fmt.Sprintf(`{"event":"ai.generate","mode":"edit","outcome":"unchanged","duration_ms":8000,"uid_hash":%q,"app_hash":%q}`, ah, a1))
	line(T.Add(3*time.Hour+4*time.Second), fmt.Sprintf(`{"event":"ai.generate","mode":"edit","outcome":"validated","duration_ms":10000,"uid_hash":%q,"app_hash":%q}`, ah, a1))
	line(T.Add(24*time.Hour), fmt.Sprintf(`{"event":"ai.generate","mode":"create","outcome":"validated","duration_ms":12000,"uid_hash":%q,"app_hash":%q}`, ah, gone))
	chat(T.Add(26*time.Hour), ah) // no generation follows
	chat(T.Add(26*time.Hour+time.Minute), ah)
	line(T.Add(-20*24*time.Hour), fmt.Sprintf(`{"event":"ai.generate","outcome":"failed","uid_hash":%q,"app_hash":%q}`, ah, a1)) // before the period
	line(T.Add(5*time.Minute), fmt.Sprintf(`{"event":"ai.generate","outcome":"failed","uid_hash":%q,"app_hash":"x"}`, oh))
	return f
}

func (f *fixture) logs() string { return strings.Join(f.logLines, "\n") + "\n" }

// fakeReader serves the fixture without Firestore.
type fakeReader struct{ f *fixture }

func (r fakeReader) Apps(_ context.Context, uid string, since time.Time) ([]appData, error) {
	var out []appData
	for _, a := range r.f.apps {
		if a.uid != uid {
			continue
		}
		last := a.versions[len(a.versions)-1]
		out = append(out, appData{ID: a.id, Name: a.name, UpdatedAt: last.CreatedAt, DSL: last.DSL, Records: -1})
	}
	return out, nil
}

func (r fakeReader) Versions(_ context.Context, uid, appID string, since time.Time) ([]versionData, error) {
	for _, a := range r.f.apps {
		if a.uid == uid && a.id == appID {
			var out []versionData
			for _, v := range a.versions {
				if !v.CreatedAt.Before(since) {
					out = append(out, v)
				}
			}
			return out, nil
		}
	}
	return nil, nil
}

func (r fakeReader) RecordCount(_ context.Context, uid, appID string, _ []string, _ int) (int, error) {
	for _, a := range r.f.apps {
		if a.uid == uid && a.id == appID {
			return a.records, nil
		}
	}
	return 0, nil
}

func (r fakeReader) Feedback(_ context.Context, uid string, since time.Time) ([]feedbackData, error) {
	var out []feedbackData
	for i, fb := range r.f.feedback {
		if fb.uid == uid && !fb.at.Before(since) {
			out = append(out, feedbackData{ID: fmt.Sprint(i), Kind: fb.kind, AppID: fb.appID, Message: fb.msg, CreatedAt: fb.at})
		}
	}
	return out, nil
}
