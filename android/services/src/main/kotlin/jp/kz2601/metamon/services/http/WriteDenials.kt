package jp.kz2601.metamon.services.http

import kotlinx.serialization.json.JsonArray
import kotlinx.serialization.json.JsonPrimitive

/**
 * scope:shared の参加者(#254)まわりの 403(iOS の `SharedAccessDenial`)。`details.reason` で見分ける。
 * #378 の「作った人だけが変更できます」(reason 無しの forbidden)とは別の文言を出すために使う。
 */
enum class SharedAccessDenial(val wire: String) {
    /** 共有のデータに参加していない(作成者でも参加者でもない) */
    NOT_MEMBER("not_member"),

    /** import: リンクを送った人がもう参加していない */
    INVITER_LEFT("inviter_left"),

    /** import: 参加者が上限(100 人) */
    MEMBERS_FULL("members_full"),
    ;

    companion object {
        fun from(error: Throwable): SharedAccessDenial? {
            val api = error as? ApiError ?: return null
            if (api.status != 403) return null
            val reason = api.reason ?: return null
            return entries.firstOrNull { it.wire == reason }
        }
    }
}

/** 共有の記録の書き込みを、アプリの決まりでサーバーが断った理由(#272 #273・iOS の `RecordWriteDenial`)。 */
sealed interface RecordWriteDenial {
    /** 403 `admins_only`: 管理者だけが入れる・変える・消せる。`field` は断られた項目(分かる時) */
    data class AdminsOnly(val field: String?) : RecordWriteDenial

    /** 409 `duplicate`: entity の unique が同じ記録がもうある。`fields` は unique の field id */
    data class Duplicate(val fields: List<String>) : RecordWriteDenial

    companion object {
        fun from(error: Throwable): RecordWriteDenial? {
            val api = error as? ApiError.Server ?: return null
            return when {
                api.status == 403 && api.reason == "admins_only" -> AdminsOnly(api.field)
                api.status == 409 && api.reason == "duplicate" -> Duplicate(
                    (api.detail("fields") as? JsonArray).orEmpty()
                        .mapNotNull { (it as? JsonPrimitive)?.takeIf { p -> p.isString }?.content },
                )
                else -> null
            }
        }
    }
}
