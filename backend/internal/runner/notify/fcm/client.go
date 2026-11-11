// Package fcm sends notify.Push to Android devices through the FCM HTTP v1
// API (Firebase Admin SDK) with a service account (#918). Only the runner
// holds the key (#337); this package never reads a key file itself and never
// logs the key or a device token.
package fcm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	firebase "firebase.google.com/go/v4"
	"firebase.google.com/go/v4/errorutils"
	"firebase.google.com/go/v4/messaging"
	"google.golang.org/api/option"

	"github.com/jphacks/kz_2601/backend/internal/runner/notify"
)

// ErrRejected wraps an answer other than success.
var ErrRejected = errors.New("fcm: push rejected")

// Data keys of the message (the same meaning as the APNs payload's appId).
const (
	DataAppID   = "appId"   // opens the app on tap (inside Tailor It only)
	DataAppName = "appName" // the sender shown with the notification (#339)
)

// Config configures a Client.
type Config struct {
	// CredentialsJSON is the service account key (JSON) from the secret store.
	CredentialsJSON []byte
	// ProjectID: "" = the key's project_id.
	ProjectID string
	// Options are added after the credentials (tests: option.WithEndpoint,
	// option.WithHTTPClient).
	Options []option.ClientOption
}

// messenger is the part of messaging.Client a Client uses.
type messenger interface {
	Send(ctx context.Context, m *messaging.Message) (string, error)
}

// Client implements notify.Sender.
type Client struct {
	m messenger
}

var _ notify.Sender = (*Client)(nil)

// ProjectOf returns the project_id of a service account key. Its errors
// carry no key material.
func ProjectOf(credentialsJSON []byte) (string, error) {
	var k struct {
		Type      string `json:"type"`
		ProjectID string `json:"project_id"`
	}
	if err := json.Unmarshal(credentialsJSON, &k); err != nil {
		return "", errors.New("fcm: the key is not JSON")
	}
	if k.Type != "service_account" {
		return "", errors.New("fcm: the key is not a service account key")
	}
	if k.ProjectID == "" {
		return "", errors.New("fcm: the key has no project_id")
	}
	return k.ProjectID, nil
}

// New builds a Client. It does not call FCM.
func New(ctx context.Context, cfg Config) (*Client, error) {
	if len(cfg.CredentialsJSON) == 0 {
		return nil, errors.New("fcm: CredentialsJSON is required")
	}
	project, err := ProjectOf(cfg.CredentialsJSON)
	if err != nil {
		return nil, err
	}
	if cfg.ProjectID != "" {
		project = cfg.ProjectID
	}
	opts := append([]option.ClientOption{option.WithAuthCredentialsJSON(option.ServiceAccount, cfg.CredentialsJSON)}, cfg.Options...)
	app, err := firebase.NewApp(ctx, &firebase.Config{ProjectID: project}, opts...)
	if err != nil {
		return nil, fmt.Errorf("fcm: init: %w", err)
	}
	m, err := app.Messaging(ctx)
	if err != nil {
		return nil, fmt.Errorf("fcm: messaging init: %w", err)
	}
	return &Client{m: m}, nil
}

// Message builds the FCM message of p: the same title, body and appId as the
// APNs payload. The app name goes in data (Android has no subtitle). Tag
// (from the collapse id) makes a retried send replace, not repeat, the
// notification, like apns-collapse-id.
func Message(p notify.Push) *messaging.Message {
	data := map[string]string{}
	if p.AppID != "" {
		data[DataAppID] = p.AppID
	}
	if p.AppName != "" {
		data[DataAppName] = p.AppName
	}
	if len(data) == 0 {
		data = nil
	}
	return &messaging.Message{
		Token:        p.Token,
		Notification: &messaging.Notification{Title: p.Title, Body: p.Body},
		Data:         data,
		Android: &messaging.AndroidConfig{
			Priority:     "high",
			Notification: &messaging.AndroidNotification{Tag: p.CollapseID, Sound: "default"},
		},
	}
}

// Send posts p. err is nil only when FCM accepted it; Result says whether
// the token is unregistered (UNREGISTERED, or SENDER_ID_MISMATCH: the token
// belongs to another Firebase project) or a retry may help (429, 5xx,
// network).
func (c *Client) Send(ctx context.Context, p notify.Push) (notify.Result, error) {
	if p.Token == "" {
		return notify.Result{}, errors.New("fcm: empty device token")
	}
	if _, err := c.m.Send(ctx, Message(p)); err != nil {
		return classify(err), fmt.Errorf("%w: %w", ErrRejected, err)
	}
	return notify.Result{Status: 200}, nil
}

func classify(err error) notify.Result {
	var r notify.Result
	resp := errorutils.HTTPResponse(err)
	if resp != nil {
		r.Status = resp.StatusCode
	}
	switch {
	case messaging.IsUnregistered(err):
		r.Reason, r.Unregistered = "UNREGISTERED", true
	case messaging.IsSenderIDMismatch(err):
		r.Reason, r.Unregistered = "SENDER_ID_MISMATCH", true
	case messaging.IsQuotaExceeded(err):
		r.Reason, r.Retryable = "QUOTA_EXCEEDED", true
	case messaging.IsUnavailable(err):
		r.Reason, r.Retryable = "UNAVAILABLE", true
	case messaging.IsInternal(err):
		r.Reason, r.Retryable = "INTERNAL", true
	case messaging.IsInvalidArgument(err):
		r.Reason = "INVALID_ARGUMENT"
	case resp == nil:
		r.Retryable = true // no answer (network, timeout)
	case r.Status == 429 || r.Status >= 500:
		r.Retryable = true
	}
	return r
}
