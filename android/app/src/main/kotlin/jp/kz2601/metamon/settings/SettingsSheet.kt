package jp.kz2601.metamon.settings

import android.app.Activity
import android.content.Context
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
import androidx.compose.runtime.LaunchedEffect
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
import jp.kz2601.metamon.account.AccountSection
import jp.kz2601.metamon.consent.LegalLinks
import jp.kz2601.metamon.dsl.isVoiceOnly
import jp.kz2601.metamon.language.AppLocales
import jp.kz2601.metamon.language.LanguageSection
import jp.kz2601.metamon.push.PushConsentSection
import jp.kz2601.metamon.renderer.components.social.BlockedUsersSection
import jp.kz2601.metamon.push.PushHost
import jp.kz2601.metamon.services.RemainingToday
import jp.kz2601.metamon.voice.ClarifySection
import jp.kz2601.metamon.voice.ClarifySettings
import jp.kz2601.metamon.voice.SpeechSection
import jp.kz2601.metamon.voice.SpeechSettings
import java.time.ZoneId
import java.time.format.DateTimeFormatter
import java.time.format.FormatStyle

/**
 * 設定(iOS の SettingsSheet): 相談の ON / OFF(#893 の [ClarifySection])・投稿に表示する名前(#875)・
 * プライバシー(AI へのデータ送信の同意)・クラッシュ報告・AI ボタンの利用状況(#888)・他の人の行動で届く通知(#887 の [PushConsentSection])・
 * 記録の自動削除(#482 の [RetentionSettingsSection])・今日の残り(#630 の [RemainingTodaySection])・
 * アプリ(元に戻す・使い方・頼み方のコツ・リセット・#898 の [AppActionsSection])・データ(同期の状態・#898 の [SyncStatusSection])・
 * 言語(#843 の [LanguageSection])・お問い合わせ・ライセンス・詳細(サポート用)(#898 の [SupportSection])・
 * アカウント(ログアウト・削除・#841 の [AccountSection])。
 * @param busy 生成中(アカウント削除をさせない)
 * @param remainingToday 音声の画面が最後に読んだ「今日の残り」。[onRefreshUsage] で開いた時に読み直す
 * @param onStopRetention 「自動で消すのをやめる」の指示文を送る(送ったらシートを閉じる)
 * @param onShowTutorial 「使い方を見る」(#900)。シートを閉じて初回のチュートリアルをもう一度出す
 * @param canUndo 「元に戻す」を押せる(#1075)
 * @param onUndo シートを閉じて直前の版に戻す
 * @param onPickTip 「頼み方のコツ」で選んだ文を入力欄に入れる(シートを閉じる・送らない)
 * @param onReset 「アプリをリセット」(確認の後・シートを閉じる)
 */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun SettingsSheet(
    container: AppContainer,
    busy: Boolean,
    onDismiss: () -> Unit,
    remainingToday: RemainingToday? = null,
    isPro: Boolean = false,
    onRefreshUsage: () -> Unit = {},
    onStopRetention: (String) -> Unit = {},
    onShowTutorial: () -> Unit = {},
    canUndo: Boolean = false,
    onUndo: () -> Unit = {},
    onPickTip: (String) -> Unit = {},
    onReset: () -> Unit = {},
) {
    LaunchedEffect(Unit) { onRefreshUsage() }
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
            val context = LocalContext.current
            ClarifySection(remember { ClarifySettings(context) })
            // 挨拶・AI の質問・完成の要約の読み上げ(#892・既定 OFF)
            SpeechSection(remember { SpeechSettings(context) })
            HorizontalDivider(Modifier.padding(vertical = 8.dp))
            AuthorNameSection(container.authorProfile)
            HorizontalDivider(Modifier.padding(vertical = 8.dp))
            PrivacySection(container)
            HorizontalDivider(Modifier.padding(vertical = 8.dp))
            // 他の人の行動で届く通知(#887)。共有・公開の記録があるアプリだけ
            val document = container.runtime.state.collectAsStateWithLifecycle().value.document
            if (container.push != null && PushHost.canSetConsent(document.dsl)) {
                PushConsentSection(container.push, document.appId, document.dsl)
                HorizontalDivider(Modifier.padding(vertical = 8.dp))
            }
            // 記録の自動削除(#482)。今のアプリに保存期間のある記録がある時だけ
            RetentionSettingsSection(container.runtime, onStop = { text ->
                onStopRetention(text)
                onDismiss()
            })
            // ブロック中のユーザー(#877)。通報・ブロックが使える時だけ
            BlockedUsersSection(container.runtime)
            // 今日の残り(#630)。AI を使うまでは「AI を使うと表示されます」
            RemainingTodaySection(remainingToday, isPro)
            HorizontalDivider(Modifier.padding(vertical = 8.dp))
            HorizontalDivider(Modifier.padding(vertical = 8.dp))
            // アプリ(#898・iOS の appSection): 元に戻す・画面マップ・自動の動き・使い方を見る(#900)・頼み方のコツ・リセット
            val runtimeState = container.runtime.state.collectAsStateWithLifecycle().value
            AppActionsSection(
                AppActionsState(
                    appName = runtimeState.dsl.app.name,
                    offline = runtimeState.offline,
                    canUndo = canUndo,
                    busy = busy,
                    hasScreens = !runtimeState.dsl.isVoiceOnly,
                    hasServerRules = runtimeState.dsl.serverRules.isNotEmpty(),
                ),
                onUndo = onUndo,
                onShowTutorial = onShowTutorial,
                onPickTip = onPickTip,
                onReset = onReset,
            )
            HorizontalDivider(Modifier.padding(vertical = 8.dp))
            // データ(#898 / #990・iOS の syncSection): 同期の状態と「今すぐ同期」
            SyncStatusSection(container.recordSync)
            HorizontalDivider(Modifier.padding(vertical = 8.dp))
            var language by remember { mutableStateOf(AppLocales.setting(context)) }
            LanguageSection(language, onSelect = { selected ->
                if (selected == language) return@LanguageSection
                language = selected
                // Android 12 / 12L は開いている画面を作り直す(13+ はシステムがやる)
                if (AppLocales.set(context, selected)) (context as? Activity)?.recreate()
            })
            HorizontalDivider(Modifier.padding(vertical = 8.dp))
            // お問い合わせ・ライセンス・詳細(サポート用)(#898・iOS の supportSection)
            SupportSection(remember(runtimeState.document.version) { supportInfo(context, container, runtimeState.document.version) })
            // ログアウト・削除の後は本体の画面ごと消える(後片付けは画面より長く生きる scope で回る)
            container.accountGuard.active.collectAsStateWithLifecycle().value?.let { user ->
                HorizontalDivider(Modifier.padding(vertical = 8.dp))
                AccountSection(container, user, busy = busy)
            }
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

/** 「詳細(サポート用)」の値(iOS の `SettingsSheet.supportInfo`)。ユーザー ID は入れない([SupportInfo])。 */
private fun supportInfo(context: Context, container: AppContainer, revision: Int): SupportInfo {
    val mock = container.knobs.mock
    val pkg = runCatching { context.packageManager.getPackageInfo(context.packageName, 0) }.getOrNull()
    return SupportInfo(
        voiceModel = container.transcriber::class.simpleName ?: "-",
        aiEndpoint = if (mock) context.getString(R.string.settings_support_mock) else SupportInfo.host(container.environment.aiBaseUrl),
        appConfigEndpoint = if (mock) context.getString(R.string.settings_support_in_memory) else SupportInfo.host(container.environment.appConfigBaseUrl),
        appVersion = pkg?.versionName ?: "-",
        build = pkg?.longVersionCode?.toString() ?: "-",
        appRevision = revision,
    )
}
