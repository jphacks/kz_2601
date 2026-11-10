package jp.kz2601.metamon.permissions

import androidx.activity.compose.LocalActivity
import androidx.activity.compose.rememberLauncherForActivityResult
import androidx.activity.result.contract.ActivityResultContracts
import androidx.annotation.StringRes
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.rememberUpdatedState
import androidx.compose.runtime.setValue
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.res.stringResource
import jp.kz2601.metamon.MetamonApplication
import jp.kz2601.metamon.R
import jp.kz2601.metamon.services.permissions.AppPermission
import jp.kz2601.metamon.services.permissions.PermissionRules
import jp.kz2601.metamon.services.permissions.PermissionStatus
import jp.kz2601.metamon.services.permissions.PermissionStep
import kotlinx.coroutines.launch

/**
 * 使う直前に権限を聞く関数を返す(iOS: 使う直前に理由 → OS のダイアログ。断られた後は「設定を開く」)。
 * - 許可済み: すぐ `onResult(true)`
 * - まだ聞ける: 理由のダイアログ([explain] が false なら出さない)→ OS のダイアログ
 * - 二度と聞かない: 理由と「設定を開く」のダイアログ([explain] が false なら出さずに `onResult(false)`)
 */
@Composable
fun rememberPermissionRequest(permission: AppPermission, explain: Boolean = true, onResult: (Boolean) -> Unit): () -> Unit {
    val context = LocalContext.current
    val activity = LocalActivity.current
    val center = remember(context) { PermissionCenter.get(context) }
    val callback by rememberUpdatedState(onResult)
    val scope = rememberCoroutineScope()
    var dialog by remember { mutableStateOf<PermissionStep?>(null) }
    var before by remember { mutableStateOf(PermissionStatus.NOT_ASKED) }
    val launcher = rememberLauncherForActivityResult(ActivityResultContracts.RequestMultiplePermissions()) { results ->
        val after = center.record(permission, results, before, activity)
        // アプリ内の OS のダイアログはプロセスの前面・背面を通らないので、通知の許可はここで端末の登録を確かめる(#887・#891)
        if (permission == AppPermission.NOTIFICATIONS && after == PermissionStatus.GRANTED) {
            (context.applicationContext as? MetamonApplication)?.builtContainer?.push?.let { scope.launch { it.refresh() } }
        }
        callback(after == PermissionStatus.GRANTED)
    }
    val ask = {
        before = center.status(permission, activity)
        launcher.launch(center.requestNames(permission))
    }

    when (dialog) {
        PermissionStep.EXPLAIN_THEN_ASK -> AlertDialog(
            onDismissRequest = { dialog = null; callback(false) },
            title = { Text(stringResource(PermissionCopy.name(permission)).replaceFirstChar { it.uppercase() }) },
            text = { Text(stringResource(PermissionCopy.reason(permission))) },
            confirmButton = { TextButton(onClick = { dialog = null; ask() }) { Text(stringResource(R.string.permission_continue)) } },
            dismissButton = { TextButton(onClick = { dialog = null; callback(false) }) { Text(stringResource(R.string.permission_not_now)) } },
        )
        PermissionStep.OPEN_SETTINGS -> {
            val name = stringResource(PermissionCopy.name(permission))
            AlertDialog(
                onDismissRequest = { dialog = null; callback(false) },
                title = { Text(stringResource(R.string.permission_settings_title, name)) },
                text = { Text(stringResource(PermissionCopy.reason(permission)) + "\n\n" + stringResource(R.string.permission_settings_body, name)) },
                confirmButton = {
                    TextButton(onClick = { dialog = null; center.openSettings(context, permission); callback(false) }) {
                        Text(stringResource(R.string.permission_open_settings))
                    }
                },
                dismissButton = { TextButton(onClick = { dialog = null; callback(false) }) { Text(stringResource(R.string.permission_cancel)) } },
            )
        }
        else -> Unit
    }

    return {
        when (PermissionRules.nextStep(center.status(permission, activity))) {
            PermissionStep.PROCEED -> callback(true)
            PermissionStep.EXPLAIN_THEN_ASK -> if (explain) dialog = PermissionStep.EXPLAIN_THEN_ASK else ask()
            PermissionStep.OPEN_SETTINGS -> if (explain) dialog = PermissionStep.OPEN_SETTINGS else callback(false)
            PermissionStep.UNAVAILABLE -> callback(false)
        }
    }
}

/** 権限ごとの文言(strings_permissions.xml)。 */
object PermissionCopy {
    @StringRes
    fun name(p: AppPermission): Int = when (p) {
        AppPermission.MICROPHONE -> R.string.permission_name_microphone
        AppPermission.CAMERA -> R.string.permission_name_camera
        AppPermission.LOCATION -> R.string.permission_name_location
        AppPermission.NOTIFICATIONS -> R.string.permission_name_notifications
        AppPermission.NEARBY -> R.string.permission_name_nearby
        AppPermission.HEALTH -> R.string.permission_name_health
    }

    @StringRes
    fun reason(p: AppPermission): Int = when (p) {
        AppPermission.MICROPHONE -> R.string.permission_reason_microphone
        AppPermission.CAMERA -> R.string.permission_reason_camera
        AppPermission.LOCATION -> R.string.permission_reason_location
        AppPermission.NOTIFICATIONS -> R.string.permission_reason_notifications
        AppPermission.NEARBY -> R.string.permission_reason_nearby
        AppPermission.HEALTH -> R.string.permission_reason_health
    }

    @StringRes
    fun off(p: AppPermission): Int = when (p) {
        AppPermission.MICROPHONE -> R.string.permission_off_microphone
        AppPermission.CAMERA -> R.string.permission_off_camera
        AppPermission.LOCATION -> R.string.permission_off_location
        AppPermission.NOTIFICATIONS -> R.string.permission_off_notifications
        AppPermission.NEARBY -> R.string.permission_off_nearby
        AppPermission.HEALTH -> R.string.permission_off_health
    }
}
