package jp.kz2601.metamon.services.permissions

import jp.kz2601.metamon.dsl.AppDSL
import jp.kz2601.metamon.dsl.decode
import jp.kz2601.metamon.services.permissions.AppPermission.BACKGROUND_LOCATION
import jp.kz2601.metamon.services.permissions.AppPermission.CAMERA
import jp.kz2601.metamon.services.permissions.AppPermission.LOCATION
import jp.kz2601.metamon.services.permissions.AppPermission.MICROPHONE
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
        assertEquals(setOf(CAMERA, AppPermission.NEARBY, MICROPHONE, LOCATION), PermissionNeeds.of(d))
    }

    @Test
    fun countdownのタイマーは終了の通知が要る_stopwatchは要らない() {
        assertEquals(setOf(NOTIFICATIONS), PermissionNeeds.of(dsl("""[{"type":"timerDisplay","id":"t","mode":"countdown","seconds":60}]""")))
        assertEquals(emptySet<AppPermission>(), PermissionNeeds.of(dsl("""[{"type":"timerDisplay","id":"t","mode":"stopwatch"}]""")))
    }

    @Test
    fun 写真をライブラリからだけ選ぶならカメラは要らない() {
        assertEquals(emptySet<AppPermission>(), PermissionNeeds.of(dsl("""[{"type":"photoField","field":"p","source":"library"}]""")))
    }

    @Test
    fun ドラレコは音を録る時だけマイクも要る() {
        assertEquals(setOf(CAMERA), PermissionNeeds.of(dsl("""[{"type":"dashcam"}]""")))
        assertEquals(setOf(CAMERA), PermissionNeeds.of(dsl("""[{"type":"dashcam","audio":false}]""")))
        assertEquals(setOf(CAMERA, MICROPHONE), PermissionNeeds.of(dsl("""[{"type":"dashcam","audio":true}]""")))
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
        assertEquals(setOf(NOTIFICATIONS, LOCATION, BACKGROUND_LOCATION), PermissionNeeds.of(near))

        val action = dsl(
            """[{"type":"button","label":"b","action":{"type":"scheduleNotification","notification":{"id":"n","title":"t","body":"b"}}}]""",
        )
        assertEquals(setOf(NOTIFICATIONS), PermissionNeeds.of(action))

        val toolbar = dsl("[]", toolbar = """[{"label":"b","action":{"type":"scheduleNotification","notification":{"id":"n","title":"t","body":"b"}}}]""")
        assertEquals(setOf(NOTIFICATIONS), PermissionNeeds.of(toolbar))

        // 進行中の表示(#1030)は常駐の通知
        val activity = dsl("""[{"type":"button","label":"b","action":{"type":"startActivity","activity":{"title":"t"}}}]""")
        assertEquals(setOf(NOTIFICATIONS), PermissionNeeds.of(activity))
    }

    @Test
    fun 裏での位置のバーは位置が許可された後だけ() {
        val near = setOf(LOCATION, BACKGROUND_LOCATION)
        // 位置が断られていれば、先に位置のバー
        assertEquals(LOCATION, PermissionOffBarModel.pick(near, mapOf(LOCATION to DENIED, BACKGROUND_LOCATION to DENIED), emptySet(), busy = false, summaryShowing = false))
        // 位置が未確認なら、裏での位置は聞けないので出さない
        assertNull(PermissionOffBarModel.pick(near, mapOf(LOCATION to NOT_ASKED, BACKGROUND_LOCATION to DENIED), emptySet(), busy = false, summaryShowing = false))
        // 位置が許可され、裏での位置が断られている
        assertEquals(BACKGROUND_LOCATION, PermissionOffBarModel.pick(near, mapOf(LOCATION to GRANTED, BACKGROUND_LOCATION to PERMANENTLY_DENIED), emptySet(), busy = false, summaryShowing = false))
        assertNull(PermissionOffBarModel.pick(near, mapOf(LOCATION to GRANTED, BACKGROUND_LOCATION to GRANTED), emptySet(), busy = false, summaryShowing = false))
    }

    @Test
    fun 場所の通知の許可は位置から裏での位置の順() {
        assertEquals(NearPermissionFlow.Step.ASK_FOREGROUND, NearPermissionFlow.next(NOT_ASKED, NOT_ASKED))
        assertEquals(NearPermissionFlow.Step.EXPLAIN_BACKGROUND, NearPermissionFlow.next(GRANTED, NOT_ASKED))
        // 断られた後は聞き直さない(バーが案内する)
        assertEquals(NearPermissionFlow.Step.NONE, NearPermissionFlow.next(DENIED, NOT_ASKED))
        assertEquals(NearPermissionFlow.Step.NONE, NearPermissionFlow.next(GRANTED, DENIED))
        assertEquals(NearPermissionFlow.Step.NONE, NearPermissionFlow.next(GRANTED, GRANTED))
        assertEquals(listOf("android.permission.ACCESS_BACKGROUND_LOCATION"), BACKGROUND_LOCATION.manifestPermissions(31))
    }

    @Test
    fun バーは使う権限で断られている時だけ() {
        val statuses = mapOf(CAMERA to DENIED, MICROPHONE to GRANTED, LOCATION to NOT_ASKED)
        assertEquals(CAMERA, PermissionOffBarModel.pick(setOf(CAMERA, MICROPHONE), statuses, emptySet(), busy = false, summaryShowing = false))
        // 使っていない権限が断られていても出さない
        assertNull(PermissionOffBarModel.pick(setOf(MICROPHONE, LOCATION), statuses, emptySet(), busy = false, summaryShowing = false))
        // 宣言していない(UNAVAILABLE)・状態が無いものは出さない
        assertNull(PermissionOffBarModel.pick(setOf(AppPermission.HEALTH), mapOf(AppPermission.HEALTH to UNAVAILABLE), emptySet(), busy = false, summaryShowing = false))
        assertNull(PermissionOffBarModel.pick(setOf(AppPermission.HEALTH), emptyMap(), emptySet(), busy = false, summaryShowing = false))
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
