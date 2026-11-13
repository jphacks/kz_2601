package extapi

import (
	"strings"
	"time"
	"unicode/utf8"
)

// Shared by the deployment-status providers (vercel.go, cloudflare.go,
// #1676): one row per deployment or project, the same state words.

// The row's value: a deployment's state in the app's words.
const (
	deployReady    = "成功"
	deployError    = "失敗"
	deployBuilding = "ビルド中"
	deployQueued   = "待ち"
	deployCanceled = "取り消し"
	deployLive     = "公開中"
)

var deployJST = time.FixedZone("JST", 9*60*60)

// deployTime is a deployment's time in Japan time ("" when unknown).
func deployTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.In(deployJST).Format("2006-01-02 15:04")
}

// deploySubtitle joins the non-empty parts with " · " (one line, 160 runes).
func deploySubtitle(parts ...string) string {
	var keep []string
	for _, p := range parts {
		p = strings.Join(strings.Fields(p), " ")
		if p != "" {
			keep = append(keep, p)
		}
	}
	s := strings.Join(keep, " · ")
	if utf8.RuneCountInString(s) > 160 {
		s = string([]rune(s)[:159]) + "…"
	}
	return s
}
