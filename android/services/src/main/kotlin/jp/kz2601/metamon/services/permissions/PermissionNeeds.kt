package jp.kz2601.metamon.services.permissions

import jp.kz2601.metamon.dsl.Action
import jp.kz2601.metamon.dsl.ActionType
import jp.kz2601.metamon.dsl.AppDSL
import jp.kz2601.metamon.dsl.Component
import jp.kz2601.metamon.dsl.ComponentType
import jp.kz2601.metamon.dsl.Source

/** 生成したアプリがどの権限を使うか(「オフ」のバーの判定に使う)。 */
object PermissionNeeds {
    fun of(dsl: AppDSL): Set<AppPermission> {
        val out = mutableSetOf<AppPermission>()
        if (dsl.app.notifications.isNotEmpty()) out += AppPermission.NOTIFICATIONS
        if (dsl.app.notifications.any { it.near != null }) out += AppPermission.LOCATION
        for (screen in dsl.screens) {
            screen.toolbar.forEach { addAction(it.action, out) }
            screen.components.forEach { addComponent(it, out) }
        }
        return out
    }

    /** 部品 1 つが要る権限。音声の入口(voiceInput)は音声のシートが自前で案内するので含めない。qrScanner は Google Code Scanner が読むので要らない(#866)。dashcam のマイクは audio: true の時だけ(iOS の DashcamRecorder と同じ)。nearbyShare は Android にまだ無いので求めない(近距離共有 #878 で戻す)。 */
    fun of(component: Component): Set<AppPermission> = when (component.type) {
        ComponentType.AUDIO_RECORDER, ComponentType.TRANSCRIBE_FIELD -> setOf(AppPermission.MICROPHONE)
        ComponentType.PHOTO_FIELD, ComponentType.VIDEO_FIELD ->
            if (component.source == Source.LIBRARY) emptySet() else setOf(AppPermission.CAMERA)
        ComponentType.DASHCAM ->
            if (component.audio == true) setOf(AppPermission.CAMERA, AppPermission.MICROPHONE) else setOf(AppPermission.CAMERA)
        ComponentType.LOCATION_BUTTON, ComponentType.ROUTE_GUIDE, ComponentType.LOCATION_TRACKER -> setOf(AppPermission.LOCATION)
        ComponentType.HEALTH_STAT -> setOf(AppPermission.HEALTH)
        else -> emptySet()
    }

    private fun addComponent(c: Component, out: MutableSet<AppPermission>) {
        out += of(c)
        addAction(c.action, out)
        addAction(c.onItemTap, out)
        addAction(c.onDateTap, out)
        c.children.forEach { addComponent(it, out) }
    }

    private fun addAction(a: Action?, out: MutableSet<AppPermission>) {
        if (a?.type != ActionType.SCHEDULE_NOTIFICATION) return
        out += AppPermission.NOTIFICATIONS
        if (a.notification?.near != null) out += AppPermission.LOCATION
    }
}

/** 「オフ」のバーの出し分け(iOS の NotificationsOffBarModel)。View から切り離してテストする。 */
object PermissionOffBarModel {
    /** 出す順(1 本だけ出す)。iOS の唯一のバーだった通知を先に。 */
    val ORDER = listOf(
        AppPermission.NOTIFICATIONS,
        AppPermission.MICROPHONE,
        AppPermission.CAMERA,
        AppPermission.LOCATION,
        AppPermission.NEARBY,
        AppPermission.HEALTH,
    )

    /**
     * 出すバーの権限。使う権限で断られていて、閉じていない時。生成中・要約の表示中は重ねない(iOS と同じ)。
     * [dismissed] はこのアプリで閉じた権限(別のアプリに切り替えたらまた判定する)。
     */
    fun pick(
        needed: Set<AppPermission>,
        statuses: Map<AppPermission, PermissionStatus>,
        dismissed: Set<AppPermission>,
        busy: Boolean,
        summaryShowing: Boolean,
    ): AppPermission? {
        if (busy || summaryShowing) return null
        return ORDER.firstOrNull { it in needed && it !in dismissed && PermissionRules.isOff(statuses[it] ?: PermissionStatus.UNAVAILABLE) }
    }
}
