package jp.kz2601.metamon.renderer.components.social

import jp.kz2601.metamon.dsl.records.InterestState
import jp.kz2601.metamon.dsl.records.MatchBox
import jp.kz2601.metamon.dsl.records.MatchInterest
import jp.kz2601.metamon.dsl.records.Record
import kotlinx.coroutines.flow.StateFlow

/**
 * 仲間探し(iOS の `MatchingHost`・#507)。「つながりたい」を送る・承認・見送る・取り下げ・解除。
 * 持たない runtime(モック・プレビュー)ではボタンを出さない。18 歳以上の確認・失敗の理由はホストが出す。
 * 操作はホストが楽観的に先に反映し、失敗したら戻す。
 */
interface MatchingHost {
    /** 一覧・送信中・ブロックが変わるたびに進む。描画はこれを読んで描き直す。 */
    val matchingVersion: StateFlow<Int>

    /** プロフィールの行の状態。null = ボタンを出さない(自分の行・プロフィールでない entity・サインインしていない)。 */
    fun interestState(entity: String, record: Record): InterestState?

    /** 送信中(ボタンを押せなくする)。 */
    fun interestBusy(record: Record): Boolean

    /** 「つながりたい」(相手から届いていれば承認と同じ)。 */
    suspend fun sendInterest(entity: String, record: Record)

    /** box の一覧(ブロックした人は除く)。null = まだ読んでいない。 */
    fun matchItems(box: MatchBox): List<MatchInterest>?

    /** 読めず、一覧も無い。 */
    fun matchLoadFailed(box: MatchBox): Boolean

    suspend fun refreshMatches(boxes: Set<MatchBox>)

    suspend fun acceptInterest(item: MatchInterest)

    /** 届いたものを見送る(incoming)/ 送ったものを取り下げる(outgoing)。 */
    suspend fun declineInterest(item: MatchInterest, box: MatchBox)

    suspend fun unmatch(item: MatchInterest)
}
