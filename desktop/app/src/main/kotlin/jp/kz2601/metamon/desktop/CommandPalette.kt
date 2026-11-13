package jp.kz2601.metamon.desktop

import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.widthIn
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.itemsIndexed
import androidx.compose.foundation.lazy.rememberLazyListState
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableIntStateOf
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.focus.FocusRequester
import androidx.compose.ui.focus.focusRequester
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.input.key.Key
import androidx.compose.ui.input.key.KeyEventType
import androidx.compose.ui.input.key.key
import androidx.compose.ui.input.key.onPreviewKeyEvent
import androidx.compose.ui.input.key.type
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import java.io.File
import java.net.JarURLConnection
import java.text.Normalizer

/** パレットの 1 行(画面・アクション・アプリ)。[hint] は右に出すショートカット。 */
class PaletteCommand(
    val id: String,
    val title: String,
    val kind: Kind,
    val keywords: List<String> = emptyList(),
    val hint: String? = null,
    val run: () -> Unit,
) {
    enum class Kind {
        ACTION, SCREEN, APP;

        val label: String
            get() = when (this) {
                ACTION -> KeyboardText.KIND_ACTION
                SCREEN -> KeyboardText.KIND_SCREEN
                APP -> KeyboardText.KIND_APP
            }
    }

    /** アプリの行ならそのアプリの id(行にサムネイルを出す・#3204)。 */
    val appId: String? get() = if (kind == Kind.APP) id.removePrefix(APP_PREFIX).takeIf { it != id } else null

    override fun toString(): String = "$kind:$title"

    companion object {
        const val APP_PREFIX = "app:"
    }
}

/**
 * パレットの絞り込みと並べ方。全角半角・大小・カタカナ / ひらがなは同じに見る。
 * 点: 完全一致 > 頭から一致 > 語の頭から一致 > 途中に含む(前ほど上)> 別名(id など)> 文字が順に現れる。同点は元の順。
 */
object CommandRanking {
    fun normalize(s: String): String = Normalizer.normalize(s, Normalizer.Form.NFKC).lowercase()
        .map { c -> if (c in 'ァ'..'ヶ') c - 0x60 else c }
        .filterNot { it.isWhitespace() }
        .joinToString("")

    /** 合わなければ null。 */
    fun score(c: PaletteCommand, query: String): Int? {
        val q = normalize(query)
        if (q.isEmpty()) return 0
        val t = normalize(c.title)
        when {
            t == q -> return 1000
            t.startsWith(q) -> return 800
            words(c.title).any { normalize(it).startsWith(q) } -> return 600
        }
        val i = t.indexOf(q)
        if (i >= 0) return 400 - i.coerceAtMost(99)
        val keys = c.keywords.map(::normalize)
        if (keys.any { it.startsWith(q) }) return 300
        if (keys.any { it.contains(q) }) return 200
        return subsequenceGap(t, q)?.let { 100 - it.coerceAtMost(99) }
    }

    fun rank(commands: List<PaletteCommand>, query: String): List<PaletteCommand> =
        commands.withIndex()
            .mapNotNull { (i, c) -> score(c, query)?.let { Triple(c, it, i) } }
            .sortedWith(compareByDescending<Triple<PaletteCommand, Int, Int>> { it.second }.thenBy { it.third })
            .map { it.first }

    private fun words(s: String): List<String> = s.split(Regex("[\\s/・_\\-()（）]+")).filter { it.isNotEmpty() }

    /** q の文字が t に順に現れるなら、間に挟まった文字の数。 */
    private fun subsequenceGap(t: String, q: String): Int? {
        var at = 0
        var gap = 0
        var started = false
        for (ch in q) {
            val j = t.indexOf(ch, at)
            if (j < 0) return null
            if (started) gap += j - at
            started = true
            at = j + 1
        }
        return gap
    }
}

/** 同梱の例(docs/dsl/examples → jar の examples/)。(id, アプリ名) を名前順に。 */
object BundledApps {
    private val cached: List<Pair<String, String>> by lazy {
        ids().mapNotNull { id -> runCatching { id to Examples.load(id).app.name }.getOrNull() }.sortedBy { it.second }
    }

    fun all(): List<Pair<String, String>> = cached

    fun ids(): List<String> {
        val url = BundledApps::class.java.classLoader.getResource("examples/shopping.json") ?: return emptyList()
        val names = when (url.protocol) {
            "file" -> File(url.toURI()).parentFile.list().orEmpty().toList()
            // jarFile はクラスローダーと共有なので閉じない
            "jar" -> (url.openConnection() as JarURLConnection).jarFile.entries().asSequence()
                .map { it.name }.filter { it.startsWith("examples/") }.map { it.removePrefix("examples/") }.toList()
            else -> emptyList()
        }
        return names.filter { it.endsWith(".json") && '/' !in it }.map { it.removeSuffix(".json") }.sorted()
    }
}

/** Ctrl+K のパレット。↑↓ で選び Enter で実行、Esc で閉じる。 */
@Composable
fun CommandPalette(commands: List<PaletteCommand>, os: HostOs, onDismiss: () -> Unit) {
    var query by remember { mutableStateOf("") }
    var selected by remember { mutableIntStateOf(0) }
    val shown = remember(commands, query) { CommandRanking.rank(commands, query) }
    val input = remember { FocusRequester() }
    val list = rememberLazyListState()
    LaunchedEffect(Unit) { runCatching { input.requestFocus() } }
    LaunchedEffect(shown) { selected = 0 }
    LaunchedEffect(selected) { if (shown.isNotEmpty()) list.scrollToItem(selected.coerceIn(0, shown.lastIndex)) }
    val runAt = { i: Int -> shown.getOrNull(i)?.let { onDismiss(); it.run() } }
    val thumbnails = LocalAppThumbnails.current

    Box(
        Modifier.fillMaxSize().background(Color(0x66000000)).clickable(onClick = onDismiss),
        contentAlignment = Alignment.TopCenter,
    ) {
        Surface(
            Modifier.padding(top = 48.dp).widthIn(max = 560.dp).fillMaxWidth(0.92f).clickable(enabled = false) {},
            shape = MaterialTheme.shapes.large,
            tonalElevation = 8.dp,
        ) {
            Column(
                Modifier.padding(12.dp).onPreviewKeyEvent { e ->
                    if (e.type != KeyEventType.KeyDown) return@onPreviewKeyEvent false
                    when (e.key) {
                        Key.DirectionDown -> true.also { if (shown.isNotEmpty()) selected = (selected + 1) % shown.size }
                        Key.DirectionUp -> true.also { if (shown.isNotEmpty()) selected = (selected - 1 + shown.size) % shown.size }
                        Key.Enter, Key.NumPadEnter -> true.also { runAt(selected) }
                        Key.Escape -> true.also { onDismiss() }
                        else -> false
                    }
                },
                verticalArrangement = Arrangement.spacedBy(8.dp),
            ) {
                OutlinedTextField(
                    value = query,
                    onValueChange = { query = it },
                    placeholder = { Text(KeyboardText.PALETTE_PLACEHOLDER) },
                    singleLine = true,
                    modifier = Modifier.fillMaxWidth().focusRequester(input),
                )
                if (shown.isEmpty()) {
                    Text(KeyboardText.PALETTE_EMPTY, color = Color.Gray, modifier = Modifier.padding(8.dp))
                }
                LazyColumn(Modifier.fillMaxWidth().heightIn(max = 360.dp), state = list) {
                    itemsIndexed(shown, key = { _, c -> c.id }) { i, c ->
                        val on = i == selected
                        Row(
                            Modifier.fillMaxWidth()
                                .background(if (on) MaterialTheme.colorScheme.secondaryContainer else Color.Transparent, MaterialTheme.shapes.small)
                                .clickable { runAt(i) }
                                .padding(horizontal = 10.dp, vertical = 8.dp),
                            verticalAlignment = Alignment.CenterVertically,
                            horizontalArrangement = Arrangement.spacedBy(8.dp),
                        ) {
                            Text(c.kind.label, style = MaterialTheme.typography.labelSmall, color = Color.Gray, modifier = Modifier.widthIn(min = 64.dp))
                            // 最後に見た画面の写し(無ければ今のまま)
                            c.appId?.let { rememberAppThumbnail(it, thumbnails) }?.let { AppRowThumbnail(it) }
                            Text(c.title, maxLines = 1, overflow = TextOverflow.Ellipsis, modifier = Modifier.weight(1f))
                            c.hint?.let { Text(it, style = MaterialTheme.typography.labelMedium, color = Color.Gray) }
                        }
                    }
                }
            }
        }
    }
}

/** F1 のショートカット一覧。[onLicenses] があれば「ライセンス」も出す(デスクトップに「このアプリについて」が無いので、ここを入り口にする)。 */
@Composable
fun ShortcutHelp(os: HostOs, onLicenses: (() -> Unit)? = null, onDismiss: () -> Unit) {
    AlertDialog(
        onDismissRequest = onDismiss,
        title = { Text(KeyboardText.HELP) },
        text = {
            Column(verticalArrangement = Arrangement.spacedBy(6.dp)) {
                Shortcut.entries.forEach { s ->
                    Row(Modifier.fillMaxWidth()) {
                        Text(s.title, modifier = Modifier.weight(1f))
                        Text(Keymap.chords(s, os).joinToString(" / ") { Keymap.label(it, os) }, color = Color.Gray)
                    }
                }
                Text(KeyboardText.HELP_TAB, style = MaterialTheme.typography.bodySmall, color = Color.Gray)
            }
        },
        confirmButton = { TextButton(onClick = onDismiss) { Text(KeyboardText.HELP_CLOSE) } },
        dismissButton = onLicenses?.let { open -> { TextButton(onClick = open) { Text(LicensesText.TITLE) } } },
    )
}
