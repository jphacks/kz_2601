package jp.kz2601.metamon.desktop.gridedit

import jp.kz2601.metamon.desktop.GridArea
import jp.kz2601.metamon.desktop.gridPlacement
import jp.kz2601.metamon.desktop.isVoiceScreen
import jp.kz2601.metamon.dsl.AppDSL
import jp.kz2601.metamon.dsl.DslJson
import jp.kz2601.metamon.dsl.PatchOp
import jp.kz2601.metamon.dsl.PatchOpOp
import jp.kz2601.metamon.dsl.Screen
import jp.kz2601.metamon.dsl.screen
import kotlinx.serialization.builtins.ListSerializer
import kotlinx.serialization.json.JsonPrimitive
import java.io.File

// 並びを直す編集の保存(#3207)。サインイン前(同梱の DSL)なので、iOS のモックと同じく端末だけで当てる。
// 保存の形は DSL の patch(GridEditDraft.patch)のまま。アプリごとに保存した順に積み、起動の時に同梱の DSL へ当て直す。

/** 編集できる画面か(iOS の `isGridEditable` と同じ: フォームでも音声でもなく、area 付きの部品がある)。 */
val Screen.isGridEditable: Boolean get() = entity == null && !isVoiceScreen && gridPlacement.placed.isNotEmpty()

/** GridEditDraft.patch が出す形(画面の id・部品の area・部品を消す)だけを当てる。 */
object GridEditPatch {
    private val screenId = Regex("""^/screens/(\d+)/id$""")
    private val area = Regex("""^/screens/(\d+)/components/(\d+)/area$""")
    private val component = Regex("""^/screens/(\d+)/components/(\d+)$""")

    /** 当てた DSL。test が合わない・知らない操作なら null(何も当てない)。 */
    fun apply(dsl: AppDSL, ops: List<PatchOp>): AppDSL? {
        var out = dsl
        for (op in ops) out = applyOne(out, op) ?: return null
        return out
    }

    private fun applyOne(dsl: AppDSL, op: PatchOp): AppDSL? {
        val value = (op.value as? JsonPrimitive)?.takeIf { it.isString }?.content
        screenId.matchEntire(op.path)?.let { m ->
            val s = dsl.screens.getOrNull(m.groupValues[1].toInt()) ?: return null
            return if (op.op == PatchOpOp.TEST && s.id == value) dsl else null
        }
        area.matchEntire(op.path)?.let { m ->
            val si = m.groupValues[1].toInt()
            val ci = m.groupValues[2].toInt()
            val c = dsl.screens.getOrNull(si)?.components?.getOrNull(ci) ?: return null
            return when (op.op) {
                PatchOpOp.TEST -> if (c.area == value) dsl else null
                PatchOpOp.REPLACE -> if (value != null && GridArea.parse(value) != null) withComponents(dsl, si) { it[ci] = c.copy(area = value) } else null
                else -> null
            }
        }
        component.matchEntire(op.path)?.let { m ->
            val si = m.groupValues[1].toInt()
            val ci = m.groupValues[2].toInt()
            val comps = dsl.screens.getOrNull(si)?.components ?: return null
            // 最後の 1 つは消さない(空の画面は DSL として不正)
            if (op.op != PatchOpOp.REMOVE || ci !in comps.indices || comps.size <= 1) return null
            return withComponents(dsl, si) { it.removeAt(ci) }
        }
        return null
    }

    private fun withComponents(dsl: AppDSL, si: Int, edit: (MutableList<jp.kz2601.metamon.dsl.Component>) -> Unit): AppDSL {
        val s = dsl.screens[si]
        val comps = s.components.toMutableList().also(edit)
        return dsl.copy(screens = dsl.screens.toMutableList().also { it[si] = s.copy(components = comps) })
    }
}

/**
 * アプリごとの並びの直し(`<保存先>/layouts/<appId>.json`・保存した順の patch の列)。
 * @param dir null ならメモリだけ(テスト用)
 */
class LayoutEditStore(private val dir: File?) {
    private val memory = HashMap<String, List<List<PatchOp>>>()
    private val serializer = ListSerializer(ListSerializer(PatchOp.serializer()))

    @Synchronized
    fun load(appId: String): List<List<PatchOp>> = memory.getOrPut(appId) {
        val f = file(appId)?.takeIf { it.isFile } ?: return@getOrPut emptyList()
        runCatching { DslJson.decodeFromString(serializer, f.readText()) }.getOrElse {
            System.err.println("[TailorIt] 並びの直しを読めませんでした: ${it::class.simpleName}")
            emptyList()
        }
    }

    /** 1 回の保存ぶんを積む。空なら何もしない。 */
    @Synchronized
    fun append(appId: String, ops: List<PatchOp>) {
        if (ops.isEmpty()) return
        val next = load(appId) + listOf(ops)
        memory[appId] = next
        val f = file(appId) ?: return
        f.parentFile?.mkdirs()
        val tmp = File(f.parentFile, "${f.name}.tmp")
        tmp.writeText(DslJson.encodeToString(serializer, next))
        if (!tmp.renameTo(f)) {
            f.delete()
            tmp.renameTo(f)
        }
    }

    /** 同梱の DSL に積んだ直しを順に当てる。当たらない回(同梱の DSL が変わった)は飛ばす。 */
    fun applyTo(appId: String, dsl: AppDSL): AppDSL =
        load(appId).fold(dsl) { acc, ops -> GridEditPatch.apply(acc, ops) ?: acc }

    /** 全部のアプリの直しを消す(「この端末のデータを消す」)。 */
    @Synchronized
    fun removeAll() {
        memory.clear()
        dir?.deleteRecursively()
    }

    private fun file(appId: String): File? {
        val safe = appId.replace(Regex("[^A-Za-z0-9._-]"), "_").takeIf { it.isNotEmpty() && it != "." && it != ".." } ?: return null
        return dir?.resolve("$safe.json")
    }
}

/** 今の画面で編集を始める時の下書き。編集できない画面なら null。 */
fun AppDSL.gridEditDraft(screenId: String): GridEditDraft? {
    val index = screens.indexOfFirst { it.id == screenId }
    val s = screen(screenId)?.takeIf { it.isGridEditable } ?: return null
    return GridEditDraft.of(s, index)
}

/** 読み上げ: 「名前 行 r・列 c」の行・列(列は A〜F の文字)。 */
fun GridEditDraft.Item.positionLabel(): Pair<Int, Char> = area.row1 to ('A' + area.col1)

/** 編集の画面の状態(選んでいる部品・矢印のキー・Tab)。画面に依存しない。 */
data class GridEditSession(val draft: GridEditDraft, val selected: String? = draft.placed.firstOrNull()?.componentId) {
    enum class Nudge { MOVED, BLOCKED, EDGE, NONE }

    val selectedItem: GridEditDraft.Item? get() = selected?.let(draft::item)?.takeIf { !it.removed }

    /** Tab / Shift+Tab: 読む順(行 → 列)で次 / 前の部品を選ぶ。 */
    fun cycle(forward: Boolean): GridEditSession {
        val order = draft.placed.sortedWith(compareBy({ it.area.row1 }, { it.area.col1 }, { it.index }))
        if (order.isEmpty()) return copy(selected = null)
        val i = order.indexOfFirst { it.componentId == selected }
        val next = if (i < 0) 0 else (i + if (forward) 1 else -1 + order.size) % order.size
        return copy(selected = order[next].componentId)
    }

    /** 矢印のキー: 選んだ部品を 1 マス。重なる時は動かさない(BLOCKED)、端では EDGE。 */
    fun nudge(dc: Int, dr: Int): Pair<GridEditSession, Nudge> {
        val item = selectedItem ?: return this to Nudge.NONE
        val to = GridEditMath.nudged(item.area, dc, dr)
        if (to == item.area) return this to Nudge.EDGE
        val next = draft.place(item.componentId, to) ?: return this to Nudge.BLOCKED
        return copy(draft = next) to Nudge.MOVED
    }

    /** ドラッグを離した: 置けたら置く。重なれば元のまま(false)。 */
    fun drop(componentId: String, to: GridArea): Pair<GridEditSession, Boolean> {
        val next = draft.place(componentId, to) ?: return copy(selected = componentId) to false
        return copy(draft = next, selected = componentId) to true
    }
}
