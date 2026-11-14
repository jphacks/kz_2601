package jp.kz2601.metamon.desktop.gridedit

import jp.kz2601.metamon.desktop.GridArea
import jp.kz2601.metamon.desktop.gridPlacement
import jp.kz2601.metamon.dsl.DslJson
import jp.kz2601.metamon.dsl.PatchOp
import jp.kz2601.metamon.dsl.PatchOpOp
import jp.kz2601.metamon.dsl.Screen
import kotlinx.serialization.builtins.ListSerializer
import kotlinx.serialization.json.JsonPrimitive
import kotlinx.serialization.json.jsonArray
import kotlinx.serialization.json.jsonObject
import kotlinx.serialization.json.jsonPrimitive
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

/** 並びを直す編集の規則(#3206)。iOS の GridEditTests.swift と同じ入力で同じ area・patch になるか。 */
class GridEdit3206Test {
    private fun area(s: String): GridArea = GridArea.parse(s)!!

    @Test
    fun notationRoundTrips() {
        for (s in listOf("A1:C3", "D11:F12", "A1:F48", "B7:B7")) assertEquals(s, area(s).notation)
    }

    // 吸着

    @Test
    fun moveSnapsToNearestCell() {
        val a = area("A1:B2")
        // 0.4 セルは動かない・0.6 セルは 1 つ動く
        assertEquals("A1:B2", GridEditMath.moved(a, 40.0, 20.0, 100.0, 50.0).notation)
        assertEquals("B2:C3", GridEditMath.moved(a, 60.0, 30.0, 100.0, 50.0).notation)
        assertEquals("C6:D7", GridEditMath.moved(a, 210.0, 260.0, 100.0, 50.0).notation)
    }

    @Test
    fun halfCellRoundsAwayFromZeroLikeSwift() {
        assertEquals("B1:C2", GridEditMath.moved(area("A1:B2"), 50.0, 0.0, 100.0, 50.0).notation)
        assertEquals("A1:B2", GridEditMath.moved(area("B1:C2"), -50.0, 0.0, 100.0, 50.0).notation)
    }

    @Test
    fun moveStaysInsideGrid() {
        assertEquals("A1:B2", GridEditMath.moved(area("B2:C3"), -1000.0, -1000.0, 100.0, 50.0).notation)
        assertEquals("E2:F3", GridEditMath.moved(area("B2:C3"), 1000.0, 0.0, 100.0, 50.0).notation)
        assertEquals("A47:F48", GridEditMath.moved(area("A1:F2"), 0.0, 100_000.0, 100.0, 50.0).notation)
    }

    @Test
    fun arrowKeysMoveOneCellAndStayInside() {
        assertEquals("C2:D3", GridEditMath.nudged(area("B2:C3"), 1, 0).notation)
        assertEquals("B3:C4", GridEditMath.nudged(area("B2:C3"), 0, 1).notation)
        assertEquals("A1:B2", GridEditMath.nudged(area("A1:B2"), -1, -1).notation)
        assertEquals("E47:F48", GridEditMath.nudged(area("E47:F48"), 1, 1).notation)
    }

    @Test
    fun resizeKeepsTopLeftAndMinimumOneCell() {
        assertEquals("B2:D5", GridEditMath.resized(area("B2:C3"), 100.0, 100.0, 100.0, 50.0).notation)
        assertEquals("B2:B2", GridEditMath.resized(area("B2:C3"), -1000.0, -1000.0, 100.0, 50.0).notation)
        assertEquals("B2:F48", GridEditMath.resized(area("B2:C3"), 1000.0, 100_000.0, 100.0, 50.0).notation)
    }

    @Test
    fun zeroCellSizeDoesNotMove() {
        assertEquals("B2:C3", GridEditMath.moved(area("B2:C3"), 50.0, 50.0, 0.0, 0.0).notation)
        assertEquals("B2:C3", GridEditMath.resized(area("B2:C3"), Double.NaN, Double.POSITIVE_INFINITY, 10.0, 10.0).notation)
    }

    // 重なり

    private fun screen(): Screen = DslJson.decodeFromString(
        Screen.serializer(),
        """
        {"id":"home","title":"ホーム","components":[
          {"id":"summary","type":"text","text":"合計","area":"A1:F2"},
          {"id":"note","type":"text","text":"積む"},
          {"id":"recent","type":"text","text":"一覧","area":"A3:F10"},
          {"id":"add","type":"button","label":"追加","area":"D11:F12","action":{"type":"navigate","screen":"home"}}
        ]}
        """.trimIndent(),
    )

    @Test
    fun draftTakesOnlyPlacedComponents() {
        val d = GridEditDraft.of(screen(), 1)
        assertEquals(listOf("summary", "recent", "add"), d.items.map { it.componentId })
        assertEquals(listOf(0, 2, 3), d.items.map { it.index })
        assertEquals(listOf("合計", "一覧", "追加"), d.items.map { it.title })
        assertFalse(d.hasChanges)
        assertEquals(emptyList<PatchOp>(), d.patch)
    }

    @Test
    fun overlapIsRefusedLikeBackend() {
        val d = GridEditDraft.of(screen(), 0)
        assertEquals(listOf("recent"), d.conflicts("add", area("A10:C11")))
        assertNull(d.place("add", area("A10:C11")))
        assertEquals("D11:F12", d.item("add")?.area?.notation)
        // 隣り合うだけ(セルを共有しない)は置ける
        val next = d.place("add", area("A11:C12"))
        assertNotNull(next)
        assertEquals("A11:C12", next!!.item("add")?.area?.notation)
        // 自分の元の場所とは重なってよい
        assertNotNull(next.place("add", area("A11:D12")))
    }

    @Test
    fun removedComponentNoLongerBlocks() {
        val d = GridEditDraft.of(screen(), 0).remove("recent")!!
        assertTrue(d.conflicts("add", area("A5:F12")).isEmpty())
        assertNull("消した部品は動かせない", d.place("recent", area("A3:F4")))
    }

    @Test
    fun lastComponentCannotBeRemoved() {
        val s = DslJson.decodeFromString(Screen.serializer(), """{"id":"s","title":"","components":[{"type":"text","text":"x","area":"A1:F2"}]}""")
        val d = GridEditDraft.of(s, 0)
        assertFalse(d.canRemove)
        assertNull(d.remove(d.items[0].componentId))
        // id の無い部品は位置で見分ける
        assertEquals("#0", d.items[0].componentId)
    }

    // JSON Patch

    private fun op(op: PatchOpOp, path: String, value: String? = null) = PatchOp(op = op, path = path, value = value?.let(::JsonPrimitive))

    @Test
    fun patchReplacesAreaAndRemovesFromTheBack() {
        val d = GridEditDraft.of(screen(), 1).remove("summary")!!.place("add", area("A11:C12"))!!.remove("recent")!!
        assertEquals(
            listOf(
                op(PatchOpOp.TEST, "/screens/1/id", "home"),
                op(PatchOpOp.TEST, "/screens/1/components/3/area", "D11:F12"),
                op(PatchOpOp.REPLACE, "/screens/1/components/3/area", "A11:C12"),
                op(PatchOpOp.TEST, "/screens/1/components/2/area", "A3:F10"),
                op(PatchOpOp.REMOVE, "/screens/1/components/2"),
                op(PatchOpOp.TEST, "/screens/1/components/0/area", "A1:F2"),
                op(PatchOpOp.REMOVE, "/screens/1/components/0"),
            ),
            d.patch,
        )
    }

    @Test
    fun movingBackToOriginalIsNoChange() {
        val d = GridEditDraft.of(screen(), 0).place("add", area("A11:C12"))!!.place("add", area("D11:F12"))!!
        assertFalse(d.hasChanges)
        assertEquals(emptyList<PatchOp>(), d.patch)
    }

    @Test
    fun patchEncodesAsRFC6902() {
        val text = DslJson.encodeToString(
            ListSerializer(PatchOp.serializer()),
            listOf(op(PatchOpOp.REPLACE, "/screens/0/components/1/area", "A1:B2"), op(PatchOpOp.REMOVE, "/screens/0/components/2")),
        )
        val arr = DslJson.parseToJsonElement(text).jsonArray
        assertEquals("replace", arr[0].jsonObject["op"]!!.jsonPrimitive.content)
        assertEquals("/screens/0/components/1/area", arr[0].jsonObject["path"]!!.jsonPrimitive.content)
        assertEquals("A1:B2", arr[0].jsonObject["value"]!!.jsonPrimitive.content)
        assertEquals("remove", arr[1].jsonObject["op"]!!.jsonPrimitive.content)
        assertFalse("remove に value は出さない", "value" in arr[1].jsonObject)
    }

    @Test
    fun appliedMatchesPatch() {
        val d = GridEditDraft.of(screen(), 0).place("add", area("A11:C12"))!!.remove("summary")!!
        val out = d.applied(screen())
        assertEquals(listOf("note", "recent", "add"), out.components.map { it.id })
        assertEquals(listOf(null, "A3:F10", "A11:C12"), out.components.map { it.area })
        val placed = out.gridPlacement.placed
        assertTrue(placed.none { p -> placed.any { q -> q !== p && q.area.overlaps(p.area) } })
    }
}
