package jp.kz2601.metamon.desktop.gridedit

import jp.kz2601.metamon.desktop.GridArea
import jp.kz2601.metamon.dsl.AppDSL
import jp.kz2601.metamon.dsl.ComponentType
import jp.kz2601.metamon.dsl.PatchOp
import jp.kz2601.metamon.dsl.PatchOpOp
import jp.kz2601.metamon.dsl.Screen
import kotlinx.serialization.json.JsonPrimitive
import kotlin.math.roundToInt

// 並びを直す編集(#3206)の規則。画面に依存しない。iOS の MetamonDSL/Layout/GridEdit.swift と同じ動かし方・重なりの解き方・patch の形。
// 決まりは backend `dsl.checkGrid` と同じ: 6 列(A〜F)・行 1〜48・`area` を持つ兄弟同士でセルを共有しない。

/** スプレッドシート記法(`A1:C3`)。backend `Area.String()` と同じ形。 */
val GridArea.notation: String get() = "${'A' + col1}$row1:${'A' + col2}$row2"

/** セルを 1 つでも共有するか(隣り合うだけは重ならない)。 */
fun GridArea.overlaps(other: GridArea): Boolean =
    col1 <= other.col2 && other.col1 <= col2 && row1 <= other.row2 && other.row1 <= row2

/** ドラッグ量・矢印のキーをセル単位に丸める。 */
object GridEditMath {
    /** 動かす: 大きさはそのまま、左上をドラッグ量ぶん(セル単位に四捨五入)ずらし、グリッドの外に出ないよう寄せる。 */
    fun moved(a: GridArea, dx: Double, dy: Double, cellW: Double, cellH: Double): GridArea =
        nudged(a, steps(dx, cellW), steps(dy, cellH))

    /** 矢印のキー: [dc] 列・[dr] 行ずらす(グリッドの外には出さない)。 */
    fun nudged(a: GridArea, dc: Int, dr: Int): GridArea {
        val col1 = clamp(a.col1 + dc, 0, GridArea.COLUMNS - a.colSpan)
        val row1 = clamp(a.row1 + dr, 1, GridArea.MAX_ROWS - a.rowSpan + 1)
        return GridArea(col1, row1, col1 + a.colSpan - 1, row1 + a.rowSpan - 1)
    }

    /** 大きさを変える: 左上は固定、右下の角をドラッグ量ぶん動かす。1 × 1 より小さくせず、グリッドの外に出さない。 */
    fun resized(a: GridArea, dx: Double, dy: Double, cellW: Double, cellH: Double): GridArea {
        val col2 = clamp(a.col2 + steps(dx, cellW), a.col1, GridArea.COLUMNS - 1)
        val row2 = clamp(a.row2 + steps(dy, cellH), a.row1, GridArea.MAX_ROWS)
        return GridArea(a.col1, a.row1, col2, row2)
    }

    /** 四捨五入は 0.5 を 0 から遠い方へ(Swift の `rounded()` と同じ)。 */
    internal fun steps(d: Double, cell: Double): Int {
        if (cell <= 0 || !d.isFinite()) return 0
        val q = d / cell
        return if (q >= 0) q.roundToInt() else -(-q).roundToInt()
    }

    internal fun clamp(v: Int, lo: Int, hi: Int): Int = minOf(maxOf(v, lo), maxOf(lo, hi))
}

/**
 * 1 画面ぶんの編集中の配置。`area` を持つ直下の部品だけを扱う(area 無しで積まれた部品・Container の中は動かさない)。
 * 不変: 動かす・消すは新しい [GridEditDraft] を返す(置けない時は null)。
 */
data class GridEditDraft(
    val screenId: String,
    /** 画面の位置(`dsl.screens` の中)。 */
    val screenIndex: Int,
    val items: List<Item>,
    /** 画面の部品の数(area 無しも含む)。最後の 1 つは消させない(空の画面は DSL として不正)。 */
    val componentCount: Int,
) {
    data class Item(
        /** `screen.components` の中の位置(patch の path に使う)。 */
        val index: Int,
        val componentId: String,
        /** DSL に書かれていた `area` の文字列(patch の `test` に使う)。 */
        val originalText: String,
        val original: GridArea,
        val area: GridArea,
        val removed: Boolean = false,
        val type: ComponentType = ComponentType.UNKNOWN,
        /** 読み上げに使う見出し(title → label → text → placeholder の最初の空でないもの)。 */
        val title: String? = null,
    ) {
        val changed: Boolean get() = removed || area != original
    }

    /** 消していない部品。 */
    val placed: List<Item> get() = items.filter { !it.removed }
    val hasChanges: Boolean get() = items.any { it.changed }
    val canRemove: Boolean get() = componentCount - items.count { it.removed } > 1

    fun item(componentId: String): Item? = items.firstOrNull { it.componentId == componentId }

    /** `area` を [candidate] にした時に重なる、ほかの部品の id。 */
    fun conflicts(componentId: String, candidate: GridArea): List<String> =
        placed.filter { it.componentId != componentId && it.area.overlaps(candidate) }.map { it.componentId }

    /** 置けるなら置いた配置。重なる・消した部品なら null(backend の重なり禁止と同じ)。 */
    fun place(componentId: String, area: GridArea): GridEditDraft? {
        val i = items.indexOfFirst { it.componentId == componentId && !it.removed }
        if (i < 0 || conflicts(componentId, area).isNotEmpty()) return null
        return copy(items = items.toMutableList().also { it[i] = it[i].copy(area = area) })
    }

    fun remove(componentId: String): GridEditDraft? {
        val i = items.indexOfFirst { it.componentId == componentId && !it.removed }
        if (!canRemove || i < 0) return null
        return copy(items = items.toMutableList().also { it[i] = it[i].copy(removed = true) })
    }

    /**
     * 元の DSL からこの配置にする JSON Patch(RFC 6902・backend `dsl.PatchOp`)。
     * - 先頭で画面の id を `test`(端末とサーバーで画面の並びがずれていたら当てない)
     * - 動かした部品: 元の `area` を `test` → `replace`
     * - 消した部品: 元の `area` を `test` → `remove`。後ろから消す(前の部品の位置がずれない)。replace は全部その前に置く
     */
    val patch: List<PatchOp>
        get() {
            if (!hasChanges) return emptyList()
            val screen = "/screens/$screenIndex"
            val ops = mutableListOf(op(PatchOpOp.TEST, "$screen/id", screenId))
            for (item in items) {
                if (item.removed || item.area == item.original) continue
                val path = "$screen/components/${item.index}/area"
                ops += op(PatchOpOp.TEST, path, item.originalText)
                ops += op(PatchOpOp.REPLACE, path, item.area.notation)
            }
            for (item in items.filter { it.removed }.sortedByDescending { it.index }) {
                val path = "$screen/components/${item.index}"
                ops += op(PatchOpOp.TEST, "$path/area", item.originalText)
                ops += PatchOp(op = PatchOpOp.REMOVE, path = path)
            }
            return ops
        }

    /** 端末だけで当てる(サーバーの版が無い時の保存・編集中の描画)。 */
    fun applied(screen: Screen): Screen {
        val comps = screen.components.toMutableList()
        for (item in items) {
            if (item.index < comps.size && componentKey(comps[item.index].id, item.index) == item.componentId) {
                comps[item.index] = comps[item.index].copy(area = item.area.notation)
            }
        }
        val removed = items.filter { it.removed }.map { it.index }.toSet()
        return screen.copy(components = comps.filterIndexed { i, _ -> i !in removed })
    }

    fun applied(dsl: AppDSL): AppDSL {
        val s = dsl.screens.getOrNull(screenIndex)?.takeIf { it.id == screenId } ?: return dsl
        return dsl.copy(screens = dsl.screens.toMutableList().also { it[screenIndex] = applied(s) })
    }

    companion object {
        fun of(screen: Screen, screenIndex: Int): GridEditDraft = GridEditDraft(
            screenId = screen.id,
            screenIndex = screenIndex,
            componentCount = screen.components.size,
            items = screen.components.mapIndexedNotNull { i, c ->
                val text = c.area ?: return@mapIndexedNotNull null
                val a = GridArea.parse(text) ?: return@mapIndexedNotNull null
                val title = sequenceOf(c.title, c.label, c.text, c.placeholder).mapNotNull { it?.trim() }.firstOrNull { it.isNotEmpty() }
                Item(i, componentKey(c.id, i), text, a, a, type = c.type, title = title)
            },
        )

        /** id の無い部品は位置で見分ける(id は DSL では任意)。 */
        internal fun componentKey(id: String?, index: Int): String = id ?: "#$index"

        private fun op(op: PatchOpOp, path: String, value: String) = PatchOp(op = op, path = path, value = JsonPrimitive(value))
    }
}
