package feeds

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
)

// ---- warnings (気象庁 bosai/warning/data/r8/{office}.json) ------------------

// The r8 file (the 2026 warning system, 警戒レベル付き) is a list of reports,
// one per kind of bulletin; each lists the 一次細分区域 (class10) with the
// warnings in force and their status.
type jmaWarningReport struct {
	ReportDatetime string `json:"reportDatetime"`
	Warning        struct {
		Class10Items []struct {
			AreaCode string `json:"areaCode"`
			Kinds    []struct {
				Code   string `json:"code"`
				Status string `json:"status"`
			} `json:"kinds"`
		} `json:"class10Items"`
	} `json:"warning"`
}

type warningKind struct {
	name  string
	level int // 50 特別警報 / 40 危険警報 / 30 警報 / 20 注意報
}

// warningKinds are the r8 codes the JMA warning page names (its own code
// table, 2026-11-08). 氾濫 (flood) comes in a separate file and is not read.
// An unknown code is skipped rather than shown under a guessed name.
var warningKinds = map[string]warningKind{
	"33": {"大雨特別警報", 50}, "43": {"大雨危険警報", 40}, "03": {"大雨警報", 30}, "10": {"大雨注意報", 20},
	"39": {"土砂災害特別警報", 50}, "49": {"土砂災害危険警報", 40}, "09": {"土砂災害警報", 30}, "29": {"土砂災害注意報", 20},
	"38": {"高潮特別警報", 50}, "48": {"高潮危険警報", 40}, "08": {"高潮警報", 30}, "19": {"高潮注意報", 20},
	"35": {"暴風特別警報", 50}, "05": {"暴風警報", 30}, "15": {"強風注意報", 20},
	"32": {"暴風雪特別警報", 50}, "02": {"暴風雪警報", 30}, "13": {"風雪注意報", 20},
	"36": {"大雪特別警報", 50}, "06": {"大雪警報", 30}, "12": {"大雪注意報", 20},
	"37": {"波浪特別警報", 50}, "07": {"波浪警報", 30}, "16": {"波浪注意報", 20},
	"14": {"雷注意報", 20}, "17": {"融雪注意報", 20}, "20": {"濃霧注意報", 20}, "21": {"乾燥注意報", 20},
	"22": {"なだれ注意報", 20}, "23": {"低温注意報", 20}, "24": {"霜注意報", 20}, "25": {"着氷注意報", 20}, "26": {"着雪注意報", 20},
}

// activeStatus: 発表・継続・警報から注意報 ... are in force; 解除 and
// 「発表警報・注意報はなし」 are not.
func activeStatus(s string) bool {
	return s != "" && s != "解除" && !strings.Contains(s, "なし")
}

// parseWarnings returns one row per warning in force (grouped by kind and
// status), strongest first: title = name, subtitle = status · areas,
// date = the newest report time (RFC 3339, JST). No warning in force is an
// empty, non-nil list (the leaf shows its emptyText), not an error.
func parseWarnings(body []byte) ([]Item, error) {
	var reports []jmaWarningReport
	if err := json.Unmarshal(body, &reports); err != nil {
		return nil, fmt.Errorf("warnings: %w", err)
	}
	if len(reports) == 0 {
		return nil, errors.New("warnings: no reports")
	}
	type key struct{ code, status string }
	type group struct {
		areas []string
		seen  map[string]bool
		at    time.Time
	}
	groups := map[key]*group{}
	for _, r := range reports {
		at, err := time.Parse(time.RFC3339, r.ReportDatetime)
		if err != nil {
			continue
		}
		for _, a := range r.Warning.Class10Items {
			for _, k := range a.Kinds {
				if _, ok := warningKinds[k.Code]; !ok || !activeStatus(k.Status) {
					continue
				}
				kk := key{k.Code, clean(k.Status, 20)}
				g := groups[kk]
				if g == nil {
					g = &group{seen: map[string]bool{}}
					groups[kk] = g
				}
				if at.After(g.at) {
					g.at = at
				}
				if name := class10Names[a.AreaCode]; name != "" && !g.seen[name] {
					g.seen[name] = true
					g.areas = append(g.areas, name)
				}
			}
		}
	}
	out := make([]Item, 0, len(groups))
	for k, g := range groups {
		w := warningKinds[k.code]
		sub := k.status
		if len(g.areas) > 0 {
			sub += " · " + strings.Join(g.areas, "・")
		}
		icon := "exclamationmark.circle"
		if w.level >= 30 {
			icon = "exclamationmark.triangle.fill"
		}
		out = append(out, Item{Date: g.at.In(jst).Format(time.RFC3339), Title: w.name,
			Subtitle: clean(sub, maxSubtitleRunes), Icon: icon})
	}
	sort.Slice(out, func(i, j int) bool {
		li, lj := levelOf(out[i].Title), levelOf(out[j].Title)
		if li != lj {
			return li > lj
		}
		if out[i].Title != out[j].Title {
			return out[i].Title < out[j].Title
		}
		return out[i].Subtitle < out[j].Subtitle
	})
	return out, nil
}

func levelOf(name string) int {
	for _, w := range warningKinds {
		if w.name == name {
			return w.level
		}
	}
	return 0
}
