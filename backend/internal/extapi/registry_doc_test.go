package extapi

import (
	"os"
	"regexp"
	"slices"
	"strings"
	"testing"
)

const apisDoc = "../../../docs/backend/features/external-apis.md"

var docTick = regexp.MustCompile("`([^`]+)`")

// docProviderOps reads the "入っている provider" table of doc: the first
// cell of a row is "`provider` の `op` / `op`(...)". It returns provider → ops.
func docProviderOps(t *testing.T, doc string) map[string][]string {
	t.Helper()
	_, sec, ok := strings.Cut(doc, "\n### 入っている provider\n")
	if !ok {
		t.Fatalf("%s: no 入っている provider section", apisDoc)
	}
	sec, _, _ = strings.Cut(sec, "\n#")
	out := map[string][]string{}
	for _, line := range strings.Split(sec, "\n") {
		if !strings.HasPrefix(line, "| `") {
			continue
		}
		cell, _, _ := strings.Cut(strings.TrimPrefix(line, "| "), " | ")
		head, _, _ := strings.Cut(cell, "(")
		prov, ops, ok := strings.Cut(head, " の ")
		pm := docTick.FindAllStringSubmatch(prov, -1)
		om := docTick.FindAllStringSubmatch(ops, -1)
		if !ok || len(pm) != 1 || len(om) == 0 {
			t.Errorf("row %q: first cell is not \"`provider` の `op`\"", cell)
			continue
		}
		for _, m := range om {
			out[pm[0][1]] = append(out[pm[0][1]], m[1])
		}
	}
	return out
}

func TestDocProviderOpsParser(t *testing.T) {
	doc := "x\n### 入っている provider\n\n| provider | op |\n|---|---|\n" +
		"| `a` の `x` / `y`(読む・`z`) | `x()` |\n| `b` の `w` | 同上 |\n| `a` の `v` | |\n\n### 次\n| `c` の `u` | |\n"
	got := docProviderOps(t, doc)
	if len(got) != 2 || !slices.Equal(got["a"], []string{"x", "y", "v"}) || !slices.Equal(got["b"], []string{"w"}) {
		t.Fatalf("got %v", got)
	}
}

// The registry and the documented provider table stay one list: every
// registered provider and op (default-off ones too; fakes aside) has a row,
// and every row is registered.
func TestRegistryMatchesDoc(t *testing.T) {
	b, err := os.ReadFile(apisDoc)
	if err != nil {
		t.Fatal(err)
	}
	doc := docProviderOps(t, string(b))
	if len(doc) == 0 {
		t.Fatal("no providers found; the doc layout changed")
	}
	registered := map[string]bool{}
	for _, p := range Providers() {
		if p.Fake {
			continue
		}
		if _, ok := doc[p.ID]; !ok {
			t.Errorf("provider %s is not in the table of %s", p.ID, apisDoc)
			continue
		}
		for _, o := range p.Ops {
			registered[p.ID+"."+o.Name] = true
			if !slices.Contains(doc[p.ID], o.Name) {
				t.Errorf("%s の %s is not in the table of %s", p.ID, o.Name, apisDoc)
			}
		}
	}
	for id, ops := range doc {
		if p, ok := Lookup(id); !ok || p.Fake {
			t.Errorf("documented provider %s is not registered", id)
			continue
		}
		for _, o := range ops {
			if !registered[id+"."+o] {
				t.Errorf("documented %s の %s is not registered", id, o)
			}
		}
	}
}
