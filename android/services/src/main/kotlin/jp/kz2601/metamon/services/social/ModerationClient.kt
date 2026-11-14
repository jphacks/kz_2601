package jp.kz2601.metamon.services.social

import kotlinx.serialization.Serializable

/** 通報の理由(`POST /v1/reports` の reason・#321)。並びは iOS の選択肢と同じ。 */
enum class ReportReason(val wire: String) {
    SPAM("spam"),
    SEXUAL("sexual"),
    VIOLENCE("violence"),
    HARASSMENT("harassment"),
    ILLEGAL("illegal"),
    PRIVACY("privacy"),
    OTHER("other"),
}

/** 通報する対象(#321)。 */
sealed interface ReportTarget {
    /** 「探す」の掲載 */
    data class Listing(val id: String) : ReportTarget

    /** 共有アプリの他人の投稿 */
    data class RecordTarget(val appId: String, val entityId: String, val recordId: String) : ReportTarget
}

/** ブロック中のユーザー(`GET /v1/me/blocks` の items・appconfig `Block`)。 */
@Serializable
data class BlockedUser(val uid: String, val name: String = "")

/** 通報・ブロック(appconfig の `moderation` タグ・#321)。iOS の `ModerationClient`。 */
interface ModerationClient {
    /** 通報する。同じ対象を通報し直しても成功(サーバーは 1 件にまとめる)。 */
    suspend fun report(target: ReportTarget, reason: ReportReason)

    suspend fun blockedUsers(): List<BlockedUser>

    /** 投稿の作者をブロックする。 */
    suspend fun block(uid: String, name: String?): BlockedUser

    /** 掲載者をブロックする(uid はサーバーが引く)。 */
    suspend fun blockListing(listingId: String): BlockedUser

    suspend fun unblock(uid: String)
}
