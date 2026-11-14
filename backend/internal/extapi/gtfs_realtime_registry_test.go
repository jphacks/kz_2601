package extapi

import (
	"slices"
	"testing"
	"time"
)

// What init registers for gtfsRealtime: the ops, their args, off by default.
func TestGTFSRealtimeRegistered_S7Backend004(t *testing.T) {
	p, ok := Lookup(GTFSRealtime)
	if !ok {
		t.Fatal("gtfsRealtime not registered")
	}
	if p.ID != "gtfsRealtime" || p.Billing != BillingFree || p.NeedsOAuth || p.Fake {
		t.Fatalf("provider = id %q billing %q oauth %t fake %t", p.ID, p.Billing, p.NeedsOAuth, p.Fake)
	}
	if !slices.Equal(p.Hosts, gtfsHosts()) || len(p.Hosts) == 0 {
		t.Fatalf("hosts = %v, want %v", p.Hosts, gtfsHosts())
	}
	if p.EnvName() != "GTFS_REALTIME" || p.FeatureKey() != "externalCall.gtfsRealtime" {
		t.Fatalf("env %q feature %q", p.EnvName(), p.FeatureKey())
	}

	want := []struct {
		name string
		args []string
	}{
		{"nextBuses", []string{"feed", "stop", "route", "limit"}},
		{"busPositions", []string{"feed", "route", "limit"}},
	}
	if len(p.Ops) != len(want) {
		t.Fatalf("ops = %d, want %d", len(p.Ops), len(want))
	}
	for i, w := range want {
		o := p.Ops[i]
		if o.Name != w.name || !slices.Equal(o.Args, w.args) {
			t.Fatalf("op %d = %s %v, want %s %v", i, o.Name, o.Args, w.name, w.args)
		}
		if o.Write || len(o.Scopes) != 0 || o.Check == nil || o.CacheTTL != 30*time.Second || o.CacheTTLFor != nil {
			t.Fatalf("op %s = %+v", o.Name, o)
		}
		if _, ok := p.Op(w.name); !ok {
			t.Fatalf("Op(%s) not found", w.name)
		}
	}
	if _, ok := p.Op("tripUpdates"); ok {
		t.Fatal("an op that is not listed was found")
	}
}

func TestGTFSRealtimeRegistered_S7Backend004_OffByDefault(t *testing.T) {
	cfg, err := ConfigFromEnv(func(string) string { return "" })
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Enabled[GTFSRealtime] {
		t.Fatal("gtfsRealtime must be off without EXT_GTFS_REALTIME_ENABLED")
	}
	cfg, err = ConfigFromEnv(func(k string) string {
		return map[string]string{"EXT_GTFS_REALTIME_ENABLED": "1"}[k]
	})
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.Enabled[GTFSRealtime] {
		t.Fatal("EXT_GTFS_REALTIME_ENABLED=1 must turn it on")
	}
}

// New builds one operation per op without a key (free, no secrets) and
// without any request.
func TestGTFSRealtimeRegistered_S7Backend004_New(t *testing.T) {
	p, _ := Lookup(GTFSRealtime)
	ops, err := p.New(Secrets{})
	if err != nil {
		t.Fatal(err)
	}
	for _, o := range p.Ops {
		if ops[o.Name] == nil {
			t.Fatalf("New has no operation for %s", o.Name)
		}
	}
	if len(ops) != len(p.Ops) {
		t.Fatalf("New = %d ops, want %d", len(ops), len(p.Ops))
	}
}
