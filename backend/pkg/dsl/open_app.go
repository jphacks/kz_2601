package dsl

import (
	"fmt"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"unicode/utf8"
)

// openUrlButton.app ("open & send" integrations, iOS first): instead of a
// fixed url, name the app or service and iOS builds the link from public,
// documented forms only (no webhooks, no import, no secrets):
//
//	paypay    to = the creator's PayPay link (https, *.paypay.ne.jp); else https://paypay.ne.jp/
//	line      to = https line.me / lin.ee link (official account, friend add); else https://line.me/R/nv/chat
//	slack     to = https *.slack.com link (workspace / channel); else slack://open, then https://slack.com/
//	discord   to = https discord.com / discord.gg link (server / invite); else https://discord.com/app
//	x         to = username → https://x.com/<name>; else https://x.com/
//	instagram to = username → https://www.instagram.com/<name>/; else https://www.instagram.com/
//	maps      to = place / address → https://maps.apple.com/?q=<to>; else Apple Maps
//	mail      to = address → mailto:<to>; else mailto:
//	phone     to = number → tel:<number> (required)
//
// `to` may hold {{fieldId}} (the open record's value) on screen.entity
// screens; iOS re-checks the built link and falls back to the app's top
// page. Sending a record's text is shareButton (shareTo line / x / mail …).

// OpenApps is the openUrlButton.app allowlist (iOS OpenApp).
var OpenApps = []string{"paypay", "line", "slack", "discord", "x", "instagram", "maps", "mail", "phone"}

// OpenAppMaxTo caps openUrlButton.to (characters).
const OpenAppMaxTo = 300

// ShareTextMax is the shareButton text length lint threshold (characters);
// longer text is cut by LINE / X links and hard to read in a chat.
const ShareTextMax = 1000

var (
	openAppPlaceholderRe = regexp.MustCompile(`\{\{([^{}]*)\}\}`)
	openAppUserRe        = map[string]*regexp.Regexp{
		"x":         regexp.MustCompile(`^@?[A-Za-z0-9_]{1,15}$`),
		"instagram": regexp.MustCompile(`^@?[A-Za-z0-9._]{1,30}$`),
	}
	openAppMailRe  = regexp.MustCompile(`^[^@\s/?#&]+@[^@\s/?#&]+\.[^@\s/?#&]+$`)
	openAppPhoneRe = regexp.MustCompile(`^\+?[0-9][0-9\-() ]{1,19}$`)
)

// openAppHosts are the https hosts a `to` link may point at (subdomains too).
var openAppHosts = map[string][]string{
	"paypay":    {"paypay.ne.jp"},
	"line":      {"line.me", "lin.ee"},
	"slack":     {"slack.com"},
	"discord":   {"discord.com", "discord.gg"},
	"x":         {"x.com", "twitter.com"},
	"instagram": {"instagram.com"},
}

// OpenAppLinkHost reports whether s is an https URL on one of app's hosts.
func OpenAppLinkHost(app, s string) bool {
	u, err := url.Parse(s)
	if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil {
		return false
	}
	h := strings.ToLower(u.Hostname())
	for _, d := range openAppHosts[app] {
		if h == d || strings.HasSuffix(h, "."+d) {
			return true
		}
	}
	return false
}

// checkOpenURLButton validates openUrlButton: a fixed url, or app (+ to).
func checkOpenURLButton(where string, c *Component, formEntity *Entity, add func(string, ...any)) {
	if c.App == "" {
		if c.To != "" {
			add("%s: openUrlButton to needs app (%s)", where, strings.Join(OpenApps, "|"))
		}
		if c.Label == "" || c.URL == "" {
			add("%s: openUrlButton needs label and url (or app)", where)
		} else if !isOpenableURL(c.URL) {
			add("%s: openUrlButton url %q must be https or one of the schemes %s", where, c.URL, strings.Join(OpenURLSchemes, ", "))
		}
		return
	}
	if !slices.Contains(OpenApps, c.App) {
		add("%s: openUrlButton app %q must be one of %s", where, c.App, strings.Join(OpenApps, "|"))
		return
	}
	if c.URL != "" {
		add("%s: openUrlButton takes url or app, not both (put the link in to)", where)
	}
	if c.App == "phone" && c.To == "" {
		add("%s: openUrlButton app phone needs to (the phone number or {{fieldId}}); do not make a number up", where)
	}
	if utf8.RuneCountInString(c.To) > OpenAppMaxTo {
		add("%s: openUrlButton to is too long (max %d characters)", where, OpenAppMaxTo)
		return
	}
	if c.To == "" {
		return
	}
	if ms := openAppPlaceholderRe.FindAllStringSubmatch(c.To, -1); len(ms) > 0 {
		for _, m := range ms {
			switch {
			case formEntity == nil:
				add("%s: openUrlButton to {{%s}} requires screen.entity", where, m[1])
			case formEntity.FieldByID(m[1]) == nil:
				add("%s: openUrlButton to {{%s}} is not a field of entity %q (fields: %s)", where, m[1], formEntity.ID, fieldIDs(formEntity))
			case !TextSubstitutes(formEntity, m[1]):
				add("%s: openUrlButton to {{%s}} must be a visible text-like field (string / text / number / enum ...)", where, m[1])
			}
		}
		return // the value is known on the device only; iOS re-checks it
	}
	if msg := openAppToProblem(c.App, c.To); msg != "" {
		add("%s: openUrlButton app %s to %q: %s", where, c.App, c.To, msg)
	}
}

// openAppToProblem checks a literal `to` for app ("" = fine).
func openAppToProblem(app, to string) string {
	switch app {
	case "paypay", "line", "slack", "discord":
		if !OpenAppLinkHost(app, to) {
			return fmt.Sprintf("must be an https link on %s (the link the author gave); omit to just open the app", strings.Join(openAppHosts[app], " / "))
		}
	case "x", "instagram":
		if !openAppUserRe[app].MatchString(to) && !OpenAppLinkHost(app, to) {
			return "must be a username (e.g. @name) or an https link on " + strings.Join(openAppHosts[app], " / ")
		}
	case "mail":
		if !openAppMailRe.MatchString(to) {
			return "must be an email address"
		}
	case "phone":
		if !openAppPhoneRe.MatchString(to) {
			return "must be a phone number (digits, + - ( ) and spaces)"
		}
	}
	return ""
}

// shareText warns about shareButton text that is too long or names a field
// the screen does not have ({{x}} then goes out literally). Warnings only:
// saved apps keep validating.
func (l *linter) shareText() {
	for i := range l.app.Screens {
		s := &l.app.Screens[i]
		e := l.entities[s.Entity]
		var walk func(where string, cs []Component)
		walk = func(where string, cs []Component) {
			for j := range cs {
				at := fmt.Sprintf("screens[%s].components%s[%d]", s.ID, where, j)
				walk(fmt.Sprintf("%s[%d].children", where, j), cs[j].Children)
				c := &cs[j]
				if c.Type != CompShareButton {
					continue
				}
				if utf8.RuneCountInString(c.Text) > ShareTextMax {
					l.add("%s: shareButton text is longer than %d characters; LINE / X cut long text, so keep it short (title and a few values)", at, ShareTextMax)
				}
				for _, m := range openAppPlaceholderRe.FindAllStringSubmatch(c.Text, -1) {
					if e == nil {
						l.add("%s: shareButton text {{%s}} needs screen.entity (it is sent as written)", at, m[1])
					} else if e.FieldByID(m[1]) == nil {
						l.add("%s: shareButton text {{%s}} is not a field of entity %q (fields: %s)", at, m[1], e.ID, fieldIDs(e))
					}
				}
			}
		}
		walk("", s.Components)
	}
}
