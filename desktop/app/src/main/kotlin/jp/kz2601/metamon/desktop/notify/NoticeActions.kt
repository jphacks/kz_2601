package jp.kz2601.metamon.desktop.notify

import jp.kz2601.metamon.desktop.i18n.Strings
import java.net.URLDecoder
import java.net.URLEncoder

// 通知のボタン(開く・あとで)を受ける土台(#3210)。
// Windows はトーストの actions(押すと `tailorit-notice:` の引数で exe が起動 → SingleInstance の handOff で起動中の方へ)、
// Linux は org.freedesktop.Notifications の ActionInvoked(D-Bus の通知の番号 → こちらの id に戻す)。
// どちらも最後は [NoticeActions.deliver] に届き、アプリを前に出してどのボタンかを渡す。記録するボタンは DSL の決まりの後。

/** 通知のボタン。[wire] は OS に渡す鍵(トーストの arguments・freedesktop の action key)。 */
enum class NoticeAction(val wire: String) {
    OPEN("open"),
    LATER("later"),
    ;

    companion object {
        fun of(wire: String?): NoticeAction? = entries.firstOrNull { it.wire == wire }
    }
}

/** ボタンの見出し(i18n)。 */
val NoticeAction.label: String
    get() = Strings[
        when (this) {
            NoticeAction.OPEN -> "desktop_notice_action_open"
            NoticeAction.LATER -> "desktop_notice_action_later"
        },
    ]

/** 押された印: どの通知([noticeId])のどのボタンか。 */
data class NoticePress(val noticeId: String, val action: NoticeAction)

/**
 * 押された印の受け口。出した通知の [DesktopNotice.onAction] と、アプリの受け手([onPress]・Main がアプリを前に出す)に渡す。
 * 受け手を置く前に届いた分(ボタンで起動した直後)は置いた時に渡す。
 */
object NoticeActions {
    private const val MAX_REMEMBERED = 32

    private var handler: ((NoticePress) -> Unit)? = null
    private val pending = mutableListOf<NoticePress>()

    /** 出した通知の押された時の口(古い物から忘れる)。 */
    private val shown = object : LinkedHashMap<String, (NoticeAction) -> Unit>() {
        override fun removeEldestEntry(eldest: MutableMap.MutableEntry<String, (NoticeAction) -> Unit>?): Boolean = size > MAX_REMEMBERED
    }

    fun onPress(h: (NoticePress) -> Unit) {
        val queued = synchronized(this) {
            handler = h
            pending.toList().also { pending.clear() }
        }
        queued.forEach(h)
    }

    /** 出す口が通知を出した時に呼ぶ(押された時に [DesktopNotice.onAction] を呼べるよう覚える)。 */
    fun remember(notice: DesktopNotice) {
        val id = notice.id ?: return
        val cb = notice.onAction ?: return
        synchronized(this) { shown[id] = cb }
    }

    fun deliver(press: NoticePress) {
        val (cb, h) = synchronized(this) {
            val h = handler
            if (h == null) pending += press
            shown[press.noticeId] to h
        }
        cb?.invoke(press.action)
        h?.invoke(press)
    }

    /** テスト・「この端末のデータを消す」。 */
    @Synchronized
    fun reset() {
        handler = null
        pending.clear()
        shown.clear()
    }
}

/** Windows: ボタンで起動した時の引数(`tailorit-notice:<action>?id=<id>`)。プロトコルの起動でも同じ 1 つの引数になる。 */
object NoticeArgs {
    const val SCHEME = "tailorit-notice:"

    fun encode(p: NoticePress): String = "$SCHEME${p.action.wire}?id=${URLEncoder.encode(p.noticeId, Charsets.UTF_8)}"

    fun parse(argv: List<String>): NoticePress? = argv.firstNotNullOfOrNull(::parseOne)

    private fun parseOne(arg: String): NoticePress? {
        if (!arg.startsWith(SCHEME)) return null
        val rest = arg.removePrefix(SCHEME).removePrefix("//")
        val action = NoticeAction.of(rest.substringBefore('?')) ?: return null
        val id = rest.substringAfter('?', "").split('&')
            .firstOrNull { it.startsWith("id=") }?.removePrefix("id=")
            ?.let { runCatching { URLDecoder.decode(it, Charsets.UTF_8) }.getOrNull() }
            ?.takeIf { it.isNotEmpty() && it.length <= 256 } ?: return null
        return NoticePress(id, action)
    }

    /** 印を除いた引数(アプリの id などの並びは崩さない)。 */
    fun strip(argv: List<String>): List<String> = argv.filterNot { it.startsWith(SCHEME) }
}

/** Windows のトーストの XML(ボタンは actions の action)。 */
object WindowsToastXml {
    fun build(notice: DesktopNotice, label: (NoticeAction) -> String): String {
        val sb = StringBuilder()
        sb.append("<toast")
        notice.id?.let { sb.append(" launch=\"").append(esc(NoticeArgs.encode(NoticePress(it, NoticeAction.OPEN)))).append("\" activationType=\"protocol\"") }
        sb.append("><visual><binding template=\"ToastGeneric\">")
        sb.append("<text>").append(esc(notice.title)).append("</text>")
        sb.append("<text>").append(esc(notice.body)).append("</text>")
        sb.append("</binding></visual>")
        val id = notice.id
        if (id != null && notice.actions.isNotEmpty()) {
            sb.append("<actions>")
            notice.actions.forEach { a ->
                sb.append("<action content=\"").append(esc(label(a)))
                    .append("\" arguments=\"").append(esc(NoticeArgs.encode(NoticePress(id, a))))
                    .append("\" activationType=\"protocol\"/>")
            }
            sb.append("</actions>")
        }
        return sb.append("</toast>").toString()
    }

    private fun esc(s: String): String = buildString {
        for (c in s) {
            when (c) {
                '&' -> append("&amp;")
                '<' -> append("&lt;")
                '>' -> append("&gt;")
                '"' -> append("&quot;")
                '\'' -> append("&apos;")
                else -> append(c)
            }
        }
    }
}

/** Linux: org.freedesktop.Notifications の Notify の actions と、ActionInvoked を印に戻す所。 */
class FreedesktopActions {
    /** D-Bus の通知の番号 → こちらの通知の id(古い物から忘れる)。 */
    private val ids = object : LinkedHashMap<Long, String>() {
        override fun removeEldestEntry(eldest: MutableMap.MutableEntry<Long, String>?): Boolean = size > 32
    }

    /** Notify の `actions`(鍵と見出しを交互に)。押されなかった本体のクリックは "default"。 */
    fun actions(notice: DesktopNotice, label: (NoticeAction) -> String): List<String> =
        notice.actions.flatMap { listOf(it.wire, label(it)) }

    /** Notify が返した番号を覚える。 */
    @Synchronized
    fun shown(dbusId: Long, notice: DesktopNotice) {
        notice.id?.let { ids[dbusId] = it }
    }

    /** ActionInvoked(id, action_key)。こちらの通知でなければ null。本体のクリック("default")は開く。 */
    @Synchronized
    fun invoked(dbusId: Long, actionKey: String): NoticePress? {
        val id = ids[dbusId] ?: return null
        val action = if (actionKey == "default") NoticeAction.OPEN else NoticeAction.of(actionKey) ?: return null
        return NoticePress(id, action)
    }

    /** NotificationClosed で忘れる。 */
    @Synchronized
    fun closed(dbusId: Long) {
        ids.remove(dbusId)
    }
}
