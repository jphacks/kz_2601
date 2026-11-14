package store

import (
	"context"
	"fmt"

	"cloud.google.com/go/firestore"
	firestorepb "cloud.google.com/go/firestore/apiv1/firestorepb"
)

var _ Counter = (*Firestore)(nil)

// CountApps implements Counter with a count aggregation (1 read per 1000 apps).
func (f *Firestore) CountApps(ctx context.Context, uid string) (int, error) {
	return countQuery(ctx, f.heldApps(uid))
}

// heldApps is uid's held apps (currentVersion > HeldVersion; the automatic
// single-field index serves it).
func (f *Firestore) heldApps(uid string) firestore.Query {
	return f.c.Collection("users").Doc(uid).Collection("apps").Where("currentVersion", ">", HeldVersion)
}

// CountRecords implements Counter with a count aggregation.
func (f *Firestore) CountRecords(ctx context.Context, rs RecordSet, field string) (int, error) {
	q := f.recordsCol(rs).Query
	if field != "" {
		q = q.WherePath(firestore.FieldPath{"data", field}, "!=", nil)
	}
	return countQuery(ctx, q)
}

func countQuery(ctx context.Context, q firestore.Query) (int, error) {
	return countAgg(ctx, q.NewAggregationQuery())
}

func countAgg(ctx context.Context, a *firestore.AggregationQuery) (int, error) {
	res, err := a.WithCount("n").Get(ctx)
	if err != nil {
		return 0, err
	}
	v, ok := res["n"].(*firestorepb.Value)
	if !ok {
		return 0, fmt.Errorf("store: count: unexpected result %T", res["n"])
	}
	return int(v.GetIntegerValue()), nil
}
