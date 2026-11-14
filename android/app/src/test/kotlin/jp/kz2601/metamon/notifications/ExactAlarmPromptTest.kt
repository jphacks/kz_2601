package jp.kz2601.metamon.notifications

import jp.kz2601.metamon.dsl.AppDSL
import jp.kz2601.metamon.dsl.decode
import jp.kz2601.metamon.services.consent.InMemoryKeyValueStore
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

class ExactAlarmPromptTest {
    private val kv = InMemoryKeyValueStore()
    private val prompt = ExactAlarmPrompt(kv)

    private fun dsl(components: String, notifications: String = "[]") = AppDSL.decode(
        """
        {"schemaVersion":1,"app":{"name":"t","notifications":$notifications},"initialScreen":"home",
         "screens":[{"id":"home","title":"t","components":$components}]}
        """.trimIndent(),
    )

    @Test
    fun 許可が無ければ1回だけ出す() {
        assertTrue(prompt.shouldShow(needed = true, sdkInt = 34, canScheduleExact = false))
        prompt.markShown()
        assertFalse(prompt.shouldShow(needed = true, sdkInt = 34, canScheduleExact = false))
        // 起動し直しても出さない(断られたら聞き直さない)
        assertFalse(ExactAlarmPrompt(kv).shouldShow(needed = true, sdkInt = 34, canScheduleExact = false))
    }

    @Test
    fun 要らない時_許可済み_古いOSでは出さない() {
        assertFalse(prompt.shouldShow(needed = false, sdkInt = 34, canScheduleExact = false))
        assertFalse(prompt.shouldShow(needed = true, sdkInt = 34, canScheduleExact = true))
        assertFalse(prompt.shouldShow(needed = true, sdkInt = 30, canScheduleExact = false))
    }

    @Test
    fun 忘れるとまた出す_サインアウト() {
        prompt.markShown()
        prompt.forget()
        assertTrue(prompt.shouldShow(needed = true, sdkInt = 34, canScheduleExact = false))
    }

    @Test
    fun countdownのタイマーとアラームの通知が要る() {
        assertTrue(ExactAlarmPrompt.needed(dsl("""[{"type":"card","children":[{"type":"timerDisplay","id":"t","mode":"countdown","seconds":60}]}]""")))
        assertFalse(ExactAlarmPrompt.needed(dsl("""[{"type":"timerDisplay","id":"t","mode":"stopwatch"}]""")))
        val alarm = """[{"id":"n","title":"起きる","body":"","repeat":"daily","time":"07:00","alarm":true}]"""
        assertTrue(ExactAlarmPrompt.needed(dsl("[]", alarm)))
        val plain = """[{"id":"n","title":"t","body":"","repeat":"daily","time":"07:00"}]"""
        assertFalse(ExactAlarmPrompt.needed(dsl("[]", plain)))
    }
}
