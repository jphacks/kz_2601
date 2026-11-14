package jp.kz2601.metamon.desktop.notify

import java.awt.GraphicsEnvironment
import java.awt.SystemTray
import java.awt.TrayIcon
import java.awt.image.BufferedImage

/**
 * 1 件の通知。[onClick] は押された時(出す口が受けられる時だけ呼ぶ)。
 * [actions] は通知のボタン(#3210・出す口が [Notifier.supportsActions] の時だけ出す)。押されたら [NoticeActions] に [id] と一緒に届く。
 */
data class DesktopNotice(
    val title: String,
    val body: String,
    val onClick: (() -> Unit)? = null,
    val id: String? = null,
    val actions: List<NoticeAction> = emptyList(),
    val onAction: ((NoticeAction) -> Unit)? = null,
)

/**
 * 通知を出す口(#2185 の 1)。トレイの吹き出し・Linux の D-Bus(#2185)・Windows のトースト(#2186)がこれを実装する。
 * 予約・取り消しは `jp.kz2601.metamon.desktop.Notifier`(スケジュール側)が持ち、鳴らす時にこの口を呼ぶ。
 */
interface Notifier {
    /** 今この口で出せるか(出せない時は予約の時点で「使えない」と返す)。 */
    val available: Boolean

    /** 出せたら true。 */
    fun show(notice: DesktopNotice): Boolean

    /** ボタン付きの通知を出し、押された印を [NoticeActions] に届けられるか(トーストの actions・freedesktop の ActionInvoked)。 */
    val supportsActions: Boolean get() = false
}

/**
 * トレイのアイコンの吹き出し(今までの出し方)。常駐トレイ(Tray.kt の DesktopTray)が出ている間はそのアイコンから、
 * 無ければ通知用のアイコンを足して出す。トレイの吹き出しは押された時を受けないので [DesktopNotice.onClick] は使わない。
 */
object TrayNotifier : Notifier {
    /** 常駐トレイが出ている間だけ入る。 */
    @Volatile
    var sink: ((title: String, body: String) -> Unit)? = null

    private var icon: TrayIcon? = null

    /** アイコンを作る・出すのは予約の鍵と別(AWT を待つ間に予約・取り消しを止めない)。 */
    private val iconLock = Any()

    override val available: Boolean
        get() = !GraphicsEnvironment.isHeadless() && SystemTray.isSupported()

    override fun show(notice: DesktopNotice): Boolean {
        if (GraphicsEnvironment.isHeadless()) return false
        sink?.let {
            it(notice.title, notice.body)
            return true
        }
        return synchronized(iconLock) { showOnIcon(notice.title, notice.body) }
    }

    private fun showOnIcon(title: String, body: String): Boolean = runCatching {
        val tray = SystemTray.getSystemTray()
        val i = icon ?: TrayIcon(BufferedImage(16, 16, BufferedImage.TYPE_INT_ARGB), "Tailor It").also {
            it.isImageAutoSize = true
            tray.add(it)
            icon = it
        }
        i.displayMessage(title, body, TrayIcon.MessageType.INFO)
    }.isSuccess
}
