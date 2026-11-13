package main

import "testing"

// TestVersionKind015: one row for each kind versionKind tells apart.
func TestVersionKind015(t *testing.T) {
	cases := []struct {
		name string
		v    versionData
		want string
	}{
		{"ai first full", versionData{Kind: "ai", Mode: "full", N: 2}, stepCreate},
		{"ai later full", versionData{Kind: "ai", Mode: "full", N: 3}, stepEdit},
		{"ai patch early", versionData{Kind: "ai", Mode: "patch", N: 2}, stepEdit},
		{"restore", versionData{Kind: "restore", N: 5}, stepRestore},
		{"manual rename", versionData{Kind: "manual", Summary: "名前を「家計簿」に変えました"}, stepRename},
		{"manual edit", versionData{Kind: "manual", Summary: "項目を足しました"}, stepManual},
		{"import", versionData{Kind: "import"}, stepImport},
		{"system", versionData{Kind: "system", N: 1}, stepSeed},
		{"empty kind", versionData{}, stepSeed},
	}
	for _, tc := range cases {
		if got := versionKind(&tc.v); got != tc.want {
			t.Errorf("%s: versionKind = %q, want %q", tc.name, got, tc.want)
		}
	}
}
