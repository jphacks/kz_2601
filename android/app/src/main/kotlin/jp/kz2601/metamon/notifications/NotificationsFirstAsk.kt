package jp.kz2601.metamon.notifications

import androidx.activity.compose.LocalActivity
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.platform.LocalContext
import jp.kz2601.metamon.permissions.PermissionCenter
import jp.kz2601.metamon.permissions.rememberPermissionRequest
import jp.kz2601.metamon.services.permissions.AppPermission
import jp.kz2601.metamon.services.permissions.PermissionStatus

/**
 * 通知のあるアプリを開いた時、通知の許可がまだ未確認なら 1 回だけ聞く(iOS は予約の時に未決定なら聞く)。
 * 理由のダイアログ → OS のダイアログは [rememberPermissionRequest]。断られた後は「オフ」のバーが案内する。
 */
@Composable
fun NotificationsFirstAsk(needed: Boolean, appId: String, busy: Boolean) {
    val context = LocalContext.current
    val activity = LocalActivity.current
    val center = remember(context) { PermissionCenter.get(context) }
    var asked by rememberSaveable(appId) { mutableStateOf(false) }
    val request = rememberPermissionRequest(AppPermission.NOTIFICATIONS) { granted ->
        center.refresh(activity)
        if (granted) LocalNotifications.get(context).refresh()
    }
    LaunchedEffect(needed, appId, busy) {
        if (!needed || busy || asked) return@LaunchedEffect
        if (center.status(AppPermission.NOTIFICATIONS, activity) != PermissionStatus.NOT_ASKED) return@LaunchedEffect
        asked = true
        request()
    }
}
