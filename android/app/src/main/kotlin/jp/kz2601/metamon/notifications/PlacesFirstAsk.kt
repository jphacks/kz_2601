package jp.kz2601.metamon.notifications

import androidx.activity.compose.LocalActivity
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.stringResource
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import jp.kz2601.metamon.R
import jp.kz2601.metamon.permissions.PermissionCenter
import jp.kz2601.metamon.permissions.rememberPermissionRequest
import jp.kz2601.metamon.services.permissions.AppPermission
import jp.kz2601.metamon.services.permissions.NearPermissionFlow
import jp.kz2601.metamon.services.permissions.PermissionStatus

/**
 * 場所に着いたら知らせる(near)のあるアプリを開いた時の許可の 2 段([NearPermissionFlow])。
 * 1. 位置がまだ未確認なら、理由のダイアログ → OS のダイアログ(iOS は場所のあるアプリの予約の時に 1 回だけ聞く)
 * 2. 許可されたら、裏での位置の説明の画面 → OS の設定の画面(「常に許可」)
 * 通知の許可を先に聞く([NotificationsFirstAsk])ので、通知が未確認の間は待つ。断られた後は「オフ」のバーが案内する。
 */
@Composable
fun PlacesFirstAsk(needed: Boolean, appId: String, busy: Boolean) {
    val context = LocalContext.current
    val activity = LocalActivity.current
    val center = remember(context) { PermissionCenter.get(context) }
    val statuses by center.statuses.collectAsStateWithLifecycle()
    var asked by rememberSaveable(appId) { mutableStateOf(false) }
    var explainBackground by rememberSaveable(appId) { mutableStateOf(false) }
    val askBackground = rememberPermissionRequest(AppPermission.BACKGROUND_LOCATION, explain = false) { granted ->
        center.refresh(activity)
        if (granted) LocalNotifications.get(context).refresh()
    }
    val askForeground = rememberPermissionRequest(AppPermission.LOCATION) { granted ->
        center.refresh(activity)
        val background = center.status(AppPermission.BACKGROUND_LOCATION, activity)
        if (granted && NearPermissionFlow.next(PermissionStatus.GRANTED, background) == NearPermissionFlow.Step.EXPLAIN_BACKGROUND) {
            explainBackground = true
        }
    }
    val notificationsAsked = statuses[AppPermission.NOTIFICATIONS]?.let { it != PermissionStatus.NOT_ASKED } ?: false
    LaunchedEffect(needed, appId, busy, notificationsAsked) {
        if (!needed || busy || asked || !notificationsAsked) return@LaunchedEffect
        val step = NearPermissionFlow.next(
            center.status(AppPermission.LOCATION, activity),
            center.status(AppPermission.BACKGROUND_LOCATION, activity),
        )
        if (step == NearPermissionFlow.Step.NONE) return@LaunchedEffect
        asked = true
        when (step) {
            NearPermissionFlow.Step.ASK_FOREGROUND -> askForeground()
            NearPermissionFlow.Step.EXPLAIN_BACKGROUND -> explainBackground = true
            NearPermissionFlow.Step.NONE -> Unit
        }
    }
    if (explainBackground) {
        BackgroundLocationExplainer(
            optionLabel = remember(context) { context.packageManager.backgroundPermissionOptionLabel.toString() },
            onContinue = {
                explainBackground = false
                askBackground()
            },
            onDismiss = { explainBackground = false },
        )
    }
}

/** 裏での位置を聞く前の説明(OS の設定の画面で選ぶ項目の名前 [optionLabel] を出す)。 */
@Composable
private fun BackgroundLocationExplainer(optionLabel: String, onContinue: () -> Unit, onDismiss: () -> Unit) {
    AlertDialog(
        onDismissRequest = onDismiss,
        modifier = Modifier.testTag("places.backgroundLocation"),
        title = { Text(stringResource(R.string.permission_background_title)) },
        text = { Text(stringResource(R.string.permission_background_body, optionLabel)) },
        confirmButton = { TextButton(onClick = onContinue) { Text(stringResource(R.string.permission_continue)) } },
        dismissButton = { TextButton(onClick = onDismiss) { Text(stringResource(R.string.permission_not_now)) } },
    )
}
