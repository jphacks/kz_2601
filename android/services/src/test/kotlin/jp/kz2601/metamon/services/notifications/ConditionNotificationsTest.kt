package jp.kz2601.metamon.services.notifications

import jp.kz2601.metamon.dsl.AppDSL
import jp.kz2601.metamon.dsl.DslJson
import jp.kz2601.metamon.dsl.records.JSONValue
import jp.kz2601.metamon.dsl.records.Record
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test
import java.time.Instant

/** iOS の ConditionNotificationTests と同じ入力・期待値(#265・#886)。 */
class ConditionNotificationsTest {
    private val inventoryJSON = """
    {"schemaVersion":1,"app":{"name":"在庫","voiceEntry":"floating","notifications":[
      {"id":"low","title":"在庫が少なくなりました","body":"5 個を下回った商品があります","repeat":"once",
       "when":{"op":"gt","args":[{"agg":"count","entity":"items","filter":{"field":"stock","op":"lt","value":5}},0]}},
      {"id":"daily","title":"記録","body":"","repeat":"daily","time":"20:00"}]},
     "initialScreen":"home",
     "entities":[
      {"id":"items","name":"商品","fields":[{"id":"name","name":"名前","type":"string"},
        {"id":"stock","name":"在庫","type":"integer","computed":{"agg":"sum","entity":"moves","field":"qty","filter":{"field":"item","op":"eq","value":{"ref":"id"}}}}]},
      {"id":"moves","name":"入出荷","fields":[{"id":"item","name":"商品","type":"ref","refEntity":"items"},{"id":"qty","name":"数","type":"integer"}]}],
     "screens":[{"id":"home","title":"在庫","components":[{"type":"list","entity":"items"}]}]}
    """

    private fun dsl(): AppDSL = DslJson.decodeFromString(AppDSL.serializer(), inventoryJSON)

    @Test
    fun transitionFiresOncePerFalseToTrue() {
        assertEquals(emptyList<String>(), ConditionNotifications.transition(emptyMap(), mapOf("low" to true)).first)
        assertEquals(listOf("low"), ConditionNotifications.transition(mapOf("low" to false), mapOf("low" to true)).first)
        assertEquals(emptyList<String>(), ConditionNotifications.transition(mapOf("low" to true), mapOf("low" to true)).first)
        val reset = ConditionNotifications.transition(mapOf("low" to true), mapOf("low" to false))
        assertEquals(emptyList<String>(), reset.first)
        assertEquals(mapOf("low" to false), reset.second)
        assertEquals(mapOf("low" to false), ConditionNotifications.transition(mapOf("gone" to false), mapOf("low" to false)).second)
    }

    /** 完了の条件: 出荷を記録して在庫が閾値を下回ると、その保存の後の評価で出る。 */
    @Test
    fun shipmentBelowThresholdFires() {
        val dsl = dsl()
        val spec = dsl.app.notifications.first { it.id == "low" }
        assertTrue(spec.isConditional)
        assertFalse(spec.isRecordBased)
        val items = listOf(Record("pen", mapOf("name" to JSONValue.Str("ペン"))))
        val moves = mutableListOf(Record("in1", mapOf("item" to JSONValue.Str("pen"), "qty" to JSONValue.Num(10.0))))
        var previous = emptyMap<String, Boolean>()
        fun step(): List<String> {
            val current = ConditionNotifications.evaluate(dsl.app.notifications, dsl, mapOf("items" to items, "moves" to moves.toList()), Instant.EPOCH)
            assertNull("時刻の通知は評価しない", current["daily"])
            val (fire, next) = ConditionNotifications.transition(previous, current)
            previous = next
            return fire
        }
        fun move(id: String, qty: Double) = moves.add(Record(id, mapOf("item" to JSONValue.Str("pen"), "qty" to JSONValue.Num(qty))))
        assertEquals(emptyList<String>(), step()) // 在庫 10
        move("out1", -3.0)
        assertEquals(emptyList<String>(), step()) // 7
        move("out2", -4.0)
        assertEquals(listOf("low"), step()) // 3 < 5
        move("out3", -1.0)
        assertEquals(emptyList<String>(), step()) // 2: 下回ったまま
        move("in2", 10.0)
        assertEquals(emptyList<String>(), step()) // 12: 戻った
        move("out4", -8.0)
        assertEquals(listOf("low"), step()) // 4: また下回った
    }

    @Test
    fun plannerSkipsConditional() {
        val dsl = dsl()
        val planned = NotificationPlanner.plan("inv", dsl.app.notifications, emptyMap(), Instant.EPOCH)
        assertEquals(listOf("dsl.inv.daily"), planned.map { it.identifier })
        assertNull(NotificationPlanner.action("inv", dsl.app.notifications.first { it.id == "low" }, Instant.EPOCH))
        assertEquals("when.inv.low", ConditionNotifications.identifier("inv", "low"))
    }
}
