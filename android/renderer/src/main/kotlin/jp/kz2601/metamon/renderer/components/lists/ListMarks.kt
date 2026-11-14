package jp.kz2601.metamon.renderer.components.lists

import jp.kz2601.metamon.dsl.records.Entity
import jp.kz2601.metamon.dsl.records.Mark
import jp.kz2601.metamon.dsl.records.MarkState
import jp.kz2601.metamon.dsl.records.Record

/**
 * 一覧の行の印(`item.mark`・#255)。iOS の `ListLeaf.toggleMark` と同じ楽観的 UI:
 * 押した瞬間に [tap] で上書きを置き、ホストの返事の後に [settle] で外す(成功ならストアが新しい値、失敗なら元の値に戻っている)。
 */
internal object ListMarks {
    /** 出す印。共有(shared / public)の entity で、`item.mark` が entity.marks に宣言されている時だけ。 */
    fun mark(markId: String?, entity: Entity?): Mark? {
        val id = markId ?: return null
        return entity?.takeIf { it.isShared }?.mark(id)
    }

    /** 表示する状態(押した直後の上書き > 記録の値)。 */
    fun shown(overrides: Map<String, MarkState>, row: Record, mark: String): MarkState = overrides[row.id] ?: row.markState(mark)

    /** 押した: 次の状態を上書きに置いて返す。 */
    fun tap(overrides: MutableMap<String, MarkState>, row: Record, mark: String): MarkState =
        shown(overrides, row, mark).toggled().also { overrides[row.id] = it }

    /** 返事が来た: 上書きがこの時の値のままなら外す(連打で別の値になっていたら残す)。 */
    fun settle(overrides: MutableMap<String, MarkState>, rowId: String, sent: MarkState) {
        if (overrides[rowId] == sent) overrides.remove(rowId)
    }

    /** アイコン(SF Symbols 名)。塗りつぶしの形があるのは heart だけなので、他は色で付けた状態を分ける。 */
    fun symbol(icon: String, on: Boolean): String {
        val base = when (icon) {
            "star", "bookmark", "eye", "flag" -> icon
            "thumbsup" -> "hand.thumbsup"
            "check" -> "checkmark.circle"
            else -> "heart"
        }
        return if (on && base == "heart") "heart.fill" else base
    }
}
