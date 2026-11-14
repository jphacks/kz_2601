package ingest

import (
	"errors"
	"strconv"
	"strings"
	"time"
)

// GenFailFilter is the Cloud Logging filter of the ai.generate lines a
// Logging API GenLogSource reads (#2146). It is a pure function: the reader
// that sends it is a follow-up.
type GenFailFilter struct {
	// Since and Until are the range, both ends included (the same as
	// GenLogSource.GenerateEvents): Since is the previous ingest, Until now.
	Since, Until time.Time
	// Services, when set, keeps only these Cloud Run services.
	Services []string
	// FailuresOnly keeps only lines with a failure code (GenEvent.FailureCodes).
	// The spike detector needs every ai.generate line (the rate's
	// denominator), so the reader leaves it false.
	FailuresOnly bool
}

// GenFailFields are the jsonPayload fields the decoder reads (genPayload):
// the reader asks for these and nothing else (no uid_hash, app_hash or text).
var GenFailFields = []string{"time", "event", "mode", "outcome", "error_code", "retry_reasons", "validation_codes"}

// genFailFailureClause matches a line with any failure code.
const genFailFailureClause = `(jsonPayload.outcome="failed" OR jsonPayload.outcome="refused" OR jsonPayload.error_code:* OR jsonPayload.retry_reasons:* OR jsonPayload.validation_codes:*)`

// Build builds the filter. Times are UTC RFC 3339 with nanoseconds, so the
// range ends are exact.
func (f GenFailFilter) Build() (string, error) {
	switch {
	case f.Since.IsZero() || f.Until.IsZero():
		return "", errors.New("genfail filter: since and until are required")
	case f.Until.Before(f.Since):
		return "", errors.New("genfail filter: until is before since")
	}
	parts := []string{`resource.type="cloud_run_revision"`}
	if len(f.Services) > 0 {
		q := make([]string, len(f.Services))
		for i, s := range f.Services {
			if s = strings.TrimSpace(s); s == "" {
				return "", errors.New("genfail filter: empty service name")
			}
			q[i] = strconv.Quote(s)
		}
		parts = append(parts, "resource.labels.service_name=("+strings.Join(q, " OR ")+")")
	}
	parts = append(parts, "jsonPayload.event="+strconv.Quote(genEvent))
	if f.FailuresOnly {
		parts = append(parts, genFailFailureClause)
	}
	parts = append(parts,
		"timestamp>="+strconv.Quote(f.Since.UTC().Format(time.RFC3339Nano)),
		"timestamp<="+strconv.Quote(f.Until.UTC().Format(time.RFC3339Nano)))
	return strings.Join(parts, " AND "), nil
}
