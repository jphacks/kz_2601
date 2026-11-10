package jp.kz2601.metamon.renderer.components.lists

import jp.kz2601.metamon.dsl.records.Record
import jp.kz2601.metamon.renderer.components.social.ModerationHost

/**
 * `list` の行の長押しメニュー(iOS の ListLeaf の contextMenu)。
 * 削除(`swipeToDelete` で消せる記録)→ 通報・ブロック(他人の共有の記録・[ModerationHost] を持つ runtime だけ)の順。
 */
object ListRowMenu {
    enum class Item { DELETE, REPORT, BLOCK }

    fun items(deletable: Boolean, moderation: ModerationHost?, entity: String, record: Record): List<Item> = buildList {
        if (deletable) add(Item.DELETE)
        if (moderation?.canReport(entity, record) == true) {
            add(Item.REPORT)
            add(Item.BLOCK)
        }
    }
}
