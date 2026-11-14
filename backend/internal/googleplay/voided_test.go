package googleplay

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"google.golang.org/api/option"
)

const voidedPath = "/androidpublisher/v3/applications/jp.kz2601.metamon/purchases/voidedpurchases"

func voidedClient(t *testing.T, h http.HandlerFunc) *Client {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	c, err := NewWithOptions(context.Background(), option.WithEndpoint(srv.URL+"/"), option.WithoutAuthentication(), option.WithHTTPClient(srv.Client()))
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestVoidedPurchasesPaging(t *testing.T) {
	since := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	var queries []string
	c := voidedClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != voidedPath {
			http.Error(w, `{"error":{"code":500,"message":"boom"}}`, http.StatusInternalServerError)
			return
		}
		q := r.URL.Query()
		queries = append(queries, q.Encode())
		w.Header().Set("Content-Type", "application/json")
		switch q.Get("token") {
		case "":
			_, _ = w.Write([]byte(`{"voidedPurchases":[
				{"purchaseToken":"t1","orderId":"GPA.1","voidedTimeMillis":"1762300800000","voidedReason":1},
				{"purchaseToken":"t2","orderId":"GPA.2..0","voidedTimeMillis":"1762304400000","voidedReason":7}],
				"tokenPagination":{"nextPageToken":"p2"}}`))
		case "p2":
			_, _ = w.Write([]byte(`{"voidedPurchases":[
				{"purchaseToken":"t3","orderId":"GPA.3","voidedTimeMillis":"1762308000000","voidedReason":0}]}`))
		default:
			http.Error(w, `{"error":{"code":400,"message":"bad token"}}`, http.StatusBadRequest)
		}
	})

	var got []VoidedPurchase
	token := ""
	for pages := 0; ; pages++ {
		if pages > 2 {
			t.Fatal("paging does not stop")
		}
		p, err := c.VoidedPurchases(context.Background(), "jp.kz2601.metamon", since, token)
		if err != nil {
			t.Fatal(err)
		}
		got = append(got, p.Purchases...)
		if p.NextPageToken == "" {
			break
		}
		token = p.NextPageToken
	}

	want := []VoidedPurchase{
		{Token: "t1", OrderID: "GPA.1", VoidedTime: time.UnixMilli(1762300800000).UTC(), Reason: 1},
		{Token: "t2", OrderID: "GPA.2..0", VoidedTime: time.UnixMilli(1762304400000).UTC(), Reason: 7},
		{Token: "t3", OrderID: "GPA.3", VoidedTime: time.UnixMilli(1762308000000).UTC(), Reason: 0},
	}
	if len(got) != len(want) {
		t.Fatalf("got %d purchases: %+v", len(got), got)
	}
	for i := range want {
		if !got[i].VoidedTime.Equal(want[i].VoidedTime) || got[i].Token != want[i].Token ||
			got[i].OrderID != want[i].OrderID || got[i].Reason != want[i].Reason {
			t.Errorf("[%d] = %+v, want %+v", i, got[i], want[i])
		}
	}
	if len(queries) != 2 {
		t.Fatalf("want 2 requests, got %v", queries)
	}
	// 1st page: type=1 and startTime; 2nd: the page token (startTime is ignored by Play, not sent)
	if !strings.Contains(queries[0], "type=1") || !strings.Contains(queries[0], "startTime=1793491200000") {
		t.Errorf("1st query = %s", queries[0])
	}
	if !strings.Contains(queries[1], "token=p2") || !strings.Contains(queries[1], "type=1") || strings.Contains(queries[1], "startTime") {
		t.Errorf("2nd query = %s", queries[1])
	}
}

func TestVoidedPurchasesErrorHasNoToken(t *testing.T) {
	c := voidedClient(t, func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, `{"error":{"code":403,"message":"denied"}}`, http.StatusForbidden)
	})
	_, err := c.VoidedPurchases(context.Background(), "jp.kz2601.metamon", time.Time{}, "PAGE-SECRET")
	if err == nil {
		t.Fatal("want error")
	}
	if strings.Contains(err.Error(), "PAGE-SECRET") || !strings.Contains(err.Error(), "HTTP 403") {
		t.Errorf("err = %v", err)
	}
	if strings.Contains(err.Error(), ErrNotFound.Error()) {
		t.Errorf("list error mapped to ErrNotFound: %v", err)
	}
}
