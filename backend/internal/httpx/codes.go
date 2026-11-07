package httpx

// Machine-readable error codes (`error.code`) of every service. This file is
// the only place they are defined: openapi-appconfig.yaml / openapi-ai.yaml
// list the codes each service returns, checked by codes_spec_test.go (#523).
//
// One declaration per line, sorted by value (#516): separate const lines keep
// gofmt from re-aligning neighbours when a code is added.

const CodeAccountDeleted = "account_deleted"

// AI_PAUSED=1 on the ai service (#511)
const CodeAIPaused = "ai_paused"

// App Check enforce (#318)
const CodeAppCheckFailed = "app_check_failed"

// the operator force-stopped the app's server features (#346)
const CodeAppStopped = "app_stopped"

const CodeAppconfigError = "appconfig_error"

const CodeAppconfigUnreachable = "appconfig_unreachable"

const CodeBadOutput = "bad_output"

const CodeBadRequest = "bad_request"

const CodeForbidden = "forbidden"

const CodeGenerationFailed = "generation_failed"

const CodeInternal = "internal"

const CodeInvalidDSL = "invalid_dsl"

const CodeInvalidRecord = "invalid_record"

const CodeInvalidTransaction = "invalid_transaction"

const CodeNotFound = "not_found"

const CodePayloadTooLarge = "payload_too_large"

// a paid feature (Tailor It Pro) the caller or the app's creator does not have (#316)
const CodeProRequired = "pro_required"

const CodeRateLimited = "rate_limited"

const CodeRefused = "refused"

const CodeTimeout = "timeout"

const CodeTransactionOwnedByOther = "transaction_owned_by_other"

const CodeUnauthenticated = "unauthenticated"

const CodeUnpublished = "unpublished"

const CodeUpstreamTimeout = "upstream_timeout"

const CodeUpstreamUnavailable = "upstream_unavailable"

const CodeVersionConflict = "version_conflict"
