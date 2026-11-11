package jp.kz2601.metamon.dsl.records

import jp.kz2601.metamon.dsl.ActionType
import jp.kz2601.metamon.dsl.AppDSL
import jp.kz2601.metamon.dsl.Component

/** follow / unfollow(#270)。 */
val ActionType.isFollow: Boolean get() = this == ActionType.FOLLOW || this == ActionType.UNFOLLOW

/**
 * このアプリがフォローを読むか(filter の `{"myFollows": true}` か follow / unfollow のボタン・iOS の `AppDSL.usesFollows`)。
 * 読む時だけ一覧を取りに行く。
 */
val AppDSL.usesFollows: Boolean
    get() {
        fun uses(c: Component): Boolean =
            c.filters.any { it.isMyFollows } || c.action?.type?.isFollow == true || c.onItemTap?.type?.isFollow == true ||
                c.children.any(::uses)
        return screens.any { s -> s.toolbar.any { it.action.type.isFollow } || s.components.any(::uses) }
    }
