package runner

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/jphacks/kz_2601/backend/internal/runner/engine"
	"github.com/jphacks/kz_2601/backend/internal/runner/scope"
	"github.com/jphacks/kz_2601/backend/internal/runner/status"
	"github.com/jphacks/kz_2601/backend/internal/store"
	"google.golang.org/grpc/codes"
	grpcstatus "google.golang.org/grpc/status"
)

// #347: each error of a run has a code, and each code a fixed reason word
// for the creator; a scope violation or an unknown error stays "internal".
func TestErrorCodeFailureReason(t *testing.T) {
	cases := []struct {
		err  error
		code string
		want string
	}{
		{errSendRetry, status.CodeSendFailed, status.FailureNotifyFailed},
		{fmt.Errorf("rule x: %w", scope.ErrNotReadable), status.CodeNotReadable, status.FailureRecordUnavailable},
		{fmt.Errorf("read: %w", store.ErrNotFound), status.CodeNotFound, status.FailureRecordUnavailable},
		{fmt.Errorf("engine: entity %q is not in the app: %w", "task", engine.ErrMismatch), status.CodeMismatch, status.FailureMismatch},
		{rejectedWrite(store.ErrDuplicate), status.CodeWriteRejected, status.FailureWriteRejected},
		{rejectedWrite(grpcstatus.Error(codes.InvalidArgument, "value too large")), status.CodeWriteRejected, status.FailureWriteRejected},
		{errFeedWait, status.CodeFeedWait, status.FailureSourceUnavailable},
		{context.DeadlineExceeded, status.CodeTimeout, status.FailureTimeout},
		{scope.ErrViolation, status.CodeScopeViolation, status.FailureInternal},
		{errors.New("users/u1/apps/a1: boom"), status.CodeError, status.FailureInternal},
		// a write that did not get through is not "rejected"
		{rejectedWrite(grpcstatus.Error(codes.Unavailable, "try later")), status.CodeError, status.FailureInternal},
	}
	for _, c := range cases {
		code := errorCode(c.err)
		if code != c.code {
			t.Errorf("errorCode(%v) = %q, want %q", c.err, code, c.code)
		}
		if got := status.FailureReason(code); got != c.want {
			t.Errorf("FailureReason(%q) = %q, want %q", code, got, c.want)
		}
		if got := status.FailureReason(code); !status.IsFailureReason(got) {
			t.Errorf("FailureReason(%q) = %q is not a Failure* word", code, got)
		}
	}
}
