package runner_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/jphacks/kz_2601/backend/internal/runner"
	"github.com/jphacks/kz_2601/backend/internal/store"
)

// #271: rules and addressed records.

// addressedDSL is the test app with rules (nil: keep its own) and, when
// plain, without addressedTo.
func addressedDSL(t *testing.T, plain bool, rules ...map[string]any) []byte {
	t.Helper()
	var a map[string]any
	if err := json.Unmarshal(readsDSL(t, "server_rules_addressed.json"), &a); err != nil {
		t.Fatal(err)
	}
	if plain {
		delete(a["entities"].([]any)[0].(map[string]any), "addressedTo")
	}
	if rules != nil {
		a["serverRules"] = rules
	}
	out, err := json.Marshal(a)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// A message notifies its recipients with the text, and nobody else.
func TestAddressedNotifyRecipients(t *testing.T) {
	stores(t, options{age: oldAccounts{}}, func(t *testing.T, h *harness) {
		owner, ann, bob, cid := "uid-ad-o"+h.sfx, "uid-ad-a"+h.sfx, "uid-ad-b"+h.sfx, "uid-ad-c"+h.sfx
		for u, tok := range map[string]string{owner: "t-o", ann: "t-a", bob: "t-b", cid: "t-c"} {
			h.tokens[u] = []string{tok}
		}
		id := "AD-NOTIFY" + h.sfx
		app := h.circle(id, owner, addressedDSL(t, false), ann, bob, cid)
		for _, u := range []string{ann, bob, cid} {
			h.consent(u, id)
		}
		h.post(ann, app+"/entities/messages/records", `{"data":{"to":["`+bob+`"],"body":"明日は休みです"}}`)
		h.tick()
		pushes := h.sender.Pushes()
		if len(pushes) != 1 || pushes[0].Token != "t-b" || !strings.Contains(pushes[0].Body, "明日は休みです") {
			t.Fatalf("pushes: %+v", pushes)
		}
	})
}

// A rule that reads an addressed entity does not run, even when the stored
// version was never validated; the status says it did not run.
func TestAddressedRunCheck(t *testing.T) {
	stores(t, options{age: oldAccounts{}}, func(t *testing.T, h *harness) {
		owner, ann := "uid-ad2-o"+h.sfx, "uid-ad2-a"+h.sfx
		h.tokens[owner] = []string{"t-o"}
		id := "AD-CHECK" + h.sfx
		app := h.circle(id, owner, addressedDSL(t, false), ann)
		_, cur := h.do(owner, http.MethodGet, app+"/dsl", "", nil)
		count := map[string]any{"agg": "count", "entity": "messages"}
		raw := addressedDSL(t, false, map[string]any{"id": "counted", "on": map[string]any{"record": "created", "entity": "notes"},
			"do": []any{map[string]any{"notify": map[string]any{"to": map[string]any{"kind": "owner"}, "title": "件数", "body": "{n}", "with": map[string]any{"n": count}}}}})
		c := store.Commit{BaseVersion: int(cur["version"].(float64)), DSL: raw}
		v, err := h.st.CommitVersion(t.Context(), owner, id, c)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := h.st.PublishApp(t.Context(), owner, id, raw, "連絡", v.Version); err != nil {
			t.Fatal(err)
		}
		h.post(ann, app+"/entities/notes/records", `{"data":{"text":"x"}}`)
		h.tick()
		if n := len(h.sender.Pushes()); n != 0 {
			t.Fatalf("pushes: %d", n)
		}
		ev := h.oneEvent(id)
		if ev.RuleOutcomes["counted"] != runner.OutcomeSkippedStopped || !strings.Contains(h.logs.String(), runner.ReasonAddressed) {
			t.Fatalf("event: %+v / %s", ev, h.logs.String())
		}
	})
}

// addressedTo removed later: the records keep their readers, so agg does not
// count them, a ref does not read them and they trigger no rule.
func TestAddressedStoredReaders(t *testing.T) {
	stores(t, options{age: oldAccounts{}}, func(t *testing.T, h *harness) {
		owner, ann, bob := "uid-ad3-o"+h.sfx, "uid-ad3-a"+h.sfx, "uid-ad3-b"+h.sfx
		h.tokens[owner] = []string{"t-o"}
		id := "AD-READERS" + h.sfx
		app := h.circle(id, owner, addressedDSL(t, false), ann, bob)
		msg := h.post(ann, app+"/entities/messages/records", `{"data":{"to":["`+bob+`"],"body":"秘密の本文"}}`)
		open := map[string]any{"id": "copy", "on": map[string]any{"record": "created", "entity": "notes"},
			"do": []any{
				map[string]any{"updateRecord": map[string]any{"values": map[string]any{"text": map[string]any{"ref": "about.body"}}}},
				map[string]any{"notify": map[string]any{"to": map[string]any{"kind": "owner"}, "title": "件数", "body": "{n}",
					"with": map[string]any{"n": map[string]any{"agg": "count", "entity": "messages"}}}},
			}}
		updated := map[string]any{"id": "changed", "on": map[string]any{"record": "updated", "entity": "messages"},
			"do": []any{map[string]any{"notify": map[string]any{"to": map[string]any{"kind": "owner"}, "title": "変更", "body": "{body}"}}}}
		h.putDSL(owner, app, addressedDSL(t, true, open, updated))
		h.tick()
		note := h.post(ann, app+"/entities/notes/records", `{"data":{"about":"`+msg+`","text":"x"}}`)
		h.tick()
		pushes := h.sender.Pushes()
		if len(pushes) != 1 || pushes[0].Body != "0" {
			t.Fatalf("pushes: %+v", pushes)
		}
		if rec := h.record(store.PublicRecords(id, "notes"), note); rec.Data["text"] == "秘密の本文" {
			t.Fatalf("note: %+v", rec.Data)
		}
		if code, out := h.do(ann, http.MethodPatch, app+"/entities/messages/records/"+msg, `{"data":{"body":"秘密の本文 2"}}`, nil); code != http.StatusOK {
			t.Fatalf("patch: %d %v", code, out)
		}
		h.tick()
		if n := len(h.sender.Pushes()); n != 1 {
			t.Fatalf("a record with stored readers triggered a rule: %+v", h.sender.Pushes())
		}
	})
}
