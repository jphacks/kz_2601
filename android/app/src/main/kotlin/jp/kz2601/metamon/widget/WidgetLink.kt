package jp.kz2601.metamon.widget

import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import java.net.URLDecoder
import java.net.URLEncoder

/**
 * ウィジェットを押した時の開き先 `tailorit://widget?app=<appId>`(iOS の `metamon://widget`)。
 * 自分の画面を名指しで開く intent([ACTION])だけで使い、intent-filter は足さない(外のアプリから開かせない)。
 */
object WidgetLink {
    const val ACTION = "jp.kz2601.metamon.action.OPEN_WIDGET"
    private const val PREFIX = "tailorit://widget"

    fun uri(appId: String?): String =
        if (appId.isNullOrEmpty()) PREFIX else "$PREFIX?app=${URLEncoder.encode(appId, Charsets.UTF_8)}"

    /** 開くアプリ。ウィジェットのリンクでない・アプリの指定が無ければ null。 */
    fun appId(link: String?): String? {
        if (link == null || !(link == PREFIX || link.startsWith("$PREFIX?"))) return null
        val query = link.substringAfter('?', "")
        val raw = query.split('&').firstOrNull { it.startsWith("app=") }?.removePrefix("app=") ?: return null
        return URLDecoder.decode(raw, Charsets.UTF_8).takeIf { it.isNotEmpty() }
    }

    private val _pending = MutableStateFlow<String?>(null)

    /** 押されて、まだ開いていないアプリ(画面が受け取って切り替える)。 */
    val pending: StateFlow<String?> = _pending.asStateFlow()

    /** ウィジェットからの intent なら開くアプリを預かる。 */
    fun offer(action: String?, link: String?) {
        if (action != ACTION) return
        appId(link)?.let { _pending.value = it }
    }

    fun consume(): String? = _pending.value.also { _pending.value = null }
}
