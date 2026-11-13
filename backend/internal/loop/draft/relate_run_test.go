package draft

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jphacks/kz_2601/backend/internal/loop"
)

func TestBuilderListsRelatedIssues(t *testing.T) {
	ctx := context.Background()
	s := loop.NewMemory()
	triaged(t, s, 1, "photo-add-crash", "bug", "写真を追加すると落ちる", "写真を追加するとアプリが終了する。", loop.SourceInApp)
	triaged(t, s, 1, "dark-mode", "feature", "暗い色にしたい", "暗い配色の切り替えの要望。", loop.SourceInApp)
	calls := 0
	b := &Builder{Store: s, Now: func() time.Time { return now }, RelatedRepo: "o/r",
		OpenIssues: func(context.Context) ([]IssueTitle, error) {
			calls++
			return []IssueTitle{{12, "写真を追加するとアプリが終了する"}, {13, "通知が来ない"}}, nil
		}}
	res, err := b.Run(ctx)
	if err != nil || res.Created != 2 || res.RelatedErr != nil {
		t.Fatalf("%+v %v", res, err)
	}
	if calls != 1 {
		t.Errorf("read %d times, want once per run", calls)
	}
	ds, _ := s.ListDrafts(ctx, loop.DraftQuery{Status: loop.DraftPending})
	for _, d := range ds {
		switch d.ClusterKey {
		case "photo-add-crash":
			if len(d.Related) != 1 || d.Related[0] != (loop.RelatedIssue{Repo: "o/r", Number: 12, Title: "写真を追加するとアプリが終了する"}) {
				t.Errorf("related %+v", d.Related)
			}
		case "dark-mode":
			if len(d.Related) != 0 {
				t.Errorf("related %+v", d.Related)
			}
		}
		if strings.Contains(d.Title+d.Body, "#12") || strings.Contains(d.Body, "o/r") {
			t.Errorf("related issues must not go into what is posted:\n%s", d.Body)
		}
	}
}

func TestBuilderDraftsWhenIssuesCannotBeRead(t *testing.T) {
	ctx := context.Background()
	s := loop.NewMemory()
	triaged(t, s, 1, "photo-add-crash", "bug", "写真を追加すると落ちる", "写真を追加するとアプリが終了する。", loop.SourceInApp)
	b := &Builder{Store: s, Now: func() time.Time { return now }, RelatedRepo: "o/r",
		OpenIssues: func(context.Context) ([]IssueTitle, error) { return nil, errors.New("github: status 403") }}
	res, err := b.Run(ctx)
	if err != nil || res.Created != 1 || res.RelatedErr == nil {
		t.Fatalf("%+v %v", res, err)
	}
}

func TestLoopIssueNumbers(t *testing.T) {
	ctx := context.Background()
	s := loop.NewMemory()
	for i, d := range []loop.Draft{
		{ClusterKey: "a", Status: loop.DraftPublished, IssueRepo: "o/r", IssueNumber: 5},
		{ClusterKey: "a", Status: loop.DraftPublished, IssueRepo: "o/r", IssueNumber: 5},
		{ClusterKey: "b", Status: loop.DraftPublished, IssueRepo: "x/y", IssueNumber: 6},
		{ClusterKey: "c", Status: loop.DraftPending},
	} {
		d.ID, d.CreatedAt, d.ExpiresAt = string(rune('a'+i)), now, now.Add(loop.Retention)
		if err := s.PutDraft(ctx, d); err != nil {
			t.Fatal(err)
		}
	}
	got, err := LoopIssueNumbers(ctx, s, "o/r")
	if err != nil || len(got) != 1 || got[0] != 5 {
		t.Fatalf("%v %v", got, err)
	}
}
