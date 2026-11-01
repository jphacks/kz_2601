package appconfig

import (
	"encoding/json"
	"log/slog"
	"net/url"
	"regexp"
	"strings"
	"unicode/utf8"
)

// In-app notice (#333): one line shown as a dismissible banner on iOS during
// incidents or maintenance. Set NOTICE_JSON on appconfig-<env>; unset or
// invalid means no notice (a bad value never breaks /v1/client-config).
//
//	NOTICE_JSON='{"id":"2026-11-02-ai","severity":"warning","message":{"ja":"…","en":"…"}}'
const EnvNotice = "NOTICE_JSON"

const (
	NoticeInfo    = "info"
	NoticeWarning = "warning"

	noticeMaxRunes = 200
)

var noticeIDPattern = regexp.MustCompile(`^[A-Za-z0-9._-]{1,64}$`)

// Notice is the "notice" field of GET /v1/client-config. iOS remembers a
// dismissal per ID, so publish a new ID to show a changed notice again.
type Notice struct {
	ID       string     `json:"id"`
	Severity string     `json:"severity"`
	Message  NoticeText `json:"message"`
	LinkURL  string     `json:"linkUrl,omitempty"`
	// Shown only to builds (CFBundleVersion) in [MinIOSBuild, MaxIOSBuild]; 0 = open end.
	MinIOSBuild int `json:"minIOSBuild,omitempty"`
	MaxIOSBuild int `json:"maxIOSBuild,omitempty"`
}

// NoticeText holds the line per language. iOS falls back to ja.
type NoticeText struct {
	Ja string `json:"ja"`
	En string `json:"en,omitempty"`
}

// NoticeFromEnv parses NOTICE_JSON. It returns nil when unset or invalid.
func NoticeFromEnv(getenv func(string) string) *Notice {
	raw := strings.TrimSpace(getenv(EnvNotice))
	if raw == "" {
		return nil
	}
	var n Notice
	dec := json.NewDecoder(strings.NewReader(raw))
	dec.DisallowUnknownFields() // a typo ("link") should be loud, not silently dropped
	if err := dec.Decode(&n); err != nil {
		slog.Warn("appconfig: ignoring invalid notice", slog.String("env", EnvNotice), slog.Any("error", err))
		return nil
	}
	if err := n.normalize(); err != "" {
		slog.Warn("appconfig: ignoring invalid notice", slog.String("env", EnvNotice), slog.String("error", err))
		return nil
	}
	slog.Info("appconfig: serving notice", slog.String("id", n.ID), slog.String("severity", n.Severity))
	return &n
}

// normalize trims and validates in place; it returns the reason when invalid.
func (n *Notice) normalize() string {
	n.ID = strings.TrimSpace(n.ID)
	if !noticeIDPattern.MatchString(n.ID) {
		return "id must match " + noticeIDPattern.String()
	}
	switch n.Severity = strings.TrimSpace(n.Severity); n.Severity {
	case "":
		n.Severity = NoticeInfo
	case NoticeInfo, NoticeWarning:
	default:
		return "severity must be info or warning"
	}
	n.Message.Ja, n.Message.En = oneLine(n.Message.Ja), oneLine(n.Message.En)
	if n.Message.Ja == "" {
		return "message.ja is required"
	}
	if utf8.RuneCountInString(n.Message.Ja) > noticeMaxRunes || utf8.RuneCountInString(n.Message.En) > noticeMaxRunes {
		return "message is too long"
	}
	if n.LinkURL = strings.TrimSpace(n.LinkURL); n.LinkURL != "" {
		if u, err := url.Parse(n.LinkURL); err != nil || u.Scheme != "https" || u.Host == "" {
			return "linkUrl must be https"
		}
	}
	if n.MinIOSBuild < 0 || n.MaxIOSBuild < 0 {
		return "build bounds must not be negative"
	}
	if n.MaxIOSBuild != 0 && n.MaxIOSBuild < n.MinIOSBuild {
		return "maxIOSBuild is below minIOSBuild"
	}
	return ""
}

// oneLine collapses all whitespace (including newlines) to single spaces.
func oneLine(s string) string { return strings.Join(strings.Fields(s), " ") }
