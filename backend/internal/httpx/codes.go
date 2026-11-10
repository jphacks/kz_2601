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

// the server allowance is used up and paying overflow with coins is off (#630; coins.WriteError, enforce only)
const CodeAllowanceExhausted = "allowance_exhausted"

// App Check enforce (#318)
const CodeAppCheckFailed = "app_check_failed"

// the operator force-stopped the app's server features (#346)
const CodeAppStopped = "app_stopped"

const CodeAppconfigError = "appconfig_error"

const CodeAppconfigUnreachable = "appconfig_unreachable"

const CodeBadOutput = "bad_output"

const CodeBadRequest = "bad_request"

// the payer has fewer coins than the call's hold (#630; coins.WriteError, enforce only)
const CodeCoinsInsufficient = "coins_insufficient"

// another record of the entity has the same entity.unique values (#273)
const CodeDuplicateRecord = "duplicate_record"

// the X-Request-Id of an AI call was already used for a coin hold (#630, enforce only)
const CodeDuplicateRequest = "duplicate_request"

const CodeForbidden = "forbidden"

// the free plan's AI work is paused: the day's AI cost passed its threshold (#317)
const CodeFreePlanPaused = "free_plan_paused"

const CodeGenerationFailed = "generation_failed"

const CodeInternal = "internal"

const CodeInvalidDSL = "invalid_dsl"

const CodeInvalidRecord = "invalid_record"

const CodeInvalidTransaction = "invalid_transaction"

const CodeNotFound = "not_found"

// overflow paid with coins reached the payer's monthly cap (#630; coins.WriteError, enforce only)
const CodeOverflowCapReached = "overflow_cap_reached"

const CodePayloadTooLarge = "payload_too_large"

// the payer's plan limit (apps, records, photos) is reached (#317)
const CodePlanLimitReached = "plan_limit_reached"

// a paid feature (Tailor It Pro) the caller or the app's creator does not have (#316)
const CodeProRequired = "pro_required"

const CodeRateLimited = "rate_limited"

const CodeRefused = "refused"

// text other people see contains a word on the moderation list (#321)
const CodeTextNotAllowed = "text_not_allowed"

const CodeTimeout = "timeout"

const CodeTransactionOwnedByOther = "transaction_owned_by_other"

const CodeUnauthenticated = "unauthenticated"

const CodeUnpublished = "unpublished"

const CodeUpstreamTimeout = "upstream_timeout"

const CodeUpstreamUnavailable = "upstream_unavailable"

const CodeVersionConflict = "version_conflict"
