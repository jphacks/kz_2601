package dsl

import (
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"
)

const quizExample = "../../internal/ai/examples/quiz.json"

func loadQuiz(t *testing.T) *App {
	t.Helper()
	raw, err := os.ReadFile(quizExample)
	if err != nil {
		t.Fatal(err)
	}
	app, err := Parse(raw)
	if err != nil {
		t.Fatalf("quiz example must validate: %v", err)
	}
	return app
}

func TestQuizExampleGradesWithHiddenAnswerKey(t *testing.T) {
	app := loadQuiz(t)
	q := app.EntityByID("questions")
	if f := q.FieldByID("correct"); f == nil || !f.Hidden {
		t.Fatalf("questions.correct must be hidden: %+v", f)
	}
	a := app.EntityByID("answers")
	for _, id := range []string{"is_correct", "score"} {
		if f := a.FieldByID(id); f == nil || f.Computed == nil {
			t.Fatalf("answers.%s must be computed", id)
		}
	}
	if !strings.Contains(exprJSON(t, a.FieldByID("is_correct").Computed), `"question.correct"`) {
		t.Fatal("is_correct must compare against the answer key via lookup")
	}
	// round-trip keeps hidden
	out, err := json.Marshal(app)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), `"hidden":true`) {
		t.Fatal("hidden lost on marshal")
	}
}

func exprJSON(t *testing.T, e *Expr) string {
	t.Helper()
	b, err := json.Marshal(e)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// Each mutation shows the answer key to answerers and must be rejected.
func TestHiddenFieldNotShownToAnswerers(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(a *App)
		want   string
	}{
		{"list trailing", func(a *App) {
			a.Screens[0].Components[0].Children[0].Item.Trailing = "correct"
		}, `list item.trailing "correct"`},
		{"table column", func(a *App) {
			c := &a.Screens[1].Components[0].Children[0]
			c.Entity = "questions"
			c.Columns = []TableColumn{{Field: "text"}, {Field: "correct"}}
			c.Sort = nil
		}, `columns[1].field "correct"`},
		{"chart groupBy", func(a *App) {
			a.Screens[1].Components = append(a.Screens[1].Components, Component{
				Type: CompChart, Kind: "pie", Entity: "questions", Aggregate: "count", GroupBy: "correct", Area: "A11:F12",
			})
		}, `groupBy "correct"`},
		{"titleField", func(a *App) {
			a.EntityByID("questions").TitleField = "correct"
		}, `titleField "correct" is hidden`},
		{"computed copies via lookup", func(a *App) {
			e := a.EntityByID("answers")
			e.Fields = append(e.Fields, Field{ID: "answer_key", Name: "正解", Type: FieldString,
				Computed: &Expr{Kind: ExprRef, Ref: "question.correct"}})
		}, `computed shows hidden field questions.correct`},
		{"computed in if branch", func(a *App) {
			e := a.EntityByID("answers")
			e.Fields = append(e.Fields, Field{ID: "hint", Name: "ヒント", Type: FieldString,
				Computed: &Expr{Kind: ExprIf, If: &Expr{Kind: ExprRef, Ref: "is_correct"},
					Then: &Expr{Kind: ExprLiteral, Lit: "正解"}, Else: &Expr{Kind: ExprRef, Ref: "question.correct"}}})
		}, `computed shows hidden field questions.correct`},
		{"hidden computed", func(a *App) {
			a.EntityByID("answers").FieldByID("score").Hidden = true
		}, `computed field cannot be hidden`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			app := loadQuiz(t)
			tc.mutate(app)
			err := app.Validate()
			var ve *ValidationError
			if !errors.As(err, &ve) {
				t.Fatalf("want validation error, got %v", err)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("want %q in\n%v", tc.want, err)
			}
		})
	}
}

// Comparing the answer key (and counting by it) stays allowed.
func TestHiddenFieldComparisonsAllowed(t *testing.T) {
	app := loadQuiz(t)
	q := app.EntityByID("questions")
	q.Fields = append(q.Fields, Field{ID: "is_a", Name: "A が正解", Type: FieldBoolean,
		Computed: &Expr{Kind: ExprOp, Op: "eq", Args: []Expr{{Kind: ExprRef, Ref: "correct"}, {Kind: ExprLiteral, Lit: "A"}}}})
	// inputs on the author's form bind the hidden field: fine
	if err := app.Validate(); err != nil {
		t.Fatal(err)
	}
}
