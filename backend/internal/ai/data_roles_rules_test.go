package ai

import (
	"context"
	"strings"
	"testing"

	"github.com/jphacks/kz_2601/backend/pkg/dsl"
)

// #272 #273: the prompts teach admins and unique, and no longer say roles are impossible.
func TestPromptTeachesDataRoles(t *testing.T) {
	for _, want := range []string{dataRolesRules, editRoleCan, limitRoleCan, `"editableBy":"admins"`, `"unique":["$author"]`} {
		if !strings.Contains(systemPrompt, want) {
			t.Errorf("system prompt lacks %q", want)
		}
	}
	for _, gone := range []string{editRoleCannot, limitRoleCannot} {
		if strings.Contains(systemPrompt, gone) {
			t.Errorf("system prompt still says roles are impossible: %q", gone)
		}
	}
	for name, p := range map[string]string{"chatCreate": chatCreateSystemPrompt, "chatEdit": chatEditSystemPrompt} {
		if !strings.Contains(p, dataRolesChatRule) || !strings.Contains(p, chatRoleCan) || strings.Contains(p, chatRoleCannot) {
			t.Errorf("%s: chat rule not swapped", name)
		}
		// the other rules files still found their anchor
		if !strings.Contains(p, singleChatRule) || !strings.Contains(p, retentionChatRule) || !strings.Contains(p, matchingChatRule) {
			t.Errorf("%s lost another chat rule", name)
		}
	}
	// entity / field keys reach the model through the generation schema, button props through the catalog
	for _, k := range []string{`"editableBy"`, `"deleteBy"`, `"unique"`, `"visibleTo"`, `"member"`} {
		if !strings.Contains(string(generationSchema), k) {
			t.Errorf("generation schema lacks %s", k)
		}
	}
	if !strings.Contains(catalog, "visibleTo") {
		t.Error("catalog lacks button visibleTo")
	}
}

// rolesDoc uses every part of #272 #273 (a member field, visibleTo, editableBy,
// deleteBy and unique on $author).
const rolesDoc = `{"schemaVersion":1,"app":{"name":"シフト","voiceEntry":"none"},"initialScreen":"home",
"entities":[{"id":"shifts","name":"シフト","scope":"shared","deleteBy":"admins","unique":["day","$author"],"fields":[
 {"id":"day","name":"日","type":"date"},{"id":"staff","name":"担当","type":"member"},
 {"id":"confirmed","name":"確定","type":"boolean","editableBy":"admins"}]}],
"screens":[{"id":"home","title":"シフト","components":[
 {"type":"list","entity":"shifts","item":{"title":"day"}},
 {"type":"button","label":"確定","visibleTo":"admins","action":{"type":"navigate","screen":"home"}}]}]}`

func allComponentTypes() []string {
	all := make([]string, 0, len(knownComponentTypes))
	for k := range knownComponentTypes {
		all = append(all, k)
	}
	return all
}

// Builds without dataRoles: no member / visibleTo in the schema, prompt or output.
func TestDataRolesRestriction(t *testing.T) {
	if _, err := dsl.Parse([]byte(rolesDoc)); err != nil {
		t.Fatalf("rolesDoc must be valid: %v", err)
	}
	all := allComponentTypes()
	old := NewClientRestriction(all, nil, []string{FeatureRecordRetention, FeatureNotificationWhen, FeatureMatching, FeatureSingleEntity})
	if !old.Active() {
		t.Fatal("a build without dataRoles must be restricted")
	}
	sch := string(old.assets().genSch)
	if strings.Contains(sch, `"visibleTo"`) || strings.Contains(sch, `"member"`) {
		t.Fatal("restricted generation schema still has member / visibleTo")
	}
	if !strings.Contains(sch, `"editableBy"`) || !strings.Contains(sch, `"deleteBy"`) {
		t.Fatal("server-enforced rules stay for older builds")
	}
	ctx := withRestriction(context.Background(), old)
	p := restrictPrompt(ctx, systemPrompt)
	if strings.Contains(p, dataRolesMember) || strings.Contains(p, dataRolesButton) || !strings.Contains(p, "- 重複の防止:") {
		t.Fatal("restricted prompt: member / visibleTo lines must go, unique stays")
	}
	out, notes, changed := enforceRestriction([]byte(rolesDoc), old)
	if !changed || len(notes) != 1 || strings.Contains(string(out), `"visibleTo"`) || strings.Contains(string(out), `"member"`) {
		t.Fatalf("enforce: changed=%v notes=%v out=%s", changed, notes, out)
	}
	if _, err := dsl.Parse(out); err != nil {
		t.Fatalf("the enforced DSL must stay valid: %v", err)
	}

	cur := NewClientRestriction(all, nil, []string{FeatureRecordRetention, FeatureNotificationWhen, FeatureMatching, FeatureSingleEntity, FeatureDataRoles, FeatureAddressed})
	if cur.Active() {
		t.Fatal("a current build must stay unrestricted")
	}
}

// Builds without shared data: the roles go with the scope, unique on fields stays.
func TestDataRolesWithoutSharedData(t *testing.T) {
	all := allComponentTypes()
	r := NewClientRestriction(all, []string{FeatureSharedData}, []string{FeatureRecordRetention, FeatureNotificationWhen, FeatureMatching, FeatureSingleEntity, FeatureDataRoles, FeatureAddressed})
	sch := string(r.assets().genSch)
	if strings.Contains(sch, `"editableBy"`) || strings.Contains(sch, `"deleteBy"`) || strings.Contains(sch, `"visibleTo"`) {
		t.Fatal("no shared data: admin rules must leave the schema")
	}
	p := restrictPrompt(withRestriction(context.Background(), r), systemPrompt)
	if strings.Contains(p, dataRolesRules) || !strings.Contains(p, dataRolesPrivate) {
		t.Fatal("no shared data: the roles block must become the private unique line")
	}
	out, _, changed := enforceRestriction([]byte(rolesDoc), r)
	if !changed {
		t.Fatal("enforce did nothing")
	}
	for _, gone := range []string{`"editableBy"`, `"deleteBy"`, `"visibleTo"`, `"$author"`, `"member"`, `"scope"`} {
		if strings.Contains(string(out), gone) {
			t.Fatalf("enforced output still has %s: %s", gone, out)
		}
	}
	if !strings.Contains(string(out), `"unique":["day"]`) {
		t.Fatalf("unique on a field stays: %s", out)
	}
	if _, err := dsl.Parse(out); err != nil {
		t.Fatalf("the enforced DSL must stay valid: %v", err)
	}
}

// The "others confirm" omission does not fire when an admin-only field answers it.
func TestDataRolesOmission(t *testing.T) {
	a, err := dsl.Parse([]byte(rolesDoc))
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range omissionRules {
		if r.id == "shared_update" && r.omitted("店長だけが確定できるシフト表", a) {
			t.Fatal("an editableBy admins field is the requested confirmation")
		}
	}
}
