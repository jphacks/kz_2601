package jp.kz2601.metamon.desktop.net

import jp.kz2601.metamon.services.update.ClientConfig
import jp.kz2601.metamon.services.update.NoticeDismissals
import jp.kz2601.metamon.services.update.ServiceNotice

/**
 * client-config の notice(#333)をデスクトップで出すか(画面はまだ無い)。android/services の ServiceNotice をそのまま使う。
 * iOS の build の範囲(`minIOSBuild` / `maxIOSBuild`)は比べない。範囲の付いたお知らせは iOS の特定の版向けなので出さない(Android と同じ)。
 */
object DesktopNotice {
    fun toShow(config: ClientConfig?, dismissals: NoticeDismissals): ServiceNotice? {
        val notice = config?.notice ?: return null
        if (!notice.isDisplayable || !notice.appliesToAndroid) return null
        return notice.takeUnless { dismissals.isDismissed(it.id) }
    }
}
