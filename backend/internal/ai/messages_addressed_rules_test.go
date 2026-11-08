package ai

import (
	"context"
	"slices"
	"strings"
	"testing"

	"github.com/jphacks/kz_2601/backend/pkg/dsl"
)

// #271: the prompts teach addressed messages; the schema offers addressedTo and members.
func TestPromptTeachesAddressed(t *testing.T) {
	if !strings.Contains(systemPrompt, addressedRules) || !strings.Contains(systemPrompt, chatThreadRules) {
		t.Fatal("system prompt lacks the addressed (or chatThread) rules")
	}
	for name, p := range map[string]string{"chatCreate": chatCreateSystemPrompt, "chatEdit": chatEditSystemPrompt} {
		if !strings.Contains(p, addressedChatRule) || !strings.Contains(p, dataRolesChatRule) || !strings.Contains(p, singleChatRule) {
			t.Errorf("%s: chat rules missing", name)
		}
	}
	for _, k := range []string{`"addressedTo"`, `"member","members"`} {
		if !strings.Contains(string(generationSchema), k) {
			t.Errorf("generation schema lacks %s", k)
		}
	}
}

// dmDoc: a group message app with a 1 対 1 note picked by a member picker.
const dmDoc = `{"schemaVersion":1,"app":{"name":"連絡","voiceEntry":"none"},"initialScreen":"inbox",
"entities":[{"id":"messages","name":"メッセージ","scope":"shared","addressedTo":"to","marks":[{"id":"read","icon":"eye","label":"既読"}],"fields":[
 {"id":"to","name":"宛先","type":"members","required":true},{"id":"body","name":"本文","type":"text","required":true}]},
 {"id":"notes","name":"ひとこと","scope":"shared","addressedTo":"to","fields":[
 {"id":"to","name":"宛先","type":"member","required":true},{"id":"body","name":"本文","type":"string"}]}],
"screens":[{"id":"inbox","title":"受信箱","components":[
 {"type":"list","entity":"messages","filter":{"field":"$author","op":"neq","value":{"me":true}},"item":{"title":"body","mark":"read"}},
 {"type":"button","label":"送る","action":{"type":"navigate","screen":"compose"}},
 {"type":"button","label":"ひとこと","action":{"type":"navigate","screen":"note"}}]},
 {"id":"compose","title":"送る","entity":"messages","toolbar":[{"label":"送信","action":{"type":"saveRecord","entity":"messages"}}],"components":[
 {"type":"picker","field":"to","label":"宛先"},{"type":"textEditor","field":"body"}]},
 {"id":"note","title":"ひとこと","entity":"notes","toolbar":[{"label":"送信","action":{"type":"saveRecord","entity":"notes"}}],"components":[
 {"type":"picker","field":"to","label":"宛先"},{"type":"textField","field":"body","label":"本文"}]}]}`

var currentFeatures = []string{FeatureRecordRetention, FeatureNotificationWhen, FeatureMatching, FeatureSingleEntity, FeatureDataRoles, FeatureAddressed, FeatureNotificationNear,
	FeatureAILong, FeatureAIInputPhoto, FeatureAISaveTo, FeatureAIInputFeeds, FeatureLookupBook, FeatureTextHighlight, FeatureFeedV2}

// featuresWithout is currentFeatures minus some.
func featuresWithout(drop ...string) []string {
	var out []string
	for _, f := range currentFeatures {
		if !slices.Contains(drop, f) {
			out = append(out, f)
		}
	}
	return out
}

// Builds without addressedMessages: no addressedTo / members in the schema,
// prompt or output, and the output stays valid.
func TestAddressedRestriction(t *testing.T) {
	if _, err := dsl.Parse([]byte(dmDoc)); err != nil {
		t.Fatalf("dmDoc must be valid: %v", err)
	}
	all := allComponentTypes()
	if r := NewClientRestriction(all, nil, currentFeatures); r.Active() {
		t.Fatal("a current build must stay unrestricted")
	}
	for name, r := range map[string]Restriction{
		"no addressed": NewClientRestriction(all, nil, featuresWithout(FeatureAddressed)),
		"no roles":     NewClientRestriction(all, nil, []string{FeatureRecordRetention, FeatureNotificationWhen, FeatureMatching, FeatureSingleEntity}),
	} {
		t.Run(name, func(t *testing.T) {
			sch := string(r.assets().genSch)
			if strings.Contains(sch, `"addressedTo"`) || strings.Contains(sch, `"refs","members"`) || strings.Contains(sch, `"member","members"`) {
				t.Fatal("restricted generation schema still has addressedTo / members")
			}
			p := restrictPrompt(withRestriction(context.Background(), r), systemPrompt)
			if strings.Contains(p, "宛先を選んで送るメッセージ") || !strings.Contains(p, "addressedTo と type members の field は使わない") {
				t.Fatal("restricted prompt: the rules must go and the note come")
			}
			out, notes, changed := enforceRestriction([]byte(dmDoc), r)
			if !changed || len(notes) == 0 || strings.Contains(string(out), `"addressedTo"`) || strings.Contains(string(out), `"members"`) {
				t.Fatalf("enforce: changed=%v notes=%v out=%s", changed, notes, out)
			}
			if _, err := dsl.Parse(out); err != nil {
				t.Fatalf("the enforced DSL must stay valid: %v\n%s", err, out)
			}
		})
	}
	// no shared data: addressed goes with the scope
	r := NewClientRestriction(all, []string{FeatureSharedData}, currentFeatures)
	if p := restrictPrompt(withRestriction(context.Background(), r), chatCreateSystemPrompt); strings.Contains(p, addressedChatRule) {
		t.Fatal("no shared data: the chat rule must go")
	}
}

// An older build gets member / members as string: a picker on them becomes a
// textField wherever it sits, a picker on an enum stays, and the DSL stays valid.
func TestRestrictionTurnsParticipantPickersIntoTextFields(t *testing.T) {
	const doc = `{"schemaVersion":1,"app":{"name":"当番","voiceEntry":"none"},"initialScreen":"form",
"entities":[{"id":"shifts","name":"当番","scope":"shared","fields":[
 {"id":"staff","name":"担当","type":"member","required":true},
 {"id":"others","name":"手伝い","type":"members"},
 {"id":"kind","name":"種類","type":"enum","options":["朝","夜"]}]}],
"screens":[{"id":"form","title":"追加","entity":"shifts","toolbar":[{"label":"保存","action":{"type":"saveRecord","entity":"shifts"}}],"components":[
 {"type":"section","title":"誰が","children":[{"type":"picker","field":"staff","label":"担当"},{"type":"picker","field":"others","label":"手伝い"}]},
 {"type":"picker","field":"kind","label":"種類"}]}]}`
	if _, err := dsl.Parse([]byte(doc)); err != nil {
		t.Fatalf("doc must be valid: %v", err)
	}
	all := allComponentTypes()
	base := []string{FeatureRecordRetention, FeatureNotificationWhen, FeatureMatching, FeatureSingleEntity}
	for name, tc := range map[string]struct {
		features []string
		pickers  int
	}{
		"neither":      {base, 1},
		"no roles":     {append(base[:4:4], FeatureAddressed), 2},
		"no addressed": {append(base[:4:4], FeatureDataRoles), 2},
	} {
		t.Run(name, func(t *testing.T) {
			out, _, changed := enforceRestriction([]byte(doc), NewClientRestriction(all, nil, tc.features))
			if !changed {
				t.Fatal("must change")
			}
			if _, err := dsl.Parse(out); err != nil {
				t.Fatalf("the enforced DSL must stay valid: %v\n%s", err, out)
			}
			if got := strings.Count(string(out), `"type":"picker"`); got != tc.pickers {
				t.Fatalf("pickers = %d, want %d\n%s", got, tc.pickers, out)
			}
		})
	}
}

// A rule to the recipients stays valid for a build without addressed messages.
func TestUnaddressRecipients(t *testing.T) {
	root := map[string]any{"serverRules": []any{map[string]any{"do": []any{map[string]any{"notify": map[string]any{
		"to": map[string]any{"kind": dsl.RecipientRecipients, "field": "to"}}}}}}}
	if n := unaddress(root, false); n != 0 {
		t.Fatalf("kept for a build with addressed messages: %d", n)
	}
	if n := unaddress(root, true); n != 1 {
		t.Fatalf("changed %d", n)
	}
	rule := root["serverRules"].([]any)[0].(map[string]any)
	to := rule["do"].([]any)[0].(map[string]any)["notify"].(map[string]any)["to"].(map[string]any)
	if to["kind"] != dsl.RecipientAuthor || to["field"] != nil {
		t.Fatalf("to: %v", to)
	}
}
