package feeds

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/jphacks/kz_2601/backend/pkg/dsl"
)

// ---- typhoon (気象庁 bosai/typhoon/data/) -----------------------------------

// targetTc.json lists the tropical cyclones JMA reports on now; each has its
// own {TC}/specifications.json (analysis + forecast parts). One fetch reads
// the list and then at most maxTyphoons spec files, all or nothing.
const maxTyphoons = 5

// tcIDRe is the only shape of a cyclone id that becomes a path segment.
var tcIDRe = regexp.MustCompile(`^TC[0-9]{4}$`)

type jmaTyphoonTarget struct {
	TropicalCyclone string `json:"tropicalCyclone"`
}

// parseTyphoonTargets returns the cyclone ids to read (checked, deduped,
// capped). No cyclone is an empty, non-nil list.
func parseTyphoonTargets(body []byte) ([]string, error) {
	var ts []jmaTyphoonTarget
	if err := json.Unmarshal(body, &ts); err != nil {
		return nil, fmt.Errorf("typhoon: %w", err)
	}
	out := []string{}
	seen := map[string]bool{}
	for _, t := range ts {
		if !tcIDRe.MatchString(t.TropicalCyclone) || seen[t.TropicalCyclone] {
			continue
		}
		seen[t.TropicalCyclone] = true
		out = append(out, t.TropicalCyclone)
		if len(out) >= maxTyphoons {
			break
		}
	}
	return out, nil
}

// jmaText is a value JMA writes as a string or a number ("990", 990, "-").
type jmaText string

func (t *jmaText) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err == nil {
		*t = jmaText(s)
		return nil
	}
	var n json.Number
	if err := json.Unmarshal(b, &n); err == nil {
		*t = jmaText(n.String())
		return nil
	}
	*t = ""
	return nil // objects / null: not a value we read
}

// num parses t when it is a number within [lo, hi].
func (t jmaText) num(lo, hi float64) (float64, bool) {
	v, err := strconv.ParseFloat(strings.TrimSpace(string(t)), 64)
	return v, err == nil && v >= lo && v <= hi && !math.IsNaN(v)
}

type jmaJPEN struct {
	JP string `json:"jp"`
	EN string `json:"en"`
}

type jmaTyphoonPart struct {
	Part          json.RawMessage `json:"part"` // "title" or {"jp":"実況",...}
	TyphoonNumber string          `json:"typhoonNumber"`
	Name          jmaJPEN         `json:"name"`
	Category      jmaJPEN         `json:"category"`
	AdvancedHours *int            `json:"advancedHours"`
	Scale         jmaText         `json:"scale"`
	Intensity     jmaText         `json:"intensity"`
	Position      struct {
		Deg []float64 `json:"deg"`
	} `json:"position"`
	Location    jmaText            `json:"location"`
	Course      jmaText            `json:"course"`
	Speed       map[string]jmaText `json:"speed"`
	Pressure    jmaText            `json:"pressure"`
	MaximumWind struct {
		Sustained map[string]jmaText `json:"sustained"`
		Gust      map[string]jmaText `json:"gust"`
	} `json:"maximumWind"`
	Validtime struct {
		JST string `json:"JST"`
	} `json:"validtime"`
}

func (p jmaTyphoonPart) isTitle() bool { return string(p.Part) == `"title"` }

// parseTyphoonSpec turns one specifications.json into a row: title =
// 台風第N号(名前), subtitle = category (when not 台風) · 大きさと強さ ·
// 位置 · 中心気圧 · 最大風速 · 進路と速さ · 予報の最後の時刻, date = the
// analysis time (RFC 3339, JST). Numbers out of range are left out.
func parseTyphoonSpec(body []byte) (Item, error) {
	var parts []jmaTyphoonPart
	if err := json.Unmarshal(body, &parts); err != nil {
		return Item{}, fmt.Errorf("typhoon: %w", err)
	}
	var title, now *jmaTyphoonPart
	var last time.Time // validtime of the furthest forecast part
	lastHours := 0
	for i := range parts {
		p := &parts[i]
		switch {
		case p.isTitle():
			if title == nil {
				title = p
			}
		case p.AdvancedHours != nil && *p.AdvancedHours == 0:
			if now == nil {
				now = p
			}
		case p.AdvancedHours != nil && *p.AdvancedHours > lastHours:
			if t, err := time.Parse(time.RFC3339, p.Validtime.JST); err == nil {
				last, lastHours = t, *p.AdvancedHours
			}
		}
	}
	if title == nil || now == nil {
		return Item{}, errors.New("typhoon: no title or analysis part")
	}
	at, err := time.Parse(time.RFC3339, now.Validtime.JST)
	if err != nil {
		return Item{}, fmt.Errorf("typhoon: analysis time: %w", err)
	}

	it := Item{Date: at.In(jst).Format(time.RFC3339), Icon: "tropicalstorm"}
	category := strings.TrimSpace(cmp.Or(now.Category.JP, title.Category.JP))
	if n, err := strconv.Atoi(title.TyphoonNumber); err == nil && len(title.TyphoonNumber) == 4 && n%100 > 0 {
		it.Title = fmt.Sprintf("台風第%d号", n%100)
		if name := clean(title.Name.JP, 20); name != "" {
			it.Title += "(" + name + ")"
		}
	} else {
		it.Title = cmp.Or(clean(category, 20), "熱帯低気圧")
		category = ""
	}
	it.Title = clean(it.Title, maxTitleRunes)

	var sub []string
	if category != "" && category != "台風" {
		sub = append(sub, category)
	}
	if strings.ToUpper(cmp.Or(now.Category.EN, title.Category.EN)) == "LOW" {
		it.Icon = "cloud"
	}
	var size []string
	for _, v := range []jmaText{now.Scale, now.Intensity} {
		if s := clean(string(v), 10); s != "" && s != "-" {
			size = append(size, s)
		}
	}
	if len(size) > 0 {
		sub = append(sub, strings.Join(size, "で"))
	}
	loc := clean(string(now.Location), 30)
	// JMA's area is the north-west Pacific (north, east); anything else is left out.
	if d := now.Position.Deg; len(d) == 2 && d[0] >= 0 && d[0] <= 90 && d[1] >= 0 && d[1] <= 180 {
		ll := fmt.Sprintf("北緯%.1f度・東経%.1f度", d[0], d[1])
		if loc != "" {
			loc += "(" + ll + ")"
		} else {
			loc = ll
		}
	}
	if loc != "" {
		sub = append(sub, loc)
	}
	if v, ok := now.Pressure.num(850, 1100); ok {
		sub = append(sub, fmt.Sprintf("中心気圧 %.0f hPa", v))
	}
	if v, ok := now.MaximumWind.Sustained["m/s"].num(1, 150); ok {
		w := fmt.Sprintf("最大風速 %.0f m/s", v)
		if g, ok := now.MaximumWind.Gust["m/s"].num(v, 200); ok {
			w += fmt.Sprintf("(瞬間 %.0f m/s)", g)
		}
		sub = append(sub, w)
	}
	move := clean(string(now.Course), 10)
	if move == "-" {
		move = ""
	}
	if v, ok := now.Speed["km/h"].num(0, 300); ok {
		move = strings.TrimSpace(fmt.Sprintf("%s %.0f km/h", move, v))
	} else if s := clean(string(now.Speed["km/h"]), 10); s != "" && s != "-" {
		move = strings.TrimSpace(move + " " + s) // ゆっくり・ほとんど停滞
	}
	if move != "" {
		sub = append(sub, move)
	}
	if !last.IsZero() {
		l := last.In(jst)
		sub = append(sub, fmt.Sprintf("予報 %d月%d日%d時まで", int(l.Month()), l.Day(), l.Hour()))
	}
	it.Subtitle = clean(strings.Join(sub, " · "), maxSubtitleRunes)
	return it, nil
}

// fetchTyphoon reads the list, then each cyclone's spec file (the URLs come
// from Endpoints and a checked id only). Any failure fails the whole fetch,
// so a partial list is never cached as fresh.
func (s *Service) fetchTyphoon(ctx context.Context) (string, []Item, int, error) {
	const title = "台風の情報"
	maxBody := providerLimits[dsl.FeedTyphoon].maxBody
	body, err := s.get(ctx, s.ep.typhoonListURL(), maxBody)
	n := len(body)
	if err != nil {
		return "", nil, n, err
	}
	ids, err := parseTyphoonTargets(body)
	if err != nil {
		return "", nil, n, fmt.Errorf("%w: %w", ErrBadResponse, err)
	}
	items := []Item{}
	for _, id := range ids {
		b, err := s.get(ctx, s.ep.typhoonSpecURL(id), maxBody)
		n += len(b)
		if err != nil {
			return "", nil, n, err
		}
		it, err := parseTyphoonSpec(b)
		if err != nil {
			return "", nil, n, fmt.Errorf("%w: %w", ErrBadResponse, err)
		}
		items = append(items, it)
	}
	return title, items, n, nil
}
