package jp.kz2601.metamon.services.http

/**
 * backend の `error.code`(backend/internal/httpx/codes.go と同じ値・同じ並び)。
 * 揃っているかは ErrorCodesTest が codes.go を読んで確かめる。
 */
object ErrorCodes {
    const val ACCOUNT_DELETED = "account_deleted"
    const val AI_PAUSED = "ai_paused"
    const val APP_CHECK_FAILED = "app_check_failed"
    const val APP_STOPPED = "app_stopped"
    const val APPCONFIG_ERROR = "appconfig_error"
    const val APPCONFIG_UNREACHABLE = "appconfig_unreachable"
    const val BAD_OUTPUT = "bad_output"
    const val BAD_REQUEST = "bad_request"
    const val DUPLICATE_RECORD = "duplicate_record"
    const val FORBIDDEN = "forbidden"
    const val FREE_PLAN_PAUSED = "free_plan_paused"
    const val GENERATION_FAILED = "generation_failed"
    const val INTERNAL = "internal"
    const val INVALID_DSL = "invalid_dsl"
    const val INVALID_RECORD = "invalid_record"
    const val INVALID_TRANSACTION = "invalid_transaction"
    const val NOT_FOUND = "not_found"
    const val PAYLOAD_TOO_LARGE = "payload_too_large"
    const val PLAN_LIMIT_REACHED = "plan_limit_reached"
    const val PRO_REQUIRED = "pro_required"
    const val RATE_LIMITED = "rate_limited"
    const val REFUSED = "refused"
    const val TEXT_NOT_ALLOWED = "text_not_allowed"
    const val TIMEOUT = "timeout"
    const val TRANSACTION_OWNED_BY_OTHER = "transaction_owned_by_other"
    const val UNAUTHENTICATED = "unauthenticated"
    const val UNPUBLISHED = "unpublished"
    const val UPSTREAM_TIMEOUT = "upstream_timeout"
    const val UPSTREAM_UNAVAILABLE = "upstream_unavailable"
    const val VERSION_CONFLICT = "version_conflict"

    val all: List<String> = listOf(
        ACCOUNT_DELETED, AI_PAUSED, APP_CHECK_FAILED, APP_STOPPED, APPCONFIG_ERROR, APPCONFIG_UNREACHABLE,
        BAD_OUTPUT, BAD_REQUEST, DUPLICATE_RECORD, FORBIDDEN, FREE_PLAN_PAUSED, GENERATION_FAILED, INTERNAL,
        INVALID_DSL, INVALID_RECORD, INVALID_TRANSACTION, NOT_FOUND, PAYLOAD_TOO_LARGE, PLAN_LIMIT_REACHED,
        PRO_REQUIRED, RATE_LIMITED, REFUSED, TEXT_NOT_ALLOWED, TIMEOUT, TRANSACTION_OWNED_BY_OTHER,
        UNAUTHENTICATED, UNPUBLISHED, UPSTREAM_TIMEOUT, UPSTREAM_UNAVAILABLE, VERSION_CONFLICT,
    )
}
