package jp.kz2601.metamon.dsl.records

import jp.kz2601.metamon.dsl.Show
import kotlinx.serialization.Serializable

/** `matchList.show`(#507)。届いた / 送った(`GET /interests?box=`)・つながった(`GET /matches`)。iOS の Model/Matching.swift。 */
enum class MatchBox(val wire: String) {
    INCOMING("incoming"),
    OUTGOING("outgoing"),
    MATCHES("matches"),
    ;

    companion object {
        fun of(show: Show?): MatchBox? = entries.firstOrNull { it.wire == show?.wire }
    }
}

/**
 * 相手 1 人とのつながり(appconfig `Interest`・`matching` タグ)。断られた・ブロックされたことは返らない(送った側には pending のまま)。
 */
@Serializable
data class MatchInterest(
    /** 相手の uid */
    val uid: String,
    /** 相手のプロフィールのレコード id(行はこのレコードで描く)。消されていることがある */
    val profileRecordId: String? = null,
    /** pending | matched */
    val status: String = PENDING,
    val updatedAt: String? = null,
    val matchedAt: String? = null,
) {
    val isMatched: Boolean get() = status == MATCHED

    companion object {
        const val PENDING = "pending"
        const val MATCHED = "matched"
    }
}

/** プロフィールの行から見た、その人とのつながり(`list.item.interest` のボタンの出し分け)。 */
enum class InterestState {
    /** まだ何も無い → 「つながりたい」 */
    NONE,

    /** 送って返事待ち → 「送りました」 */
    SENT,

    /** 相手から届いている → 「承認する」(押すとつながる) */
    INCOMING,

    /** つながった → 「つながりました」 */
    MATCHED,
    ;

    /** 押せる(送る・承認する)。 */
    val tappable: Boolean get() = this == NONE || this == INCOMING
}
