package httpx

const (
	CodeBadRequest      = "bad_request"
	CodeUnauthenticated = "unauthenticated"
	CodeForbidden       = "forbidden"
	CodeNotFound        = "not_found"
	CodeVersionConflict = "version_conflict"
	CodeInvalidDSL      = "invalid_dsl"
	CodeInvalidRecord   = "invalid_record"
	CodePayloadTooLarge = "payload_too_large"
	CodeRateLimited     = "rate_limited"
	CodeAppCheckFailed  = "app_check_failed" // App Check enforce (#318)
	CodeAIPaused        = "ai_paused"        // AI_PAUSED=1 on the ai service (#511)
	CodeInternal        = "internal"
)
