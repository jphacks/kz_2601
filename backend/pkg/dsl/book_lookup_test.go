package dsl

import (
	"encoding/json"
	"strings"
	"testing"
)

// bookApp: a lending app's book form with the lookup button on it.
func bookApp(act Action) App {
	app := Initial()
	app.Entities = []Entity{{ID: "book", Name: "本", TitleField: "title", Fields: []Field{
		{ID: "isbn", Name: "ISBN", Type: FieldString},
		{ID: "title", Name: "書名", Type: FieldString, Required: true},
		{ID: "author", Name: "著者", Type: FieldString},
		{ID: "publisher", Name: "出版社", Type: FieldString},
		{ID: "published", Name: "出版日", Type: FieldDate},
		{ID: "memo", Name: "メモ", Type: FieldText},
		{ID: "pages", Name: "ページ数", Type: FieldInteger},
	}}}
	app.Screens[0].Components = []Component{{Type: CompList, Entity: "book", Item: &ListItem{Title: "title"},
		OnItemTap: &Action{Type: ActionNavigate, Screen: "book_form"}}}
	app.Screens = append(app.Screens, Screen{ID: "book_form", Title: "本", Entity: "book", Components: []Component{
		{Type: CompQrScanner, Field: "isbn", Label: "バーコードを読む"},
		{Type: CompTextField, Field: "isbn"},
		{Type: CompButton, Label: "ISBN から入れる", Action: &act},
		{Type: CompTextField, Field: "title"},
	}, Toolbar: []ToolbarButton{{Label: "保存", Action: Action{Type: ActionSaveRecord, Entity: "book"}}}})
	return app
}

func TestBookLookupValid(t *testing.T) {
	for _, b := range []*BookLookup{
		{ISBNField: "isbn", TitleField: "title"},
		{ISBNField: "isbn", TitleField: "title", AuthorsField: "author", PublisherField: "publisher", PublishedDateField: "published"},
		{ISBNField: "isbn", AuthorsField: "memo", PublishedDateField: "publisher"},
	} {
		app := bookApp(Action{Type: ActionLookupBook, BookLookup: b})
		if err := app.Validate(); err != nil {
			t.Errorf("%+v rejected: %v", b, err)
		}
	}
	// Also from the toolbar of the form.
	app := bookApp(Action{Type: ActionLookupBook, BookLookup: &BookLookup{ISBNField: "isbn", TitleField: "title"}})
	app.Screens[1].Components = app.Screens[1].Components[:2]
	app.Screens[1].Components = append(app.Screens[1].Components, Component{Type: CompTextField, Field: "title"})
	app.Screens[1].Toolbar = append(app.Screens[1].Toolbar, ToolbarButton{Label: "検索", Action: Action{Type: ActionLookupBook, BookLookup: &BookLookup{ISBNField: "isbn", TitleField: "title"}}})
	if err := app.Validate(); err != nil {
		t.Errorf("toolbar rejected: %v", err)
	}
}

func TestBookLookupErrors(t *testing.T) {
	for _, c := range []struct {
		book *BookLookup
		want string
	}{
		{nil, "needs bookLookup"},
		{&BookLookup{TitleField: "title"}, "isbnField is required"},
		{&BookLookup{ISBNField: "nope", TitleField: "title"}, `isbnField "nope" not in entity`},
		{&BookLookup{ISBNField: "pages", TitleField: "title"}, `isbnField "pages" is integer`},
		{&BookLookup{ISBNField: "isbn"}, "maps no field"},
		{&BookLookup{ISBNField: "isbn", TitleField: "pages"}, `titleField "pages" is integer`},
		{&BookLookup{ISBNField: "isbn", TitleField: "published"}, `titleField "published" is date`},
		{&BookLookup{ISBNField: "isbn", TitleField: "title", AuthorsField: "title"}, `maps field "title" twice`},
		{&BookLookup{ISBNField: "isbn", TitleField: "isbn"}, `maps field "isbn" twice`},
	} {
		app := bookApp(Action{Type: ActionLookupBook, BookLookup: c.book})
		err := app.Validate()
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%+v: got %v, want %q", c.book, err, c.want)
		}
	}
}

func TestBookLookupPlacement(t *testing.T) {
	act := &Action{Type: ActionLookupBook, BookLookup: &BookLookup{ISBNField: "isbn", TitleField: "title"}}
	// On a screen without screen.entity.
	app := bookApp(*act)
	app.Screens[0].Components = append(app.Screens[0].Components, Component{Type: CompButton, Label: "x", Action: act})
	if err := app.Validate(); err == nil || !strings.Contains(err.Error(), "lookupBook needs a form screen") {
		t.Errorf("no form screen: %v", err)
	}
	// As a list tap.
	app = bookApp(*act)
	app.Screens[0].Components[0].OnItemTap = act
	if err := app.Validate(); err == nil || !strings.Contains(err.Error(), "not on a tap") {
		t.Errorf("onItemTap: %v", err)
	}
}

func TestBookLookupSchema(t *testing.T) {
	app := bookApp(Action{Type: ActionLookupBook, BookLookup: &BookLookup{ISBNField: "isbn", TitleField: "title", PublishedDateField: "published"}})
	raw, err := json.Marshal(app)
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateSchema(raw); err != nil {
		t.Fatalf("schema rejects lookupBook: %v", err)
	}
	// Only field ids: a url (or any other key) inside bookLookup is refused.
	bad := strings.Replace(string(raw), `"isbnField":"isbn"`, `"isbnField":"isbn","url":"https://evil.example/"`, 1)
	if err := ValidateSchema([]byte(bad)); err == nil {
		t.Fatal("schema accepted an unknown key in bookLookup")
	}
}
