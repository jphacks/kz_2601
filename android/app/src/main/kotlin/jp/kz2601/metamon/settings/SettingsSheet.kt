package jp.kz2601.metamon.settings

import android.app.Activity
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.ModalBottomSheet
import androidx.compose.material3.Switch
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.material3.rememberModalBottomSheetState
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.unit.dp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import jp.kz2601.metamon.AppContainer
import jp.kz2601.metamon.R
import jp.kz2601.metamon.consent.LegalLinks
import jp.kz2601.metamon.language.AppLocales
import jp.kz2601.metamon.language.LanguageSection
import java.time.ZoneId
import java.time.format.DateTimeFormatter
import java.time.format.FormatStyle

/**
 * 最小の設定(iOS の SettingsSheet のうち #888 の分): プライバシー(AI へのデータ送信の同意)・クラッシュ報告・
 * AI ボタンの利用状況・言語(#843 の [LanguageSection])。ほかの項目は設定の画面(#898)で足す。
 */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun SettingsSheet(container: AppContainer, onDismiss: () -> Unit) {
    ModalBottomSheet(
        onDismissRequest = onDismiss,
        sheetState = rememberModalBottomSheetState(skipPartiallyExpanded = true),
        modifier = Modifier.testTag("settings.sheet"),
    ) {
        Column(Modifier.verticalScroll(rememberScrollState()).padding(bottom = 24.dp)) {
            Row(Modifier.fillMaxWidth().padding(horizontal = 16.dp), verticalAlignment = Alignment.CenterVertically) {
                Text(stringResource(R.string.settings_title), style = MaterialTheme.typography.titleLarge, modifier = Modifier.weight(1f))
                TextButton(onClick = onDismiss) { Text(stringResource(R.string.settings_close)) }
            }
            PrivacySection(container)
            HorizontalDivider(Modifier.padding(vertical = 8.dp))
            val context = LocalContext.current
            var language by remember { mutableStateOf(AppLocales.setting(context)) }
            LanguageSection(language, onSelect = { selected ->
                if (selected == language) return@LanguageSection
                language = selected
                // Android 12 / 12L は開いている画面を作り直す(13+ はシステムがやる)
                if (AppLocales.set(context, selected)) (context as? Activity)?.recreate()
            })
        }
    }
}

/** iOS の `PrivacySettingsSection`。同意の確認・取り消しと、クラッシュ報告・AI ボタンの利用状況のオン / オフ。 */
@Composable
private fun PrivacySection(container: AppContainer) {
    val consent by container.consent.state.collectAsStateWithLifecycle()
    val crashEnabled by container.crashReporting.enabled.collectAsStateWithLifecycle()
    val metricsEnabled by container.aiButtonMetrics.enabled.collectAsStateWithLifecycle()
    var confirmRevoke by rememberSaveable { mutableStateOf(false) }

    Header(stringResource(R.string.settings_privacy_header))
    Row(Modifier.fillMaxWidth().padding(horizontal = 16.dp, vertical = 8.dp)) {
        Text(stringResource(R.string.settings_ai_consent), modifier = Modifier.weight(1f))
        Text(
            stringResource(if (consent.isGranted) R.string.settings_ai_consent_granted else R.string.settings_ai_consent_not_granted),
            color = if (consent.isGranted) MaterialTheme.colorScheme.primary else MaterialTheme.colorScheme.error,
            modifier = Modifier.testTag("settings.aiConsent.status"),
        )
    }
    if (consent.isGranted) {
        consent.grantedAt?.let { at ->
            val text = remember(at) {
                DateTimeFormatter.ofLocalizedDateTime(FormatStyle.MEDIUM, FormatStyle.SHORT).withZone(ZoneId.systemDefault()).format(at)
            }
            Row(Modifier.fillMaxWidth().padding(horizontal = 16.dp)) {
                Text(stringResource(R.string.settings_ai_consent_date), style = MaterialTheme.typography.bodySmall, modifier = Modifier.weight(1f))
                Text(text, style = MaterialTheme.typography.bodySmall)
            }
        }
        TextButton(onClick = { confirmRevoke = true }, modifier = Modifier.padding(horizontal = 4.dp).testTag("settings.aiConsent.revoke")) {
            Text(stringResource(R.string.settings_ai_consent_revoke), color = MaterialTheme.colorScheme.error)
        }
    } else {
        TextButton(onClick = container.consentPrompt::show, modifier = Modifier.padding(horizontal = 4.dp).testTag("settings.aiConsent.grant")) {
            Text(stringResource(R.string.settings_ai_consent_grant))
        }
    }
    LegalLinks(Modifier.padding(horizontal = 4.dp))
    Footer(stringResource(R.string.settings_privacy_footer))

    ToggleRow(stringResource(R.string.settings_crash_reporting), crashEnabled, "settings.crashReporting.toggle", container.crashReporting::setEnabled)
    Footer(stringResource(R.string.settings_crash_reporting_footer))
    ToggleRow(stringResource(R.string.settings_ai_button_metrics), metricsEnabled, "settings.aiButtonMetrics.toggle", container.aiButtonMetrics::setEnabled)
    Footer(stringResource(R.string.settings_ai_button_metrics_footer))

    if (confirmRevoke) {
        AlertDialog(
            onDismissRequest = { confirmRevoke = false },
            title = { Text(stringResource(R.string.settings_revoke_title)) },
            text = { Text(stringResource(R.string.settings_revoke_message)) },
            confirmButton = {
                TextButton(onClick = {
                    confirmRevoke = false
                    container.consent.revoke()
                }) { Text(stringResource(R.string.settings_revoke_confirm), color = MaterialTheme.colorScheme.error) }
            },
            dismissButton = { TextButton(onClick = { confirmRevoke = false }) { Text(stringResource(R.string.settings_revoke_cancel)) } },
        )
    }
}

@Composable
private fun Header(text: String) {
    Text(text, style = MaterialTheme.typography.titleSmall, modifier = Modifier.padding(horizontal = 16.dp, vertical = 8.dp))
}

@Composable
private fun Footer(text: String) {
    Text(
        text,
        style = MaterialTheme.typography.bodySmall,
        color = MaterialTheme.colorScheme.onSurfaceVariant,
        modifier = Modifier.padding(horizontal = 16.dp, vertical = 8.dp),
    )
}

@Composable
private fun ToggleRow(label: String, checked: Boolean, tag: String, onChange: (Boolean) -> Unit) {
    Row(Modifier.fillMaxWidth().padding(horizontal = 16.dp, vertical = 4.dp), verticalAlignment = Alignment.CenterVertically) {
        Text(label, modifier = Modifier.weight(1f))
        Switch(checked = checked, onCheckedChange = onChange, modifier = Modifier.testTag(tag))
    }
}
