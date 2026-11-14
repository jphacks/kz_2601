package store

import (
	"encoding/json"
	"reflect"
	"testing"
	"time"
)

func base2773() time.Time { return time.Date(2026, 3, 1, 12, 34, 56, 0, time.UTC) }

func catalogDocAllFields2773() catalogDoc {
	base := base2773()
	return catalogDoc{
		OwnerUID:     "uid1",
		Name:         "家計簿",
		Description:  "毎日の出費",
		Tags:         []string{"money", "life"},
		AuthorName:   "あんな",
		Components:   []string{"text", "number"},
		Version:      7,
		DSLJSON:      `{"v":7}`,
		Installs:     42,
		CreatedAt:    base.Add(-48 * time.Hour),
		UpdatedAt:    base.Add(-time.Hour),
		Hidden:       true,
		HiddenReason: "通報",
		HiddenAt:     base,
	}
}

func TestToCatalogWithDSL(t *testing.T) {
	d := catalogDocAllFields2773()
	base := base2773()
	want := &CatalogApp{
		ID: "APP1", OwnerUID: "uid1", Name: "家計簿", Description: "毎日の出費",
		Tags: []string{"money", "life"}, AuthorName: "あんな",
		Components: []string{"text", "number"}, Version: 7,
		DSL: json.RawMessage(`{"v":7}`), Installs: 42,
		CreatedAt: base.Add(-48 * time.Hour), UpdatedAt: base.Add(-time.Hour),
		ModerationState: ModerationState{Hidden: true, HiddenReason: "通報", HiddenAt: base},
	}
	if got := d.toCatalog("APP1", true); !reflect.DeepEqual(got, want) {
		t.Errorf("toCatalog(APP1, true) = %#v, want %#v", got, want)
	}
}

func TestToCatalogWithoutDSLLeavesDSLEmpty(t *testing.T) {
	d := catalogDocAllFields2773()
	got := d.toCatalog("APP1", false)
	if len(got.DSL) != 0 {
		t.Errorf("toCatalog(APP1, false).DSL = %q, want empty", got.DSL)
	}
	withDSL := d.toCatalog("APP1", true)
	got.DSL = withDSL.DSL
	if !reflect.DeepEqual(got, withDSL) {
		t.Errorf("DSL 以外が withDSL と違う: %#v, want %#v", got, withDSL)
	}
}

func TestToCatalogEmptyDoc(t *testing.T) {
	d := catalogDoc{}
	want := &CatalogApp{ID: "APP2"}
	got := d.toCatalog("APP2", false)
	if !reflect.DeepEqual(got, want) {
		t.Errorf("toCatalog(APP2, false) = %#v, want %#v", got, want)
	}
	if got := d.toCatalog("APP2", true); len(got.DSL) != 0 {
		t.Errorf("dslJson が空のとき DSL = %q, want empty", got.DSL)
	}
}

func TestToRelease(t *testing.T) {
	base := base2773()
	for _, tc := range []struct {
		name string
		id   string
		doc  releaseDoc
		want *ReleaseRequest
	}{
		{
			name: "全部の欄",
			id:   "rel1",
			doc: releaseDoc{AppID: "APP1", OwnerUID: "uid1", Name: "家計簿", Version: 7,
				DSLJSON: `{"v":7}`, Purpose: "公開したい", Contact: "a@example.com",
				Status: ReleaseReceived, CreatedAt: base.Add(-time.Hour), UpdatedAt: base},
			want: &ReleaseRequest{ID: "rel1", AppID: "APP1", OwnerUID: "uid1", Name: "家計簿", Version: 7,
				Purpose: "公開したい", Contact: "a@example.com", Status: ReleaseReceived,
				CreatedAt: base.Add(-time.Hour), UpdatedAt: base},
		},
		{
			name: "欄が空",
			id:   "rel2",
			doc:  releaseDoc{},
			want: &ReleaseRequest{ID: "rel2"},
		},
	} {
		if got := tc.doc.toRelease(tc.id); !reflect.DeepEqual(got, tc.want) {
			t.Errorf("%s: toRelease(%q) = %#v, want %#v", tc.name, tc.id, got, tc.want)
		}
	}
}

func TestToReleaseNeverReturnsDSL(t *testing.T) {
	d := releaseDoc{DSLJSON: `{"v":7}`}
	if got := d.toRelease("rel1"); len(got.DSL) != 0 {
		t.Errorf("toRelease(rel1).DSL = %q, want empty", got.DSL)
	}
}

func feedbackDocAllFields2773() feedbackDoc {
	return feedbackDoc{
		UID: "uid1", Kind: "bug", Message: "落ちた", Untrusted: true,
		ConsentVersion: "v1", Status: FeedbackReceived,
		Attachments: []string{"screenshot", "dsl", "device"},
		AppID:       "APP1", DSLVersion: 7, DSLJSON: `{"v":7}`,
		Screenshot: []byte{0xff, 0xd8, 0xff},
		Device:     &FeedbackDevice{AppVersion: "1.0", Build: "10", OS: "iOS 18", Model: "iPhone", Locale: "ja_JP"},
		Day:        "2026-03-01", CreatedAt: base2773(),
	}
}

func TestToFeedbackWithBodies(t *testing.T) {
	d := feedbackDocAllFields2773()
	want := &Feedback{
		ID: "fb1", UID: "uid1", Kind: "bug", Message: "落ちた", ConsentVersion: "v1",
		Status: FeedbackReceived, AppID: "APP1", DSLVersion: 7,
		DSL: json.RawMessage(`{"v":7}`), Screenshot: []byte{0xff, 0xd8, 0xff},
		Device: d.Device, Day: "2026-03-01", CreatedAt: base2773(),
	}
	if got := d.toFeedback("fb1", true); !reflect.DeepEqual(got, want) {
		t.Errorf("toFeedback(fb1, true) = %#v, want %#v", got, want)
	}
}

func TestToFeedbackWithoutBodiesLeavesDSLAndScreenshotEmpty(t *testing.T) {
	d := feedbackDocAllFields2773()
	got := d.toFeedback("fb1", false)
	if len(got.DSL) != 0 || len(got.Screenshot) != 0 {
		t.Errorf("toFeedback(fb1, false): DSL = %q, Screenshot = %v, want both empty", got.DSL, got.Screenshot)
	}
	withBodies := d.toFeedback("fb1", true)
	got.DSL, got.Screenshot = withBodies.DSL, withBodies.Screenshot
	if !reflect.DeepEqual(got, withBodies) {
		t.Errorf("本文以外が withBodies と違う: %#v, want %#v", got, withBodies)
	}
}

func TestToFeedbackWithBodiesEmptyDSLJSON(t *testing.T) {
	d := feedbackDoc{UID: "uid1", DSLJSON: ""}
	if got := d.toFeedback("fb1", true); got.DSL != nil {
		t.Errorf("dslJson が空のとき DSL = %#v, want nil", got.DSL)
	}
}

func TestToFeedbackDoesNotFillListed(t *testing.T) {
	d := feedbackDocAllFields2773()
	for _, withBodies := range []bool{false, true} {
		if got := d.toFeedback("fb1", withBodies); got.Listed != nil {
			t.Errorf("toFeedback(fb1, %v).Listed = %#v, want nil", withBodies, got.Listed)
		}
	}
}

func TestToFeedbackEmptyDoc(t *testing.T) {
	d := feedbackDoc{}
	want := &Feedback{ID: "fb2"}
	if got := d.toFeedback("fb2", false); !reflect.DeepEqual(got, want) {
		t.Errorf("toFeedback(fb2, false) = %#v, want %#v", got, want)
	}
}
