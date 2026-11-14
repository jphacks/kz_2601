package appconfig

import (
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/jphacks/kz_2601/backend/internal/store"
	"github.com/jphacks/kz_2601/backend/pkg/dsl"
)

func filterTestEntity(scope dsl.EntityScope) *dsl.Entity {
	return &dsl.Entity{ID: "item", Name: "item", Scope: scope, Fields: []dsl.Field{
		{ID: "title", Name: "title", Type: dsl.FieldString},
		{ID: "amount", Name: "amount", Type: dsl.FieldInteger},
		{ID: "price", Name: "price", Type: dsl.FieldNumber},
		{ID: "done", Name: "done", Type: dsl.FieldBoolean},
		{ID: "day", Name: "day", Type: dsl.FieldDate},
		{ID: "secret", Name: "secret", Type: dsl.FieldString, Encrypted: true},
		{ID: "double", Name: "double", Type: dsl.FieldNumber, Computed: dsl.NumExpr(2)},
	}}
}

// #2569: a bad filterField / filterOp / filterValue is a 400 with the reason.
func TestBuildFilterRejectsBadSpec(t *testing.T) {
	private, shared := filterTestEntity(dsl.ScopePrivate), filterTestEntity(dsl.ScopeShared)
	for _, c := range []struct {
		name              string
		ent               *dsl.Entity
		field, op, value  string
		msg, detailsField string
	}{
		{"unknown field", private, "nope", "eq", "x", "unknown filterField", "nope"},
		{"field without op", private, "nope", "", "", "unknown filterField", "nope"},
		{"author on a private entity", private, "author", "eq", "u1", "unknown filterField", "author"},
		{"unknown op", private, "title", "like", "x", "filterOp must be one of", ""},
		{"op without field", private, "", "eq", "x", "unknown filterField", ""},
		{"author with a range op", shared, "author", "gt", "u1", "filterOp for author must be eq or neq", ""},
		{"integer with a word", private, "amount", "gt", "abc", "filterValue must be numeric", ""},
		{"number with an empty value", private, "price", "eq", "", "filterValue must be numeric", ""},
		{"boolean with a word", private, "done", "eq", "maybe", "filterValue must be a boolean", ""},
		{"today on a string", private, "title", "today", "", "today requires a date/datetime field", ""},
		{"thisMonth on a number", private, "amount", "thisMonth", "", "thisMonth requires a date/datetime field", ""},
		{"encrypted field", private, "secret", "eq", "x", "field is encrypted", "secret"},
	} {
		pred, err := buildFilter(c.ent, c.field, c.op, c.value)
		var ae *apiError
		if pred != nil || !errors.As(err, &ae) {
			t.Errorf("%s: pred=%v err=%v", c.name, pred != nil, err)
			continue
		}
		if ae.status != http.StatusBadRequest || ae.code != "bad_request" || !strings.Contains(ae.msg, c.msg) {
			t.Errorf("%s: %d %s %q", c.name, ae.status, ae.code, ae.msg)
		}
		if c.detailsField != "" && ae.details["field"] != c.detailsField {
			t.Errorf("%s: details %v", c.name, ae.details)
		}
	}
}

// #2569: valid specs keep filtering as before.
func TestBuildFilterValidSpecs(t *testing.T) {
	private, shared := filterTestEntity(dsl.ScopePrivate), filterTestEntity(dsl.ScopeShared)
	if pred, err := buildFilter(private, "", "", "ignored"); pred != nil || err != nil {
		t.Fatalf("no filter: %v %v", pred != nil, err)
	}
	today := time.Now().In(jst).Format(dateLayout)
	rec := &store.Record{Author: "u1", Data: map[string]any{
		"title": "りんご", "amount": float64(300), "price": 1.5, "done": true, "day": today,
	}}
	empty := &store.Record{Author: "u2", Data: map[string]any{}}
	for _, c := range []struct {
		name             string
		ent              *dsl.Entity
		field, op, value string
		rec, emptyRec    bool
	}{
		{"string eq", private, "title", "eq", "りんご", true, false},
		{"string neq", private, "title", "neq", "みかん", true, true},
		// a missing value compares as the largest (nil sorts last), so gt /
		// gte keep records without it: the current behavior
		{"integer gt", private, "amount", "gt", "299", true, true},
		{"integer gte", private, "amount", "gte", "300", true, true},
		{"integer lt", private, "amount", "lt", "300", false, false},
		{"integer lte", private, "amount", "lte", "300", true, false},
		{"number eq", private, "price", "eq", "1.5", true, false},
		{"boolean eq", private, "done", "eq", "true", true, false},
		{"date today", private, "day", "today", "", true, false},
		{"date thisMonth", private, "day", "thisMonth", "", true, false},
		{"author eq", shared, "author", "eq", "u1", true, false},
		{"author neq", shared, "author", "neq", "u1", false, true},
		// a computed field is accepted; its value is not stored, so eq never
		// matches and neq always does
		{"computed eq", private, "double", "eq", "2", false, false},
		{"computed neq", private, "double", "neq", "2", true, true},
	} {
		pred, err := buildFilter(c.ent, c.field, c.op, c.value)
		if err != nil || pred == nil {
			t.Errorf("%s: %v", c.name, err)
			continue
		}
		if got, gotEmpty := pred(rec), pred(empty); got != c.rec || gotEmpty != c.emptyRec {
			t.Errorf("%s: rec=%v empty=%v", c.name, got, gotEmpty)
		}
	}
}

// #2569: recordValue reads the timestamps and data fields the sort and
// filter compare.
func TestRecordValueFields(t *testing.T) {
	created := time.Date(2026, 1, 2, 3, 4, 5, 6, time.UTC)
	updated := created.Add(time.Hour)
	rec := &store.Record{CreatedAt: created, UpdatedAt: updated, Data: map[string]any{"title": "x", "createdAt": "data"}}
	for _, c := range []struct {
		field string
		want  any
	}{
		{"createdAt", created.Format(time.RFC3339Nano)},
		{"updatedAt", updated.Format(time.RFC3339Nano)},
		{"title", "x"},
		{"missing", nil},
	} {
		if got := recordValue(rec, c.field); got != c.want {
			t.Errorf("%s: %v, want %v", c.field, got, c.want)
		}
	}
	if got := recordValue(&store.Record{}, "title"); got != nil {
		t.Errorf("nil data: %v", got)
	}
}
