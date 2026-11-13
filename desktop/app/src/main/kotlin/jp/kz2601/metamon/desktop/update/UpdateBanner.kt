package jp.kz2601.metamon.desktop.update

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.widthIn
import androidx.compose.foundation.text.selection.SelectionContainer
import androidx.compose.material3.Button
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.semantics.LiveRegionMode
import androidx.compose.ui.semantics.heading
import androidx.compose.ui.semantics.liveRegion
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.unit.dp
import jp.kz2601.metamon.desktop.DesktopLicenses
import jp.kz2601.metamon.desktop.i18n.Strings
import jp.kz2601.metamon.desktop.net.ClientPlatform
import jp.kz2601.metamon.desktop.net.DesktopApi
import jp.kz2601.metamon.desktop.net.DesktopApiConfig
import jp.kz2601.metamon.services.http.HttpRequest
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.delay
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive
import java.net.URI
import java.util.prefs.Preferences

/** 判定(#2157 の 1)の結果から、画面に出す物(#2377)。 */
sealed interface UpdateView {
    /** 何も出さない */
    data object Hidden : UpdateView

    /** 閉じられる「新しい版があります」の帯 */
    data class Banner(val latest: Int, val downloadUrl: String?) : UpdateView

    /** アプリの中身を出さない「更新してください」の画面 */
    data class Blocking(val minimum: Int, val downloadUrl: String?) : UpdateView

    companion object {
        /** [dismissedLatest] は帯を閉じた時の最新の版(同じ版では出さない・新しい版が来たらまた出す)。 */
        fun of(decision: UpdateDecision, dismissedLatest: Int?, downloadUrl: String?): UpdateView = when (decision) {
            UpdateDecision.None -> Hidden
            is UpdateDecision.Recommended -> if (dismissedLatest == decision.latest) Hidden else Banner(decision.latest, downloadUrl)
            is UpdateDecision.Required -> Blocking(decision.minimum, downloadUrl)
        }
    }
}

/** ダウンロードの場所(client-config の `windowsDownloadUrl` / `linuxDownloadUrl`)。https だけ。無ければ null(文言だけ出す)。 */
object UpdateDownload {
    private val json = Json { ignoreUnknownKeys = true }

    fun url(body: String?, os: ClientPlatform): String? {
        val obj = body?.let { runCatching { json.parseToJsonElement(it) }.getOrNull() } as? JsonObject ?: return null
        val key = when (os) {
            ClientPlatform.WINDOWS -> "windowsDownloadUrl"
            ClientPlatform.LINUX -> "linuxDownloadUrl"
            ClientPlatform.MACOS -> return null
        }
        val raw = (obj[key] as? JsonPrimitive)?.takeIf { it.isString }?.content?.trim() ?: return null
        val uri = runCatching { URI(raw) }.getOrNull() ?: return null
        return raw.takeIf { uri.scheme.equals("https", ignoreCase = true) && !uri.host.isNullOrEmpty() }
    }
}

/** 帯を閉じた版の置き場(端末の設定)。 */
interface UpdateDismissStore {
    fun get(): Int?
    fun set(latest: Int)
}

class MemoryUpdateDismissStore(private var value: Int? = null) : UpdateDismissStore {
    override fun get() = value
    override fun set(latest: Int) {
        value = latest
    }
}

/** java.util.prefs(Windows はレジストリ・Linux は ~/.java)。 */
class PrefsUpdateDismissStore : UpdateDismissStore {
    private val prefs by lazy { Preferences.userRoot().node("jp/kz2601/tailorit/desktop") }
    override fun get(): Int? = prefs.getInt(KEY, 0).takeIf { it > 0 }
    override fun set(latest: Int) = prefs.putInt(KEY, latest)

    private companion object {
        const val KEY = "updateDismissedLatest"
    }
}

/**
 * 版の確認(#2377)。起動時と [INTERVAL_MILLIS] ごとに [fetch] で client-config を読み、[view] を替える。
 * [fetch] が null(`MOCK`・通信しない)なら読まず、何も出さない。読めなかった時は前の結果のまま(fail open)。
 */
class UpdateState(
    private val fetch: (suspend () -> String?)?,
    private val os: ClientPlatform = ClientPlatform.current(),
    private val build: String = DesktopApi.BUILD,
    private val dismissals: UpdateDismissStore = MemoryUpdateDismissStore(),
) {
    var decision: UpdateDecision by mutableStateOf(UpdateDecision.None)
        private set
    var downloadUrl: String? by mutableStateOf(null)
        private set
    private var dismissed: Int? by mutableStateOf(dismissals.get())

    val view: UpdateView get() = UpdateView.of(decision, dismissed, downloadUrl)

    /** 1 回読む。読んだら true。 */
    suspend fun refresh(): Boolean {
        val f = fetch ?: return false
        val body = try {
            f()
        } catch (e: CancellationException) {
            throw e
        } catch (_: Exception) {
            null
        } ?: return false
        decision = UpdatePolicy.decide(body, os, build)
        downloadUrl = UpdateDownload.url(body, os)
        return true
    }

    /** 起動時から [INTERVAL_MILLIS] ごとに読み続ける。`MOCK` ではすぐ返る。 */
    suspend fun run(sleep: suspend (Long) -> Unit = { delay(it) }) {
        if (fetch == null) return
        while (true) {
            refresh()
            sleep(INTERVAL_MILLIS)
        }
    }

    /** 帯を閉じた: 同じ版では出さない(端末に残す)。 */
    fun dismiss() {
        val latest = (decision as? UpdateDecision.Recommended)?.latest ?: return
        dismissals.set(latest)
        dismissed = latest
    }

    companion object {
        const val INTERVAL_MILLIS = 6 * 60 * 60 * 1000L

        /** 接続先の設定から。`MOCK` は読まない。client-config はトークン無しで読む(サインイン前にも読む)。 */
        fun create(config: DesktopApiConfig, dismissals: UpdateDismissStore = PrefsUpdateDismissStore()): UpdateState {
            val fetch: (suspend () -> String?)? = if (!config.isLive) {
                null
            } else {
                val http = DesktopApi.http(config.appConfigUrl, DesktopApi.devTokens(null))
                suspend { withContext(Dispatchers.IO) { http.send(HttpRequest("GET", listOf("v1", "client-config"), timeoutMillis = 8_000)) } }
            }
            return UpdateState(fetch, dismissals = dismissals)
        }
    }
}

/** ウィンドウの中身を包む: 更新が要れば中身を出さず、新しい版があれば上に帯を出す。 */
@Composable
fun UpdateGate(state: UpdateState, content: @Composable () -> Unit) {
    when (val v = state.view) {
        is UpdateView.Blocking -> UpdateRequiredScreen(v)
        is UpdateView.Banner -> Column(Modifier.fillMaxSize()) {
            UpdateBanner(v, onDismiss = state::dismiss)
            Box(Modifier.weight(1f)) { content() }
        }
        UpdateView.Hidden -> content()
    }
}

/** 「新しい版があります」の帯。閉じられる。 */
@Composable
fun UpdateBanner(view: UpdateView.Banner, onDismiss: () -> Unit) {
    Surface(color = MaterialTheme.colorScheme.secondaryContainer, modifier = Modifier.fillMaxWidth()) {
        Column(Modifier.padding(horizontal = 12.dp, vertical = 6.dp)) {
            Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                Text(
                    Strings["update_recommended_title"],
                    style = MaterialTheme.typography.bodyMedium,
                    modifier = Modifier.weight(1f).semantics { liveRegion = LiveRegionMode.Polite },
                )
                view.downloadUrl?.let { url -> DownloadButton(url, filled = false) }
                TextButton(onClick = onDismiss) { Text(Strings["desktop_close"]) }
            }
        }
    }
}

/** 「更新してください」。アプリの中身は出さず、ダウンロードの場所を開くだけ。 */
@Composable
fun UpdateRequiredScreen(view: UpdateView.Blocking) {
    Surface(Modifier.fillMaxSize()) { Box(Modifier.fillMaxSize().padding(24.dp), contentAlignment = Alignment.Center) {
        Column(
            Modifier.widthIn(max = 420.dp),
            horizontalAlignment = Alignment.CenterHorizontally,
            verticalArrangement = Arrangement.spacedBy(16.dp),
        ) {
            Text(Strings["update_required_title"], style = MaterialTheme.typography.headlineSmall, modifier = Modifier.semantics { heading() })
            Text(Strings["desktop_update_required_body"], textAlign = TextAlign.Center)
            val url = view.downloadUrl
            if (url != null) {
                DownloadButton(url, filled = true)
            } else {
                Text(Strings["desktop_update_no_download_url"], style = MaterialTheme.typography.bodySmall, textAlign = TextAlign.Center)
            }
        }
    } }
}

/** 既定のブラウザで開く。開けなければ URL を添えて知らせる(選んで写せる)。 */
@Composable
private fun DownloadButton(url: String, filled: Boolean) {
    val scope = rememberCoroutineScope()
    var failed by remember { mutableStateOf(false) }
    val open: () -> Unit = {
        scope.launch {
            // browse は OS の起動を待つことがあるので UI のスレッドで呼ばない
            failed = !withContext(Dispatchers.IO) { DesktopLicenses.browse(URI(url)) }
        }
    }
    Column(horizontalAlignment = Alignment.CenterHorizontally) {
        if (filled) {
            Button(onClick = open) { Text(Strings["desktop_update_open_download"]) }
        } else {
            TextButton(onClick = open) { Text(Strings["desktop_update_open_download"]) }
        }
        if (failed) {
            SelectionContainer {
                Text(Strings.format("desktop_licenses_browse_failed", url), style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.error)
            }
        }
    }
}
