package jp.kz2601.metamon.dsl.nearby

import jp.kz2601.metamon.dsl.logic.graphemeBounds
import jp.kz2601.metamon.dsl.logic.graphemeCount
import jp.kz2601.metamon.dsl.logic.graphemePrefix
import jp.kz2601.metamon.dsl.logic.trimWhitespace

/**
 * 近距離共有・すれ違い通信で受け取る値の上限と、相手に見せる名前。iOS の `NearbyLimits` と同じ値。
 * 文字数は Swift の `Character`(書記素)で数える。
 */
object NearbyLimits {
    /** 表示名の上限(文字数)。端末の名前(相手の一覧に出る名前)は UTF-8 で 63 バイトまで。 */
    const val MAX_NAME_LENGTH = 63

    /** senderId・記録の id・appId・entityId・項目 id の上限(文字数)。 */
    const val MAX_ID_LENGTH = 128

    /** アプリ名・entity 名の上限(文字数)。 */
    const val MAX_LABEL_LENGTH = 100

    /** 受け取りの結果メッセージの上限(文字数)。 */
    const val MAX_MESSAGE_LENGTH = 200

    /** Header の項目一覧の上限(件数)。 */
    const val MAX_FIELDS = 200

    /** 名前を出さない時に表示名の代わりに送る決まった文言(言語に依らず同じ値を送り、表示する側で訳す)。 */
    const val ANONYMOUS_NAME = "近くの人"

    /** 前後の空白を落とし、先頭から [max] 文字。 */
    fun clip(s: String, max: Int): String = s.trimWhitespace().graphemePrefix(max)

    fun isValidId(s: String): Boolean = s.isNotEmpty() && s.graphemeCount() <= MAX_ID_LENGTH

    /** 相手に送る名前。匿名・空なら [ANONYMOUS_NAME]。 */
    fun publicName(name: String, anonymous: Boolean): String {
        val n = clip(name, MAX_NAME_LENGTH)
        return if (anonymous || n.isEmpty()) ANONYMOUS_NAME else n
    }

    /** 近距離共有の端末の名前と Header の送り主に使う名前。[publicName] を UTF-8 で 63 バイト以内に切る。 */
    fun peerName(name: String, anonymous: Boolean): String {
        var n = publicName(name, anonymous)
        while (n.toByteArray(Charsets.UTF_8).size > MAX_NAME_LENGTH) n = n.substring(0, n.graphemeBounds().last().first)
        return n
    }

    /**
     * 受け取った名前を画面に出す形に。匿名の印(と空)は [anonymousLabel](今の言語の「近くの人」)にする。
     * 文言の訳はアプリ側のリソースにあるので、呼ぶ側が渡す。
     */
    fun displayName(received: String, anonymousLabel: String = ANONYMOUS_NAME): String {
        val n = clip(received, MAX_NAME_LENGTH)
        return if (n.isEmpty() || n == ANONYMOUS_NAME) anonymousLabel else n
    }
}
