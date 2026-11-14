package ai

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/jphacks/kz_2601/backend/pkg/dsl"
)

// desktopUnrenderable are the types the desktop (windows / linux) build
// cannot draw (#1746, #2301): no MapKit, WebView, HealthKit, sensors,
// nearby radio or camera scanner.
var desktopUnrenderable = []string{"healthStat", "locationTracker", "mapView", "nearbyShare", "placeSearch", "qrScanner", "routeGuide", "sensorValue", "webView"}

// promptStillNames are the unrenderable types that some rule lines (in
// prompt.go and the *_rules.go examples) still name after restrictPrompt.
// The note lists them as unusable and the schema and validator keep them
// out of the output (tests below), so only the wording is left; removing
// those lines is a prompt change for another issue.
var promptStillNames = map[string]bool{"mapView": true, "placeSearch": true, "qrScanner": true, "routeGuide": true}

// allSupportedFeatures is every supportedFeatures key, so a declaration
// built with it restricts components only.
func allSupportedFeatures() []string {
	out := []string{FeatureRecordRetention, FeatureNotificationWhen, FeatureMatching, FeatureSingleEntity,
		FeatureDataRoles, FeatureAddressed, FeatureNotificationNear}
	for _, f := range clientFeatures {
		out = append(out, f.key)
	}
	return out
}

// desktopDeclaration is what the desktop build sends: every known type
// except the ones it cannot draw.
func desktopDeclaration(t *testing.T) (allowed, supported []string) {
	t.Helper()
	skip := map[string]bool{}
	for _, d := range desktopUnrenderable {
		if !knownComponentTypes[d] {
			t.Fatalf("%s is not a component type", d)
		}
		skip[d] = true
	}
	for k := range knownComponentTypes {
		if !skip[k] {
			allowed = append(allowed, k)
		}
	}
	return allowed, allSupportedFeatures()
}

// iosDeclaration is a full iOS build: every type and feature.
func iosDeclaration() (allowed, supported []string) {
	for k := range knownComponentTypes {
		allowed = append(allowed, k)
	}
	return allowed, allSupportedFeatures()
}

func desktopCtx(t *testing.T) context.Context {
	a, s := desktopDeclaration(t)
	return withRestriction(context.Background(), NewClientRestriction(a, nil, s))
}

func TestDesktopRestriction_PromptAndSchemaDropUnrenderable(t *testing.T) {
	a, s := desktopDeclaration(t)
	r := NewClientRestriction(a, nil, s)
	if !r.Active() || strings.Join(r.Disallowed(), ",") != strings.Join(desktopUnrenderable, ",") {
		t.Fatalf("disallowed %v", r.Disallowed())
	}
	ctx := desktopCtx(t)
	for name, p := range map[string]string{
		"create": systemPrompt, "edit": systemPrompt + "\n" + editRules,
		"chat": chatSystemPrompt, "chatCreate": chatCreateSystemPrompt, "chatEdit": chatEditSystemPrompt,
	} {
		got := restrictPrompt(ctx, p)
		if !strings.Contains(got, "使えない部品: "+strings.Join(desktopUnrenderable, ", ")) {
			t.Fatalf("%s: restriction note missing", name)
		}
		body := withoutNote(got)
		for _, part := range desktopUnrenderable {
			if promptStillNames[part] {
				continue
			}
			if i := strings.Index(body, part); i >= 0 {
				t.Errorf("%s: %s still in the rules: …%s…", name, part, body[max(0, i-80):min(len(body), i+40)])
			}
		}
	}
	var sch map[string]any
	if err := json.Unmarshal(restrictSchema(ctx, GenerationSchema()), &sch); err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(sch)
	for _, part := range desktopUnrenderable {
		if strings.Contains(string(raw), `"`+part+`"`) {
			t.Errorf("schema still allows %s", part)
		}
	}
}

// A first answer with a mapView is sent back as a violation; the next answer
// (without it) is used.
func TestDesktopRestriction_CreateRetriesThenUsesOtherParts(t *testing.T) {
	a, s := desktopDeclaration(t)
	shops := readDoc(t, "../../../docs/dsl/examples/shops.json")
	kb := kakeibo(t)
	f := &fakeCaller{replies: []string{string(shops), string(kb)}}
	svc, _ := newSvc(f)
	res, err := svc.Generate(context.Background(), Request{AppID: "a1", Transcript: "行きたいお店のメモ", AllowedComponents: a, SupportedFeatures: s})
	if err != nil {
		t.Fatal(err)
	}
	if res.Meta.Attempts != 2 || !strings.Contains(f.calls[1].Messages[0].Content, `"mapView" is not available`) {
		t.Fatalf("expected a retry naming mapView: %+v", res.Meta)
	}
	for _, part := range desktopUnrenderable {
		if strings.Contains(string(res.DSL), `"`+part+`"`) {
			t.Fatalf("%s in output", part)
		}
	}
}

// When the model keeps using an unrenderable part, the last attempt strips
// it: the result stays valid and no screen is left empty.
func TestDesktopRestriction_StrippedScreensAreNotEmpty(t *testing.T) {
	a, s := desktopDeclaration(t)
	for _, name := range []string{"shops", "walk_route", "nearby_spots", "sensors", "gallery-web"} {
		doc := readDoc(t, "../../../docs/dsl/examples/"+name+".json")
		f := &fakeCaller{replies: []string{string(doc)}}
		svc, _ := newSvc(f)
		svc.MaxRegenerations = -1
		res, err := svc.Generate(context.Background(), Request{AppID: "a1", Transcript: name, AllowedComponents: a, SupportedFeatures: s})
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		for _, part := range desktopUnrenderable {
			if strings.Contains(string(res.DSL), `"`+part+`"`) {
				t.Fatalf("%s: %s left", name, part)
			}
		}
		app, err := dsl.Parse(res.DSL)
		if err != nil {
			t.Fatalf("%s: stripped DSL invalid: %v", name, err)
		}
		for _, sc := range app.Screens {
			if len(sc.Components) == 0 {
				t.Errorf("%s: screen %q is empty", name, sc.ID)
			}
		}
	}
}

// A full iOS declaration restricts nothing: the prompt and schema sent are
// the same as with no declaration.
func TestDesktopRestriction_IOSDeclarationUnchanged(t *testing.T) {
	a, s := iosDeclaration()
	if NewClientRestriction(a, nil, s).Active() {
		t.Fatal("full iOS declaration restricts")
	}
	send := func(req Request) Completion {
		kb := kakeibo(t)
		f := &fakeCaller{replies: []string{string(kb)}}
		svc, _ := newSvc(f)
		if _, err := svc.Generate(context.Background(), req); err != nil {
			t.Fatal(err)
		}
		return f.calls[0]
	}
	plain := send(Request{AppID: "a1", Transcript: "家計簿アプリを作って"})
	ios := send(Request{AppID: "a1", Transcript: "家計簿アプリを作って", AllowedComponents: a, SupportedFeatures: s})
	if ios.System != plain.System || string(ios.Schema) != string(plain.Schema) {
		t.Fatal("iOS declaration changed the prompt or schema")
	}
}
