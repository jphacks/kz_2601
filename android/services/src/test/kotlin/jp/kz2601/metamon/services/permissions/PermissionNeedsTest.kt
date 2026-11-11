package jp.kz2601.metamon.services.permissions

import jp.kz2601.metamon.dsl.AppDSL
import jp.kz2601.metamon.dsl.decode
import jp.kz2601.metamon.services.permissions.AppPermission.CAMERA
import jp.kz2601.metamon.services.permissions.AppPermission.LOCATION
import jp.kz2601.metamon.services.permissions.AppPermission.MICROPHONE
import jp.kz2601.metamon.services.permissions.AppPermission.NEARBY
import jp.kz2601.metamon.services.permissions.AppPermission.NOTIFICATIONS
import jp.kz2601.metamon.services.permissions.PermissionStatus.DENIED
import jp.kz2601.metamon.services.permissions.PermissionStatus.GRANTED
import jp.kz2601.metamon.services.permissions.PermissionStatus.NOT_ASKED
import jp.kz2601.metamon.services.permissions.PermissionStatus.PERMANENTLY_DENIED
import jp.kz2601.metamon.services.permissions.PermissionStatus.UNAVAILABLE
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Test

class PermissionNeedsTest {
    private fun dsl(components: String, notifications: String = "[]", toolbar: String = "[]") = AppDSL.decode(
        """
        {"schemaVersion":1,"app":{"name":"t","notifications":$notifications},"initialScreen":"home",
         "screens":[{"id":"home","title":"t","toolbar":$toolbar,"components":$components}]}
        """.trimIndent(),
    )

    @Test
    fun 部品から要る権限を集める_入れ子も見る() {
        val d = dsl(
            """[{"type":"vstack","children":[{"type":"photoField","field":"p"},{"type":"card","children":[{"type":"nearbyShare"}]}]},
               {"type":"audioRecorder","field":"a"},{"type":"locationButton","field":"l"}]""",
        )
        assertEquals(setOf(CAMERA, NEARBY, MICROPHONE, LOCATION), PermissionNeeds.of(d))
    }

    @Test
    fun 写真をライブラリからだけ選ぶならカメラは要らない() {
        assertEquals(emptySet<AppPermission>(), PermissionNeeds.of(dsl("""[{"type":"photoField","field":"p","source":"library"}]""")))
    }

    @Test
    fun 音声の入口と表示だけの部品は権限を求めない() {
        assertEquals(emptySet<AppPermission>(), PermissionNeeds.of(dsl("""[{"type":"voiceInput"},{"type":"text","text":"x"}]""")))
    }

    @Test
    fun 通知の定義か予約のアクションがあれば通知_場所の通知は位置も() {
        val defined = dsl("[]", notifications = """[{"id":"n","title":"t","body":"b","repeat":"daily","time":"09:00"}]""")
        assertEquals(setOf(NOTIFICATIONS), PermissionNeeds.of(defined))

        val near = dsl("[]", notifications = """[{"id":"n","title":"t","body":"b","near":{"entity":"e","locationField":"loc"}}]""")
        assertEquals(setOf(NOTIFICATIONS, LOCATION), PermissionNeeds.of(near))

        val action = dsl(
            """[{"type":"button","label":"b","action":{"type":"scheduleNotification","notification":{"id":"n","title":"t","body":"b"}}}]""",
        )
        assertEquals(setOf(NOTIFICATIONS), PermissionNeeds.of(action))

        val toolbar = dsl("[]", toolbar = """[{"label":"b","action":{"type":"scheduleNotification","notification":{"id":"n","title":"t","body":"b"}}}]""")
        assertEquals(setOf(NOTIFICATIONS), PermissionNeeds.of(toolbar))
    }

    @Test
    fun バーは使う権限で断られている時だけ() {
        val statuses = mapOf(CAMERA to DENIED, MICROPHONE to GRANTED, LOCATION to NOT_ASKED)
        assertEquals(CAMERA, PermissionOffBarModel.pick(setOf(CAMERA, MICROPHONE), statuses, emptySet(), busy = false, summaryShowing = false))
        // 使っていない権限が断られていても出さない
        assertNull(PermissionOffBarModel.pick(setOf(MICROPHONE, LOCATION), statuses, emptySet(), busy = false, summaryShowing = false))
        // 宣言していない(UNAVAILABLE)・状態が無いものは出さない
        assertNull(PermissionOffBarModel.pick(setOf(NEARBY), mapOf(NEARBY to UNAVAILABLE), emptySet(), busy = false, summaryShowing = false))
        assertNull(PermissionOffBarModel.pick(setOf(NEARBY), emptyMap(), emptySet(), busy = false, summaryShowing = false))
    }

    @Test
    fun バーは閉じたら出さず_生成中と要約の表示中は重ねない() {
        val needed = setOf(CAMERA)
        val statuses = mapOf(CAMERA to PERMANENTLY_DENIED)
        assertEquals(CAMERA, PermissionOffBarModel.pick(needed, statuses, emptySet(), busy = false, summaryShowing = false))
        assertNull(PermissionOffBarModel.pick(needed, statuses, setOf(CAMERA), busy = false, summaryShowing = false))
        assertNull(PermissionOffBarModel.pick(needed, statuses, emptySet(), busy = true, summaryShowing = false))
        assertNull(PermissionOffBarModel.pick(needed, statuses, emptySet(), busy = false, summaryShowing = true))
    }

    @Test
    fun バーは1本だけ_通知を先に() {
        val needed = setOf(CAMERA, NOTIFICATIONS)
        val statuses = mapOf(CAMERA to DENIED, NOTIFICATIONS to PERMANENTLY_DENIED)
        assertEquals(NOTIFICATIONS, PermissionOffBarModel.pick(needed, statuses, emptySet(), busy = false, summaryShowing = false))
        assertEquals(CAMERA, PermissionOffBarModel.pick(needed, statuses, setOf(NOTIFICATIONS), busy = false, summaryShowing = false))
    }
}
