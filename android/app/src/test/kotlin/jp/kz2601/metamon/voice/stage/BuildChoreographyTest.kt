package jp.kz2601.metamon.voice.stage

import jp.kz2601.metamon.dsl.AppDSL
import jp.kz2601.metamon.dsl.decode
import jp.kz2601.metamon.renderer.layout.GridArea
import jp.kz2601.metamon.services.StreamedComponent
import jp.kz2601.metamon.voice.BuildProgress
import jp.kz2601.metamon.voice.stage.BuildChoreography.AssemblyPlan
import jp.kz2601.metamon.voice.stage.BuildChoreography.LiveDraft
import jp.kz2601.metamon.voice.stage.BuildChoreography.Move
import jp.kz2601.metamon.voice.stage.BuildChoreography.PartKind
import jp.kz2601.metamon.voice.stage.BuildChoreography.WaitingScript
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNotEquals
import org.junit.Assert.assertTrue
import org.junit.Test

/** iOS の BuildChoreographyTests・BuildLiveDraftTests の写し(#894)。 */
class BuildChoreographyTest {
    private fun dsl(components: String, tabs: String = "[]", screens: String = ""): AppDSL = AppDSL.decode(
        """{"schemaVersion":1,"app":{"name":"t","tabs":$tabs},"initialScreen":"home",
           "screens":[{"id":"home","components":[$components]}$screens]}""",
    )

    // MARK: 依頼文からの当て推量

    @Test
    fun hintsFollowWordOrderAndCapAtThree() {
        assertEquals(listOf(PartKind.FIELD, PartKind.CHART), BuildChoreography.hints("体重を記録してグラフで見たい"))
        assertEquals(listOf(PartKind.TOGGLE, PartKind.LIST, PartKind.STAT), BuildChoreography.hints("やることをチェックする一覧と合計と写真"))
        assertEquals(emptyList<PartKind>(), BuildChoreography.hints("こんにちは"))
        assertEquals(listOf(PartKind.TOGGLE), BuildChoreography.hints("todo"))
    }

    @Test
    fun trayPutsHintsFirstWithoutDuplicates() {
        val tray = BuildChoreography.tray(listOf(PartKind.CHART, PartKind.FIELD))
        assertEquals(6, tray.size)
        assertEquals(listOf(PartKind.CHART, PartKind.FIELD), tray.take(2))
        assertEquals(tray.size, tray.toSet().size)
        assertEquals(PartKind.HEADING, BuildChoreography.tray(emptyList()).first())
    }

    // MARK: 待っている間

    @Test
    fun waitingPlacesOneByOneThenSwapsReordersRelayouts() {
        val script = WaitingScript("グラフで見たい")
        val slots = WaitingScript.LAYOUTS[0].size
        for (step in 0 until slots) {
            val f = script.frame(step)
            assertEquals("1 手で 1 つずつ置く", step + 1, f.placed.size)
            assertEquals(Move.PLACE, f.move)
            assertEquals("置いた部品は箱から選んだもの", f.placed.last().kind, f.picking)
        }
        assertEquals("当て推量の部品から置く", PartKind.CHART, script.frame(0).placed.first().kind)
        assertEquals(Move.SWAP, script.frame(slots).move)
        assertEquals(Move.REORDER, script.frame(slots + 1).move)
        assertEquals(Move.RELAYOUT, script.frame(slots + 2).move)
        assertEquals(Move.SWAP, script.frame(slots + 3).move)
    }

    @Test
    fun waitingFramesStayValid() {
        val script = WaitingScript("家計簿")
        for (step in 0 until 40) {
            val f = script.frame(step)
            assertTrue(f.placed.size <= WaitingScript.LAYOUTS[0].size)
            assertEquals("id は重ならない(動きのアニメ用)", f.placed.size, f.placed.map { it.id }.toSet().size)
            f.placed.forEachIndexed { i, a ->
                assertTrue(a.area.row2 <= GridArea.VISIBLE_ROWS)
                f.placed.drop(i + 1).forEach { b -> assertFalse("step $step: 部品が重なった", a.area.overlaps(b.area)) }
            }
        }
        // 配置を変える手は同じ部品のまま場所だけ変わる
        val slots = WaitingScript.LAYOUTS[0].size
        val before = script.frame(slots + 1)
        val after = script.frame(slots + 2)
        assertEquals(before.placed.map { it.id }.toSet(), after.placed.map { it.id }.toSet())
        assertNotEquals(before.placed.map { it.area }, after.placed.map { it.area })
    }

    @Test
    fun stepPacingSlowsWithReduceMotion() {
        assertEquals(2, WaitingScript.step(1.7, reduceMotion = false))
        assertEquals(0, WaitingScript.step(1.7, reduceMotion = true))
        assertEquals(0, WaitingScript.step(-1.0, reduceMotion = false))
    }

    // MARK: 結果が届いた時

    @Test
    fun assemblyOrdersTopToBottomThenLeftToRight() {
        val d = dsl(
            """
            {"id":"add","type":"button","label":"記録する","area":"D11:F12"},
            {"id":"right","type":"stat","label":"合計","area":"D1:F2"},
            {"id":"left","type":"text","text":"家計簿","style":"title","area":"A1:C2"},
            {"id":"list","type":"list","area":"A3:F10"},
            {"id":"more","type":"chart"}
            """,
        )
        val plan = AssemblyPlan.make(d, reduceMotion = false)
        assertEquals(listOf("left", "right", "list", "add", "more"), plan.steps.map { it.part.id })
        assertEquals(listOf(PartKind.HEADING, PartKind.STAT, PartKind.LIST, PartKind.BUTTON, PartKind.CHART), plan.steps.map { it.part.kind })
        assertEquals("家計簿", plan.steps.first().part.caption)
        assertEquals("記録する", plan.steps[3].part.caption)
        assertEquals("area の無い部品は下に全幅で積む", GridArea.parse("A13:F16"), plan.steps[4].part.area)
        assertEquals(16, plan.rows)
        assertEquals(listOf(PartKind.HEADING, PartKind.STAT, PartKind.LIST, PartKind.BUTTON, PartKind.CHART), plan.tray)
        val delays = plan.steps.map { it.delay }
        assertEquals("上から順に置く", delays.sorted(), delays)
        assertTrue(plan.steps.last().delay < plan.total)
    }

    @Test
    fun assemblyTotalIsCappedForManyParts() {
        val many = (1..12).joinToString(",") { """{"id":"t$it","type":"text","text":"x","area":"A$it:F$it"}""" }
        val plan = AssemblyPlan.make(dsl(many), reduceMotion = false)
        assertTrue("待たせすぎない", plan.total <= 1.5)
        assertEquals("上限を超えた分は最後の手でまとめて置く", AssemblyPlan.MAX_STEPS, plan.steps.map { it.delay }.toSet().size)
        val few = AssemblyPlan.make(dsl("""{"id":"a","type":"button","area":"A1:F2"}"""), reduceMotion = false)
        assertTrue("部品が少なければ短い", few.total < plan.total)
        assertTrue(few.total >= AssemblyPlan.LEAD_IN + AssemblyPlan.HOLD)
    }

    @Test
    fun assemblyWithReduceMotionPlacesAllAtOnce() {
        val plan = AssemblyPlan.make(dsl("""{"id":"a","type":"button","area":"A1:F2"},{"id":"b","type":"list","area":"A3:F8"}"""), reduceMotion = true)
        assertEquals(2, plan.steps.size)
        assertTrue(plan.steps.all { it.delay == 0.0 })
        assertEquals(AssemblyPlan.REDUCED_TOTAL, plan.total, 0.0)
    }

    @Test
    fun assemblyUsesInitialTabScreenAndShowsTabs() {
        val tabs = """[{"screen":"home","title":"ホーム"},{"screen":"stats","title":"集計"}]"""
        val d = AppDSL.decode(
            """{"schemaVersion":1,"app":{"name":"t","tabs":$tabs},"initialScreen":"stats",
               "screens":[{"id":"home","components":[{"id":"h","type":"text","text":"x"}]},
                          {"id":"stats","components":[{"id":"s","type":"stat","area":"A1:F2"}]}]}""",
        )
        val plan = AssemblyPlan.make(d, reduceMotion = false)
        assertEquals(listOf("s"), plan.steps.map { it.part.id })
        assertEquals(listOf("ホーム", "集計"), plan.tabs)
    }

    @Test
    fun captionsAreShortened() {
        assertEquals("あいうえおかきくけこさ…", AssemblyPlan.shortCaption(listOf(null, "  ", "あいうえおかきくけこさしすせそ")))
        assertEquals("短い", AssemblyPlan.shortCaption(listOf(" 短い ")))
    }

    // MARK: 届いた本物の部品(仮置き)

    private fun streamed(index: Int, type: String, area: String? = null, id: String? = null, label: String? = null) =
        StreamedComponent(attempt = 1, screen = "home", index = index, id = id, type = type, area = area, label = label)

    @Test
    fun liveComponentsReplaceGuessesOncePerIndex() {
        val live = LiveDraft(
            BuildProgress(
                appName = "家計簿",
                components = listOf(
                    streamed(0, "text", "A1:F1", id = "title", label = "家計簿"),
                    streamed(0, "text", "A1:F1", id = "title"),
                    streamed(1, "list", "A2:F8", id = "items"),
                ),
            ),
        )
        assertEquals(listOf("title", "items"), live.parts.map { it.id })
        assertEquals(listOf(PartKind.HEADING, PartKind.LIST), live.parts.map { it.kind })
        assertEquals(PartKind.LIST, live.latestKind)
        val frame = WaitingScript("グラフ").frame(0, live)
        assertEquals("本物の部品を先に置く", listOf("title", "items"), frame.placed.take(2).map { it.id })
        assertTrue("下書きは本物と重ならない", frame.placed.drop(2).none { g -> live.parts.any { it.area.overlaps(g.area) } })
        assertEquals(listOf(PartKind.HEADING, PartKind.LIST), live.tray(listOf(PartKind.CHART)).take(2))
    }

    @Test
    fun livePartsWithoutAreaStackBelowAndUnknownTypesAreCards() {
        val live = LiveDraft(BuildProgress(components = listOf(streamed(0, "button", "A1:F2"), streamed(1, "chart"), streamed(2, "wormhole"))))
        assertEquals(GridArea.parse("A3:F6"), live.parts[1].area)
        assertEquals(PartKind.CARD, live.parts[2].kind)
        assertEquals("stream-2", live.parts[2].id)
    }

    @Test
    fun resultKeepsMatchingProvisionalPartsAndPlacesTheRest() {
        val live = LiveDraft(BuildProgress(components = listOf(streamed(0, "button", "A1:F2", id = "add"))))
        val d = dsl("""{"id":"add","type":"button","area":"A1:F2"},{"id":"list","type":"list","area":"A3:F10"}""")
        val plan = AssemblyPlan.make(d, reduceMotion = false, shown = live.parts)
        assertEquals("仮置きと同じ部品は置いたまま", 0.0, plan.steps.first { it.part.id == "add" }.delay, 0.0)
        assertTrue("残りは 1 つずつ置く", plan.steps.first { it.part.id == "list" }.delay > 0)
    }
}
