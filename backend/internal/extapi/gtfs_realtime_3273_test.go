package extapi

import (
	"slices"
	"testing"
	"time"
)

func TestGTFSRealtimeRegistered_3273(t *testing.T) {
	p, ok := Lookup(GTFSRealtime)
	if !ok {
		t.Fatal("gtfsRealtime is not registered")
	}
	if p.ID != "gtfsRealtime" || p.Billing != BillingFree {
		t.Fatalf("id/billing = %q/%q", p.ID, p.Billing)
	}
	if p.NeedsOAuth || p.Fake {
		t.Fatalf("NeedsOAuth=%v Fake=%v, want both false", p.NeedsOAuth, p.Fake)
	}
	if got := p.FeatureKey(); got != "externalCall.gtfsRealtime" {
		t.Fatalf("FeatureKey = %q", got)
	}
	if !slices.Equal(p.Hosts, gtfsHosts()) || len(p.Hosts) == 0 {
		t.Fatalf("Hosts = %v", p.Hosts)
	}

	want := map[string][]string{
		"nextBuses":    {"feed", "stop", "route", "limit"},
		"busPositions": {"feed", "route", "limit"},
	}
	if len(p.Ops) != len(want) {
		t.Fatalf("ops = %d, want %d", len(p.Ops), len(want))
	}
	for name, args := range want {
		spec, ok := p.Op(name)
		if !ok {
			t.Fatalf("op %s missing", name)
		}
		if !slices.Equal(spec.Args, args) {
			t.Errorf("%s args = %v, want %v", name, spec.Args, args)
		}
		if spec.Write || len(spec.Scopes) != 0 {
			t.Errorf("%s must be a read op without scopes", name)
		}
		if spec.CacheTTL != 30*time.Second {
			t.Errorf("%s CacheTTL = %v", name, spec.CacheTTL)
		}
		if spec.Check == nil {
			t.Errorf("%s has no Check", name)
		}
	}
	if _, ok := p.Op("nope"); ok {
		t.Error("unknown op found")
	}
}

func TestGTFSRealtimeRegisteredNew_3273(t *testing.T) {
	p, _ := Lookup(GTFSRealtime)
	ops, err := p.New(Secrets{})
	if err != nil {
		t.Fatal(err)
	}
	for _, spec := range p.Ops {
		if ops[spec.Name] == nil {
			t.Errorf("New gave no operation for %s", spec.Name)
		}
	}
	if len(ops) != len(p.Ops) {
		t.Errorf("ops = %d, want %d", len(ops), len(p.Ops))
	}
}

func TestGTFSRealtimeRegisteredCheckArgs_3273(t *testing.T) {
	p, _ := Lookup(GTFSRealtime)
	if _, err := p.CheckArgs("busPositions", map[string]string{"feed": "toeiBus", "limit": "5"}); err != nil {
		t.Fatalf("valid args refused: %v", err)
	}
	if _, err := p.CheckArgs("busPositions", map[string]string{"feed": "toeiBus", "stop": "x"}); err == nil {
		t.Error("an arg the op does not take was accepted")
	}
	if _, err := p.CheckArgs("busPositions", map[string]string{"feed": "unlisted"}); err == nil {
		t.Error("unlisted feed accepted")
	}
}
