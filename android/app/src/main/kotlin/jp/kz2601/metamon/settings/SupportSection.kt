package jp.kz2601.metamon.settings

import android.content.ClipData
import android.content.ClipboardManager
import android.content.Context
import android.content.Intent
import android.net.Uri
import androidx.activity.compose.BackHandler
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.text.selection.SelectionContainer
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.produceState
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.heading
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.compose.ui.window.Dialog
import androidx.compose.ui.window.DialogProperties
import jp.kz2601.metamon.R
import jp.kz2601.metamon.renderer.a11y.rememberAnnouncer
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.withContext

/** お問い合わせの窓口(iOS の `LegalDocument.contactURL`)。 */
const val CONTACT_URL = "https://tailor-it.app/contact"

/**
 * 設定の一番下(iOS の `SettingsSheet.supportSection`・#279 / #306): お問い合わせ・ライセンス・詳細(サポート用)。
 * 詳細は 1 画面目に英数字を出さないよう既定で畳む。ユーザー ID などの識別子は出さない([SupportInfo])。
 */
@Composable
fun SupportSection(info: SupportInfo) {
    val context = LocalContext.current
    var expanded by rememberSaveable { mutableStateOf(false) }
    var copied by rememberSaveable { mutableStateOf(false) }
    var licensesOpen by rememberSaveable { mutableStateOf(false) }
    val labels = SupportInfo.Field.entries.associateWith { supportLabel(it) }
    val announce = rememberAnnouncer()
    val copiedText = stringResource(R.string.settings_support_copied)

    TextButton(
        onClick = { runCatching { context.startActivity(Intent(Intent.ACTION_VIEW, Uri.parse(CONTACT_URL))) } },
        modifier = Modifier.padding(horizontal = 4.dp).testTag("settings.contact"),
    ) { Text(stringResource(R.string.settings_contact)) }
    TextButton(onClick = { licensesOpen = true }, modifier = Modifier.padding(horizontal = 4.dp).testTag("settings.licenses")) {
        Text(stringResource(R.string.settings_licenses))
    }
    TextButton(onClick = { expanded = !expanded }, modifier = Modifier.padding(horizontal = 4.dp).testTag("settings.supportDetails")) {
        Text(stringResource(R.string.settings_support_header) + if (expanded) " ▲" else " ▼")
    }
    if (expanded) {
        info.rows.forEach { row ->
            Row(Modifier.fillMaxWidth().padding(horizontal = 16.dp, vertical = 2.dp), horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                Text(labels.getValue(row.field), style = MaterialTheme.typography.bodySmall, modifier = Modifier.weight(1f))
                SelectionContainer {
                    Text(
                        row.value,
                        style = MaterialTheme.typography.bodySmall,
                        fontFamily = FontFamily.Monospace,
                        maxLines = 1,
                        overflow = TextOverflow.MiddleEllipsis,
                    )
                }
            }
        }
        TextButton(
            onClick = {
                copyToClipboard(context, info.clipboardText { labels.getValue(it) })
                copied = true
                announce(copiedText) // TalkBack にも「コピーしました」を読ませる(H8)
            },
            modifier = Modifier.padding(horizontal = 4.dp).testTag("settings.supportDetails.copy"),
        ) { Text(stringResource(if (copied) R.string.settings_support_copied else R.string.settings_support_copy)) }
        SettingsFooter(stringResource(R.string.settings_support_footer))
    }
    if (licensesOpen) LicensesDialog(onDismiss = { licensesOpen = false })
}

@Composable
private fun supportLabel(field: SupportInfo.Field): String = when (field) {
    SupportInfo.Field.VOICE_MODEL -> stringResource(R.string.settings_support_voice_model)
    SupportInfo.Field.AI -> "ai"
    SupportInfo.Field.APPCONFIG -> "appconfig"
    SupportInfo.Field.VERSION -> stringResource(R.string.settings_support_version)
    SupportInfo.Field.APP_REVISION -> stringResource(R.string.settings_support_app_revision)
}

private fun copyToClipboard(context: Context, text: String) {
    val clipboard = context.getSystemService(ClipboardManager::class.java) ?: return
    clipboard.setPrimaryClip(ClipData.newPlainText("support", text))
}

/** ライセンスの一覧と本文(iOS の `LicensesView` / `LicenseDetailView`)。設定のシートの上に全画面で重ねる。 */
@Composable
private fun LicensesDialog(onDismiss: () -> Unit) {
    val context = LocalContext.current
    val entries by produceState<List<LicenseEntry>?>(initialValue = null) {
        value = withContext(Dispatchers.IO) {
            runCatching {
                context.resources.openRawResource(R.raw.third_party_licenses).bufferedReader().use { it.readText() }
            }.getOrNull()?.let(ThirdPartyLicenses::parse) ?: emptyList()
        }
    }
    var selectedId by rememberSaveable { mutableStateOf<String?>(null) }
    val selected = entries?.firstOrNull { it.id == selectedId }
    Dialog(onDismissRequest = onDismiss, properties = DialogProperties(usePlatformDefaultWidth = false)) {
        Surface(Modifier.fillMaxSize().testTag("licenses.sheet")) {
            Column {
                Row(Modifier.fillMaxWidth().padding(horizontal = 8.dp), verticalAlignment = Alignment.CenterVertically) {
                    Text(
                        selected?.name ?: stringResource(R.string.settings_licenses),
                        style = MaterialTheme.typography.titleMedium,
                        maxLines = 1,
                        overflow = TextOverflow.Ellipsis,
                        modifier = Modifier.weight(1f).padding(horizontal = 8.dp).semantics { heading() },
                    )
                    TextButton(onClick = { if (selected != null) selectedId = null else onDismiss() }) {
                        Text(stringResource(R.string.settings_close))
                    }
                }
                HorizontalDivider()
                when {
                    entries == null -> Unit // 読み込み中(約 2MB の JSON を裏で読む)
                    entries!!.isEmpty() -> Text(
                        stringResource(R.string.settings_licenses_failed),
                        color = MaterialTheme.colorScheme.onSurfaceVariant,
                        modifier = Modifier.padding(16.dp),
                    )
                    selected != null -> {
                        BackHandler { selectedId = null }
                        LicenseDetail(selected)
                    }
                    else -> LicenseList(entries!!) { selectedId = it.id }
                }
            }
        }
    }
}

@Composable
private fun LicenseList(entries: List<LicenseEntry>, onOpen: (LicenseEntry) -> Unit) {
    val app = entries.filter { it.kind == LicenseEntry.Kind.APP }
    val deps = entries.filter { it.kind == LicenseEntry.Kind.DEPENDENCY }
    LazyColumn(Modifier.fillMaxSize()) {
        item { SettingsHeader(stringResource(R.string.settings_licenses_app)) }
        items(app, key = { it.id }) { LicenseRow(it, onOpen) }
        item { SettingsHeader(stringResource(R.string.settings_licenses_oss)) }
        items(deps, key = { it.id }) { LicenseRow(it, onOpen) }
        item { SettingsFooter(stringResource(R.string.settings_licenses_footer)) }
    }
}

@Composable
private fun LicenseRow(entry: LicenseEntry, onOpen: (LicenseEntry) -> Unit) {
    Column(
        Modifier.fillMaxWidth().clickable { onOpen(entry) }.padding(horizontal = 16.dp, vertical = 8.dp)
            .semantics(mergeDescendants = true) {}.testTag("licenses.${entry.id}"),
    ) {
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
    val context = LocalContext.current
    LazyColumn(Modifier.fillMaxSize().padding(horizontal = 16.dp)) {
        item {
            Column(Modifier.padding(vertical = 12.dp), verticalArrangement = Arrangement.spacedBy(4.dp)) {
                entry.version?.let { DetailRow(stringResource(R.string.settings_licenses_version), it) }
                DetailRow(stringResource(R.string.settings_licenses), entry.license)
                entry.copyright.forEach { Text(it, style = MaterialTheme.typography.bodySmall) }
                entry.url?.let { url ->
                    TextButton(onClick = { runCatching { context.startActivity(Intent(Intent.ACTION_VIEW, Uri.parse(url))) } }) {
                        Text(stringResource(R.string.settings_licenses_source))
                    }
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
        item { Text("", modifier = Modifier.padding(bottom = 24.dp)) }
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
