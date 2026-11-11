package jp.kz2601.metamon.notifications

import android.app.AlarmManager
import android.content.ActivityNotFoundException
import android.content.Context
import android.content.Intent
import android.net.Uri
import android.os.Build
import android.provider.Settings
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.res.stringResource
import jp.kz2601.metamon.R
import jp.kz2601.metamon.dsl.Action
import jp.kz2601.metamon.dsl.ActionType
import jp.kz2601.metamon.dsl.AppDSL
import jp.kz2601.metamon.dsl.Component
import jp.kz2601.metamon.dsl.ComponentMode
import jp.kz2601.metamon.dsl.ComponentType
import jp.kz2601.metamon.services.consent.KeyValueStore
import jp.kz2601.metamon.services.consent.SharedPreferencesKeyValueStore

/**
 * 正確なアラームの許可の案内(#1123)。端末全体で 1 回だけ(断られたら聞き直さない)。
 * Android 14 以降の新しいインストールでは `SCHEDULE_EXACT_ALARM` が既定でオフ。
 */
class ExactAlarmPrompt(private val store: KeyValueStore) {
    fun shouldShow(needed: Boolean, sdkInt: Int, canScheduleExact: Boolean): Boolean =
        needed && sdkInt >= Build.VERSION_CODES.S && !canScheduleExact && store.getBoolean(KEY_SHOWN) != true

    fun markShown() = store.putBoolean(KEY_SHOWN, true)

    /** サインアウト・別の人(iOS の `LocalWipe.clearDefaults`)。 */
    fun forget() = store.remove(KEY_SHOWN)

    companion object {
        const val KEY_SHOWN = "exactAlarm.prompted.v1"

        fun of(context: Context) = ExactAlarmPrompt(SharedPreferencesKeyValueStore.of(context, TimerAlarms.PREFS))

        /** 正確な時刻が要るか: アラーム(`alarm: true`)の通知か、countdown の timerDisplay がある。 */
        fun needed(dsl: AppDSL): Boolean =
            dsl.app.notifications.any { it.alarm == true } ||
                dsl.screens.any { s -> s.toolbar.any { needs(it.action) } || s.components.any(::needs) }

        private fun needs(c: Component): Boolean =
            (c.type == ComponentType.TIMER_DISPLAY && c.mode == ComponentMode.COUNTDOWN) ||
                needs(c.action) || needs(c.onItemTap) || needs(c.onDateTap) || c.children.any(::needs)

        private fun needs(a: Action?): Boolean = a?.type == ActionType.SCHEDULE_NOTIFICATION && a.notification?.alarm == true
    }
}

/** 正確なアラームが要るアプリを開いた時、許可が無ければ理由を出して設定を開く(1 回だけ)。 */
@Composable
fun ExactAlarmFirstAsk(needed: Boolean, busy: Boolean) {
    val context = LocalContext.current
    val prompt = remember(context) { ExactAlarmPrompt.of(context) }
    var showing by remember { mutableStateOf(false) }
    LaunchedEffect(needed, busy) {
        if (busy || showing) return@LaunchedEffect
        val alarms = context.getSystemService(AlarmManager::class.java)
        if (prompt.shouldShow(needed, Build.VERSION.SDK_INT, alarms.canScheduleExactAlarms())) {
            prompt.markShown()
            showing = true
        }
    }
    if (!showing) return
    AlertDialog(
        onDismissRequest = { showing = false },
        title = { Text(stringResource(R.string.exact_alarm_prompt_title)) },
        text = { Text(stringResource(R.string.exact_alarm_prompt_body)) },
        confirmButton = {
            TextButton(onClick = {
                showing = false
                openSettings(context)
            }) { Text(stringResource(R.string.exact_alarm_prompt_open)) }
        },
        dismissButton = {
            TextButton(onClick = { showing = false }) { Text(stringResource(R.string.exact_alarm_prompt_later)) }
        },
    )
}

private fun openSettings(context: Context) {
    val intent = Intent(Settings.ACTION_REQUEST_SCHEDULE_EXACT_ALARM, Uri.parse("package:${context.packageName}"))
        .addFlags(Intent.FLAG_ACTIVITY_NEW_TASK)
    try {
        context.startActivity(intent)
    } catch (_: ActivityNotFoundException) {
        // 設定の画面が無い端末。許可が変わった時の予約し直しは NotificationReceiver が受ける
    }
}
