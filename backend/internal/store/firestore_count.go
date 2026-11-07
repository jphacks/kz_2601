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
	return countQuery(ctx, f.c.Collection("users").Doc(uid).Collection("apps").Query)
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
	res, err := q.NewAggregationQuery().WithCount("n").Get(ctx)
	if err != nil {
		return 0, err
	}
	v, ok := res["n"].(*firestorepb.Value)
	if !ok {
		return 0, fmt.Errorf("store: count: unexpected result %T", res["n"])
	}
	return int(v.GetIntegerValue()), nil
}
