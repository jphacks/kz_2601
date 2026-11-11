package jp.kz2601.metamon.desktop

import androidx.compose.ui.input.key.Key
import jp.kz2601.metamon.desktop.io.Tsv

// list・table の行を選ぶ(#1766): クリック・Shift+クリックで範囲・Ctrl(macOS は ⌘)+クリックで足し引き・Ctrl+A で全部。

/**
 * 選んだ行。[ids] は選んだ時の行の集まり(行が増えた・消えた・絞り込みが変わったら選びを外す)。
 * [anchor] は Shift+クリックの起点。
 */
data class RowSelection(
    val selected: Set<String> = emptySet(),
    val anchor: String? = null,
    val ids: Set<String> = emptySet(),
) {
    val isEmpty: Boolean get() = selected.isEmpty()

    /** 今の並びで選んだ行(コピーの順)。 */
    fun inOrder(order: List<String>): List<String> = order.filter { it in selected }
}

/** 行の押し方。toggle は Windows・Linux の Ctrl、macOS の ⌘。 */
data class RowClick(val shift: Boolean = false, val toggle: Boolean = false)

object RowSelect {
    enum class Command { SELECT_ALL, COPY }

    /** 行の集まりが変わったら(足した・消した・検索で絞った)選びを外す。並べ替えだけなら残す。 */
    fun sync(s: RowSelection, order: List<String>): RowSelection {
        val ids = order.toSet()
        return if (s.ids == ids) s else RowSelection(ids = ids)
    }

    /** 押した行 [id]。[order] は見えている並び。 */
    fun click(s: RowSelection, id: String, order: List<String>, how: RowClick): RowSelection {
        val base = sync(s, order)
        if (how.shift) {
            val from = base.anchor?.let(order::indexOf)?.takeIf { it >= 0 }
            val to = order.indexOf(id)
            if (from == null || to < 0) return base.copy(selected = setOf(id), anchor = id)
            val range = order.subList(minOf(from, to), maxOf(from, to) + 1).toSet()
            // Ctrl+Shift は今の選びに足す。Shift だけなら置き換える(起点は動かさない)
            return base.copy(selected = if (how.toggle) base.selected + range else range)
        }
        if (how.toggle) {
            return base.copy(selected = if (id in base.selected) base.selected - id else base.selected + id, anchor = id)
        }
        return base.copy(selected = setOf(id), anchor = id)
    }

    /** 押すと詳細へ移る行: 選びは外し、Shift+クリックの起点だけ覚える。 */
    fun tapped(s: RowSelection, id: String, order: List<String>): RowSelection = sync(s, order).copy(selected = emptySet(), anchor = id)

    fun selectAll(s: RowSelection, order: List<String>): RowSelection = sync(s, order).copy(selected = order.toSet())

    /** 一覧に効くキー。Ctrl+A・Ctrl+C(macOS は ⌘)。他は null。 */
    fun command(p: KeyPress, os: HostOs): Command? {
        val chord = when (p.key) {
            Key.A -> Command.SELECT_ALL
            Key.C -> Command.COPY
            else -> return null
        }
        return chord.takeIf { Keymap.matches(KeyChord(p.key, primary = true), p, os) }
    }

    /** マウスの修飾キー → 押し方。 */
    fun click(shift: Boolean, ctrl: Boolean, meta: Boolean, os: HostOs): RowClick = RowClick(shift, if (os == HostOs.MAC) meta else ctrl)
}

/** table の選んだ行の TSV(見出し付き・見えている列と文字そのまま・今の並び)。 */
object TableCopy {
    fun tsv(headers: List<String>, rows: List<TableCells.Row>, selected: RowSelection): String {
        val chosen = rows.filter { it.id in selected.selected }
        return Tsv.string(headers, chosen.map { r -> headers.indices.map { r.texts.getOrElse(it) { "" } } })
    }
}
