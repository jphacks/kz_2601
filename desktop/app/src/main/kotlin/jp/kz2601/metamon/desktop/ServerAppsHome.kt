package jp.kz2601.metamon.desktop

import androidx.compose.foundation.border
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.widthIn
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.material3.Icon
import androidx.compose.material3.LocalContentColor
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.semantics.heading
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import jp.kz2601.metamon.desktop.i18n.Strings
import jp.kz2601.metamon.desktop.net.ServerApps
import jp.kz2601.metamon.desktop.net.ServerAppsResult
import jp.kz2601.metamon.dsl.AppDSL
import jp.kz2601.metamon.services.AppConfigClient
import jp.kz2601.metamon.services.AppSummary
import jp.kz2601.metamon.services.local.DocumentCache
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.launch

/** 一覧の 1 行(#2379)。[icon] は写しの DSL の `app.icon`(無ければ null・既定の印)。 */
data class ServerAppRow(val id: String, val name: String, val icon: String?)

/**
 * サーバーのアプリの一覧(#2378 の [ServerApps])を画面に出すための状態。全部のウィンドウで 1 つ。
 * [source] が null(MOCK・サインインしていない)なら何も出さない。
 */
class ServerAppsModel(
    private val source: ServerApps?,
    /** 写しの DSL からアイコンの名前を引く(無ければ null)。 */
    private val iconOf: suspend (String) -> String? = { null },
) {
    val available: Boolean get() = source != null

    var loading: Boolean by mutableStateOf(false)
        private set
    var result: ServerAppsResult? by mutableStateOf(null)
        private set
    var rows: List<ServerAppRow> by mutableStateOf(emptyList())
        private set

    /** 前回の写しを出している。 */
    val fromCache: Boolean get() = (result as? ServerAppsResult.Loaded)?.fromCache == true

    suspend fun refresh() {
        val s = source ?: return
        loading = true
        try {
            val r = s.load()
            rows = ServerAppPalette.rows((r as? ServerAppsResult.Loaded)?.apps.orEmpty()) { runCatching { iconOf(it) }.getOrNull() }
            result = r
        } finally {
            loading = false
        }
    }
}

/** 一覧 → 行・パレットの項目(描画に頼らない。テストはこれを見る)。 */
object ServerAppPalette {
    suspend fun rows(apps: List<AppSummary>, iconOf: suspend (String) -> String?): List<ServerAppRow> =
        apps.map { ServerAppRow(it.id, it.name.trim().ifEmpty { Strings["desktop_untitled"] }, iconOf(it.id)) }

    /** 並びの順のまま。写しなら右に「前回の一覧」。 */
    fun commands(rows: List<ServerAppRow>, fromCache: Boolean, open: (ServerAppRow) -> Unit): List<PaletteCommand> {
        val hint = if (fromCache) Strings["desktop_server_apps_cached"] else null
        return rows.map { row ->
            PaletteCommand("server:${row.id}", row.name, PaletteCommand.Kind.APP, keywords = listOf(row.id, KeyboardText.NEW_WINDOW), hint = hint) { open(row) }
        }
    }
}

/**
 * 選んだアプリの DSL(`GET /v1/apps/{appId}/dsl`)。読めたら写しは `HttpAppConfigClient` が [cache] に置く。
 * 圏外・5xx は写しの DSL、それも無ければ null。
 */
class ServerAppOpener(private val client: AppConfigClient, private val cache: DocumentCache) {
    suspend fun load(appId: String): AppDSL? = try {
        client.fetchDocument(appId).document.dsl
    } catch (e: CancellationException) {
        throw e
    } catch (e: Exception) {
        if (ServerApps.usesCopy(e)) cache.load(appId)?.document?.dsl else null
    }
}

/** ホーム(最後に開いたアプリが無い時の最初のウィンドウ)。読み込み中はスケルトン、写しなら「前回の一覧」。 */
@Composable
fun ServerAppsHome(model: ServerAppsModel, onOpen: suspend (ServerAppRow) -> Boolean) {
    if (model.loading && model.result == null) {
        LoadingSkeleton()
        return
    }
    val scope = rememberCoroutineScope()
    var failed by remember { mutableStateOf(false) }
    Surface(Modifier.fillMaxSize()) {
        Box(Modifier.fillMaxSize(), contentAlignment = Alignment.TopCenter) {
            Column(Modifier.widthIn(max = 720.dp).fillMaxWidth().padding(16.dp), verticalArrangement = Arrangement.spacedBy(8.dp)) {
                Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                    Text(Strings["settings_usage_apps"], style = MaterialTheme.typography.titleLarge, modifier = Modifier.weight(1f).semantics { heading() })
                    if (model.fromCache) Text(Strings["desktop_server_apps_cached"], style = MaterialTheme.typography.labelSmall, color = Color.Gray)
                }
                if (failed) Text(Strings["desktop_server_app_open_failed"], color = MaterialTheme.colorScheme.error, style = MaterialTheme.typography.bodySmall)
                if (model.rows.isEmpty()) {
                    Text(Strings["switcher_empty"], color = Color.Gray, modifier = Modifier.padding(vertical = 16.dp))
                }
                LazyColumn(Modifier.fillMaxWidth()) {
                    items(model.rows, key = { it.id }) { row ->
                        Row(
                            Modifier.fillMaxWidth().clickable { scope.launch { failed = !onOpen(row) } }.padding(horizontal = 8.dp, vertical = 12.dp),
                            verticalAlignment = Alignment.CenterVertically,
                            horizontalArrangement = Arrangement.spacedBy(12.dp),
                        ) {
                            ServerAppIcon(row.icon)
                            Text(row.name, maxLines = 1, overflow = TextOverflow.Ellipsis, modifier = Modifier.weight(1f))
                        }
                    }
                }
            }
        }
    }
}

/** アプリのアイコン(飾り・読み上げは名前の文字で足りる)。表に無い名前は細い円。 */
@Composable
private fun ServerAppIcon(name: String?) {
    val tint = LocalContentColor.current
    val vector = DesktopIcons.vector(name)
    if (vector != null) {
        Icon(vector, contentDescription = null, modifier = Modifier.size(28.dp), tint = tint)
    } else {
        Box(Modifier.size(28.dp).padding(4.dp).border(2.dp, tint, CircleShape))
    }
}
