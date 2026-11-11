package jp.kz2601.metamon.renderer.components.social

import jp.kz2601.metamon.dsl.Action
import jp.kz2601.metamon.dsl.ActionType
import jp.kz2601.metamon.dsl.records.Filter
import jp.kz2601.metamon.dsl.records.isFollow
import jp.kz2601.metamon.renderer.DSLRuntime
import kotlinx.coroutines.flow.StateFlow

/**
 * フォロー(iOS の `FollowsHost`・#270)。持たない runtime(モック・プレビュー)では、フォロー中の一覧は空・
 * フォローのボタンは出さない。:app の AppRuntime が実装する。
 */
interface FollowsHost {
    /** フォローの一覧・送信中が変わるたびに進む。描画はこれを読んで描き直す。 */
    val followsVersion: StateFlow<Int>

    /** このアプリでフォローしている人の uid。null = まだ読めていない(読み込みはホストが始める)。 */
    fun followedAuthors(): Set<String>?

    /** 投稿を書いた人をフォローしているか。null = ボタンを出さない(自分の投稿・作者の無い投稿・読めていない・サインインしていない)。 */
    fun followState(entity: String, recordId: String): Boolean?
}

val DSLRuntime.follows: FollowsHost? get() = this as? FollowsHost

/** `{"myFollows": true}` を解く uid(使っていなければ一覧を読みに行かない)。 */
fun DSLRuntime.followedFor(filters: List<Filter>): Set<String>? =
    if (filters.any { it.isMyFollows }) follows?.followedAuthors() else null

/** follow / unfollow のボタンを出すか。今の状態に合う方だけ(フォロー済みなら unfollow、まだなら follow)。 */
fun DSLRuntime.showsFollowButton(action: Action?, recordId: String?): Boolean {
    if (action == null || !action.type.isFollow) return true
    val entity = action.entity ?: return false
    val following = recordId?.let { follows?.followState(entity, it) } ?: return false
    return following == (action.type == ActionType.UNFOLLOW)
}
