package jp.kz2601.metamon.settings

import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.unit.dp
import jp.kz2601.metamon.R
import jp.kz2601.metamon.voice.PromptTipsSheet

/** 設定の「アプリ」の節に出す行(iOS の `SettingsSheet.appSection`)。画面マップと自動の動きは Android にまだ無い(#897 / #905)。 */
data class AppActionsState(
    val appName: String,
    /** 起動時にサーバーへ届かず、保存済みの DSL を表示中 */
    val offline: Boolean,
    /** 「元に戻す」を押せる(#1075: 前の版がある・生成中や戻している最中でない) */
    val canUndo: Boolean,
    /** 生成中・戻している最中(リセットを押させない。iOS の `isMorphing`) */
    val busy: Boolean,
    /** 作ったアプリを表示中(初期の音声画面だけでない)。画面マップの行を出す(iOS の `ScreenMapSettingsRow`) */
    val hasScreens: Boolean,
    /** サーバーの自動の動き(serverRules)があるアプリ。自動の動きの行を出す(iOS の `AutomationsSettingsRow`) */
    val hasServerRules: Boolean,
)

/**
 * 設定の「アプリ」(iOS の `SettingsSheet.appSection`): 元に戻す(#1075)・画面マップ・自動の動き・使い方を見る(#900 / #1236)・
 * 頼み方のコツ(#1161)・リセット(初期状態に戻す)。Android に無い画面マップ・自動の動きは「この端末では使えません」と出すだけ。
 * @param onUndo シートを閉じて直前の版に戻す
 * @param onShowTutorial シートを閉じてチュートリアルをもう一度出す
 * @param onPickTip 選んだコツの文を入力欄に入れる(送らない・シートを閉じる)
 * @param onReset 初期状態に戻す(確認の後。シートを閉じる)
 */
@Composable
fun AppActionsSection(
    state: AppActionsState,
    onUndo: () -> Unit,
    onShowTutorial: () -> Unit,
    onPickTip: (String) -> Unit,
    onReset: () -> Unit,
) {
    var tipsOpen by rememberSaveable { mutableStateOf(false) }
    var confirmReset by rememberSaveable { mutableStateOf(false) }

    SettingsHeader(stringResource(R.string.settings_app_header))
    TextButton(onClick = onUndo, enabled = state.canUndo, modifier = Modifier.padding(horizontal = 4.dp).testTag("settings.undo")) {
        Text(stringResource(R.string.settings_undo))
    }
    if (state.hasScreens) UnavailableRow(stringResource(R.string.settings_screen_map), "settings.screenMap")
    if (state.hasServerRules) UnavailableRow(stringResource(R.string.settings_automations), "settings.automations")
    TextButton(onClick = onShowTutorial, modifier = Modifier.padding(horizontal = 4.dp).testTag("settings.tutorial")) {
        Text(stringResource(R.string.onboarding_replay))
    }
    TextButton(onClick = { tipsOpen = true }, modifier = Modifier.padding(horizontal = 4.dp).testTag("settings.tips")) {
        Text(stringResource(R.string.settings_tips))
    }
    TextButton(
        onClick = { confirmReset = true },
        enabled = !state.busy,
        modifier = Modifier.padding(horizontal = 4.dp).testTag("settings.reset"),
    ) { Text(stringResource(R.string.settings_reset), color = if (state.busy) MaterialTheme.colorScheme.onSurfaceVariant else MaterialTheme.colorScheme.error) }
    SettingsFooter(appFooter(state))

    if (tipsOpen) {
        PromptTipsSheet(onDismiss = { tipsOpen = false }) { text ->
            tipsOpen = false
            onPickTip(text)
        }
    }
    if (confirmReset) {
        AlertDialog(
            onDismissRequest = { confirmReset = false },
            title = { Text(stringResource(R.string.settings_reset_title)) },
            text = { Text(stringResource(R.string.settings_reset_message)) },
            confirmButton = {
                TextButton(onClick = {
                    confirmReset = false
                    onReset()
                }, modifier = Modifier.testTag("settings.reset.confirm")) {
                    Text(stringResource(R.string.settings_reset_confirm), color = MaterialTheme.colorScheme.error)
                }
            },
            dismissButton = { TextButton(onClick = { confirmReset = false }) { Text(stringResource(R.string.settings_revoke_cancel)) } },
        )
    }
}

@Composable
private fun appFooter(state: AppActionsState): String =
    stringResource(R.string.settings_app_footer_current, state.appName) +
        (if (state.offline) stringResource(R.string.settings_app_footer_offline) else "") +
        "\n" + stringResource(R.string.settings_app_footer_hint)

/** Android にまだ無い機能の行(iOS にはある)。押せない・「この端末では使えません」を添える */
@Composable
private fun UnavailableRow(label: String, tag: String) {
    Row(
        Modifier.fillMaxWidth().padding(horizontal = 16.dp, vertical = 10.dp).semantics(mergeDescendants = true) {}.testTag(tag),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        Text(label, color = MaterialTheme.colorScheme.onSurfaceVariant, modifier = Modifier.weight(1f))
        Text(
            stringResource(R.string.settings_unavailable),
            style = MaterialTheme.typography.bodySmall,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
        )
    }
}
