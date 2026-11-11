package jp.kz2601.metamon.settings

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
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.unit.dp
import jp.kz2601.metamon.R
import jp.kz2601.metamon.voice.PromptTipsSheet

/** 設定の「アプリ」の節に出す行(iOS の `SettingsSheet.appSection`)。 */
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
    /** サーバーの自動の動き(serverRules)があるアプリ。自動の動きの行を出す(iOS の `AutomationsSettingsRow`・#905) */
    val hasServerRules: Boolean,
)

/**
 * 設定の「アプリ」(iOS の `SettingsSheet.appSection`): 元に戻す(#1075)・画面マップ・自動の動き・使い方を見る(#900 / #1236)・
 * 頼み方のコツ(#1161)・リセット(初期状態に戻す)。
 * @param onUndo シートを閉じて直前の版に戻す
 * @param onOpenScreenMap 画面マップ(#897)を開く
 * @param onOpenAutomations 自動の動き(#905)を開く。null は使えない構成(審査用の版など)で、行を出さない
 * @param onShowTutorial シートを閉じてチュートリアルをもう一度出す
 * @param onReplayCoachMarks シートを閉じて操作のヒント(吹き出し)を未読に戻す
 * @param onPickTip 選んだコツの文を入力欄に入れる(送らない・シートを閉じる)
 * @param onReset 初期状態に戻す(確認の後。シートを閉じる)
 */
@Composable
fun AppActionsSection(
    state: AppActionsState,
    onUndo: () -> Unit,
    onOpenScreenMap: () -> Unit,
    onOpenAutomations: (() -> Unit)?,
    onShowTutorial: () -> Unit,
    onReplayCoachMarks: () -> Unit,
    onPickTip: (String) -> Unit,
    onReset: () -> Unit,
) {
    var tipsOpen by rememberSaveable { mutableStateOf(false) }
    var confirmReset by rememberSaveable { mutableStateOf(false) }

    SettingsHeader(stringResource(R.string.settings_app_header))
    TextButton(onClick = onUndo, enabled = state.canUndo, modifier = Modifier.padding(horizontal = 4.dp).testTag("settings.undo")) {
        Text(stringResource(R.string.settings_undo))
    }
    if (state.hasScreens) {
        TextButton(onClick = onOpenScreenMap, modifier = Modifier.padding(horizontal = 4.dp).testTag("settings.screenMap")) {
            Text(stringResource(R.string.settings_screen_map))
        }
    }
    if (state.hasServerRules && onOpenAutomations != null) {
        TextButton(onClick = onOpenAutomations, modifier = Modifier.padding(horizontal = 4.dp).testTag("settings.automations")) {
            Text(stringResource(R.string.settings_automations))
        }
    }
    TextButton(onClick = onShowTutorial, modifier = Modifier.padding(horizontal = 4.dp).testTag("settings.tutorial")) {
        Text(stringResource(R.string.onboarding_replay))
    }
    TextButton(onClick = onReplayCoachMarks, modifier = Modifier.padding(horizontal = 4.dp).testTag("settings.coachMarks")) {
        Text(stringResource(R.string.coach_mark_replay))
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
