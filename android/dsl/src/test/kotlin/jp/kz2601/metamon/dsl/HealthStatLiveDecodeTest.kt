package jp.kz2601.metamon.dsl

import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

/** `healthStat` の `live`(#3694)は書いた時だけ true。省略は null(開いた時に取り直すだけ)。 */
class HealthStatLiveDecodeTest {
    private fun component(json: String): Component = DslJson.decodeFromString(Component.serializer(), json)

    @Test
    fun liveDecodesOnlyWhenWritten() {
        val live = component("""{"type":"healthStat","metric":"steps","label":"歩数","live":true}""")
        assertEquals(ComponentType.HEALTH_STAT, live.type)
        assertEquals(Metric.STEPS, live.metric)
        assertEquals(true, live.live)
        assertNull(component("""{"type":"healthStat","metric":"steps","label":"歩数"}""").live)
        assertEquals(false, component("""{"type":"healthStat","metric":"steps","label":"歩数","live":false}""").live)
    }

    @Test
    fun liveIsAKnownKeyOfHealthStat() {
        assertTrue(DslKeys.component.getValue(ComponentType.HEALTH_STAT).contains("live"))
    }
}
