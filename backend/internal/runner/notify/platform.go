package notify

import (
	"context"
	"errors"
	"log/slog"
	"sync"
)

// ErrPlatformNotConfigured is returned for a push whose platform has no
// sender (e.g. an Android device while the FCM key is not set).
var ErrPlatformNotConfigured = errors.New("notify: no sender for the device's platform")

// PlatformSender sends each push through the sender of its platform (#918):
// iOS (and "") through IOS (APNs), Android through Android (FCM). A nil
// sender skips its platform: the push fails without retry (the recipient
// counts as send_failed) and the first skip per platform is logged.
type PlatformSender struct {
	IOS     Sender
	Android Sender
	Logger  *slog.Logger // nil = slog.Default()

	logged sync.Map // platform -> struct{}
}

var _ Sender = (*PlatformSender)(nil)

// Send implements Sender.
func (s *PlatformSender) Send(ctx context.Context, p Push) (Result, error) {
	platform, to := p.Platform, Sender(nil)
	switch platform {
	case "", PlatformIOS:
		platform, to = PlatformIOS, s.IOS
	case PlatformAndroid:
		to = s.Android
	}
	if to == nil {
		if _, seen := s.logged.LoadOrStore(platform, struct{}{}); !seen {
			l := s.Logger
			if l == nil {
				l = slog.Default()
			}
			l.WarnContext(ctx, "notify: no sender for the platform; its pushes are skipped", slog.String("platform", platform))
		}
		return Result{Reason: "PlatformNotConfigured"}, ErrPlatformNotConfigured
	}
	return to.Send(ctx, p)
}
