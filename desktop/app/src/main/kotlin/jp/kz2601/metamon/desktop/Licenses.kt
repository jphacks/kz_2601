package jp.kz2601.metamon.desktop

import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.widthIn
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.text.selection.SelectionContainer
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.produceState
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.semantics.heading
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.unit.dp
import jp.kz2601.metamon.desktop.i18n.Strings
import jp.kz2601.metamon.settings.LicenseEntry
import jp.kz2601.metamon.settings.ThirdPartyLicenses
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.withContext
import java.awt.Desktop
import java.io.File
import java.net.URI

/**
 * サードパーティのライセンス(Android の設定の「ライセンス」・iOS の `ThirdPartyLicenses` と同じ形)。
 * 一覧はビルド時に third-party-licenses.gradle.kts が runtimeClasspath の POM と jar の NOTICE から作る。
 * 同梱の Java 実行環境の分は、起動している実行環境の `legal/` から読む(jpackage が同梱する物と同じ)。
 */
object DesktopLicenses {
    const val RESOURCE = "licenses/third_party_licenses.json"

    /** ビルド時に作った一覧。読めなければ null。 */
    fun bundled(): List<LicenseEntry>? =
        DesktopLicenses::class.java.classLoader.getResourceAsStream(RESOURCE)
            ?.use { it.readBytes().toString(Charsets.UTF_8) }
            ?.let(ThirdPartyLicenses::parse)

    /** 実行環境(`java.home/legal/<module>/`)。java.base の LICENSE を本文に、各 module の表示(*.md・例外)を notices に。 */
    fun javaRuntime(home: File = File(System.getProperty("java.home").orEmpty())): LicenseEntry? {
        val legal = home.resolve("legal")
        val license = legal.resolve("java.base/LICENSE").takeIf { it.isFile } ?: return null
        val seen = mutableSetOf<String>()
        val notices = legal.listFiles().orEmpty().filter { it.isDirectory }.sortedBy { it.name }.flatMap { dir ->
            dir.listFiles().orEmpty().filter { it.isFile && it.name != "LICENSE" }.sortedBy { it.name }
                .filter { seen.add(it.name) } // 各 module に同じ物(ASSEMBLY_EXCEPTION など)が入っている
                .map { LicenseEntry.Notice(it.name.removeSuffix(".md"), it.readText()) }
        }
        val text = license.readText()
        val vendor = System.getProperty("java.vendor").orEmpty()
        return LicenseEntry(
            id = "java-runtime",
            kind = LicenseEntry.Kind.DEPENDENCY,
            name = listOf("Java Runtime", vendor).filter { it.isNotBlank() }.joinToString(" · "),
            version = System.getProperty("java.runtime.version") ?: System.getProperty("java.version"),
            url = null,
            // 種類は書き写さず、実行環境の LICENSE の 1 行目(OpenJDK なら「The GNU General Public License (GPL)」)
            license = text.lineSequence().map { it.trim() }.firstOrNull { it.isNotEmpty() } ?: "LICENSE",
            copyright = emptyList(),
            text = text,
            notices = notices,
        )
    }

    /** 画面に出す一覧: このアプリ → 同梱物と依存 → 実行環境。一覧が読めなければ null。 */
    fun all(): List<LicenseEntry>? = bundled()?.let { it + listOfNotNull(javaRuntime()) }
}

/** 一覧と本文。F1 の一覧とコマンドパレットから開く。 */
@Composable
fun LicensesDialog(onDismiss: () -> Unit) {
    val entries by produceState<List<LicenseEntry>?>(initialValue = null) {
        value = withContext(Dispatchers.IO) { runCatching { DesktopLicenses.all() }.getOrNull() ?: emptyList() }
    }
    var selected by remember { mutableStateOf<LicenseEntry?>(null) }
    val open = selected
    AlertDialog(
        onDismissRequest = { if (open != null) selected = null else onDismiss() },
        modifier = Modifier.widthIn(max = 640.dp),
        title = { Text(open?.name ?: LicensesText.TITLE, modifier = Modifier.semantics { heading() }) },
        text = {
            val list = entries
            Column(Modifier.heightIn(max = 480.dp)) {
                when {
                    list == null -> Unit // 読み込み中(数百 KB の JSON を裏で読む)
                    list.isEmpty() -> Text(LicensesText.FAILED, color = MaterialTheme.colorScheme.onSurfaceVariant)
                    open != null -> LicenseDetail(open)
                    else -> LicenseList(list) { selected = it }
                }
            }
        },
        confirmButton = {
            TextButton(onClick = { if (open != null) selected = null else onDismiss() }) {
                Text(if (open != null) LicensesText.BACK else LicensesText.CLOSE)
            }
        },
    )
}

/** ライセンスの画面の文言(Android の settings_licenses_* と同じキー・無い物は desktop_licenses*)。 */
object LicensesText {
    val TITLE get() = Strings["desktop_licenses"]
    val APP get() = Strings["settings_licenses_app"]
    val OSS get() = Strings["settings_licenses_oss"]
    val FOOTER get() = Strings["settings_licenses_footer"]
    val FAILED get() = Strings["settings_licenses_failed"]
    val LICENSE get() = Strings["settings_licenses"]
    val VERSION get() = Strings["settings_licenses_version"]
    val SOURCE get() = Strings["settings_licenses_source"]
    val BACK get() = Strings["renderer_back"]
    val CLOSE get() = Strings["desktop_close"]
}

@Composable
private fun LicenseList(entries: List<LicenseEntry>, onOpen: (LicenseEntry) -> Unit) {
    val app = entries.filter { it.kind == LicenseEntry.Kind.APP }
    val deps = entries.filter { it.kind == LicenseEntry.Kind.DEPENDENCY }
    LazyColumn(Modifier.fillMaxWidth()) {
        item { Header(LicensesText.APP) }
        items(app, key = { it.id }) { LicenseRow(it, onOpen) }
        item { Header(LicensesText.OSS) }
        items(deps, key = { it.id }) { LicenseRow(it, onOpen) }
        item {
            Text(
                LicensesText.FOOTER,
                style = MaterialTheme.typography.bodySmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
                modifier = Modifier.padding(top = 12.dp),
            )
        }
    }
}

@Composable
private fun Header(text: String) {
    Text(
        text,
        style = MaterialTheme.typography.titleSmall,
        color = MaterialTheme.colorScheme.primary,
        modifier = Modifier.padding(top = 12.dp, bottom = 4.dp).semantics { heading() },
    )
}

@Composable
private fun LicenseRow(entry: LicenseEntry, onOpen: (LicenseEntry) -> Unit) {
    Column(Modifier.fillMaxWidth().clickable { onOpen(entry) }.padding(vertical = 6.dp)) {
        Text(entry.name)
        Text(
            listOfNotNull(entry.version, entry.license).joinToString(" · "),
            style = MaterialTheme.typography.bodySmall,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
        )
    }
}

@Composable
private fun LicenseDetail(entry: LicenseEntry) {
    LazyColumn(Modifier.fillMaxWidth()) {
        item {
            Column(Modifier.padding(bottom = 8.dp), verticalArrangement = Arrangement.spacedBy(4.dp)) {
                entry.version?.let { DetailRow(LicensesText.VERSION, it) }
                DetailRow(LicensesText.LICENSE, entry.license)
                entry.copyright.forEach { Text(it, style = MaterialTheme.typography.bodySmall) }
                entry.url?.takeIf { it.startsWith("https://") || it.startsWith("http://") }?.let { url ->
                    TextButton(onClick = { runCatching { Desktop.getDesktop().browse(URI(url)) } }) { Text(LicensesText.SOURCE) }
                }
            }
        }
        items(ThirdPartyLicenses.paragraphs(entry.text)) { Paragraph(it) }
        entry.notices.forEach { notice ->
            item {
                Text(
                    notice.title,
                    style = MaterialTheme.typography.titleSmall,
                    modifier = Modifier.padding(top = 16.dp, bottom = 4.dp).semantics { heading() },
                )
            }
            items(ThirdPartyLicenses.paragraphs(notice.text)) { Paragraph(it) }
        }
    }
}

@Composable
private fun DetailRow(label: String, value: String) {
    Row(Modifier.fillMaxWidth()) {
        Text(label, modifier = Modifier.weight(1f))
        Text(value, color = MaterialTheme.colorScheme.onSurfaceVariant)
    }
}

@Composable
private fun Paragraph(text: String) {
    SelectionContainer {
        Text(text, style = MaterialTheme.typography.bodySmall, modifier = Modifier.fillMaxWidth().padding(vertical = 4.dp))
    }
}
