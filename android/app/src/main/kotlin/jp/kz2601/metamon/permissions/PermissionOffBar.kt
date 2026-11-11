package jp.kz2601.metamon.permissions

import androidx.activity.compose.LocalActivity
import androidx.compose.animation.AnimatedVisibility
import androidx.compose.animation.expandVertically
import androidx.compose.animation.fadeIn
import androidx.compose.animation.fadeOut
import androidx.compose.animation.shrinkVertically
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Close
import androidx.compose.material.icons.filled.Warning
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.key
import androidx.compose.runtime.remember
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.LiveRegionMode
import androidx.compose.ui.semantics.liveRegion
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import androidx.lifecycle.Lifecycle
import androidx.lifecycle.compose.LifecycleEventEffect
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import jp.kz2601.metamon.R
import jp.kz2601.metamon.dsl.AppDSL
import jp.kz2601.metamon.notifications.NotificationsFirstAsk
import jp.kz2601.metamon.services.permissions.AppPermission
import jp.kz2601.metamon.services.permissions.PermissionNeeds
import jp.kz2601.metamon.services.permissions.PermissionOffBarModel
import jp.kz2601.metamon.services.permissions.PermissionRules
import jp.kz2601.metamon.services.permissions.PermissionStatus
import jp.kz2601.metamon.services.permissions.PermissionStep

/**
 * 生成したアプリの上に「オフ」のバーを出す(iOS の NotificationsOffBarHost を権限全部に広げたもの)。
 * 閉じたらそのアプリの間は出さない(別のアプリに切り替えたらまた判定)。許可されたら消える(画面に戻るたびに取り直す)。
 */
@Composable
fun PermissionOffBarHost(dsl: AppDSL, appId: String, busy: Boolean, summaryShowing: Boolean) {
    val context = LocalContext.current
    val activity = LocalActivity.current
    val center = remember(context) { PermissionCenter.get(context) }
    val statuses by center.statuses.collectAsStateWithLifecycle()
    LifecycleEventEffect(Lifecycle.Event.ON_RESUME) { center.refresh(activity) }
    val needed = remember(dsl) { PermissionNeeds.of(dsl) }
    // 通知のあるアプリは、未確認なら開いた時に 1 回だけ聞く(#886)
    NotificationsFirstAsk(AppPermission.NOTIFICATIONS in needed, appId, busy = busy || summaryShowing)
    var dismissed by rememberSaveable(appId) { mutableStateOf(emptyList<String>()) }
    val shown = PermissionOffBarModel.pick(
        needed = needed,
        statuses = statuses,
        dismissed = dismissed.mapNotNull { n -> AppPermission.entries.firstOrNull { it.name == n } }.toSet(),
        busy = busy,
        summaryShowing = summaryShowing,
    )
    AnimatedVisibility(shown != null, enter = expandVertically() + fadeIn(), exit = shrinkVertically() + fadeOut()) {
        val permission = shown ?: return@AnimatedVisibility
        key(permission) {
            val status = statuses[permission] ?: PermissionStatus.UNAVAILABLE
            val request = rememberPermissionRequest(permission) { center.refresh(activity) }
            PermissionOffBar(
                permission = permission,
                canAsk = PermissionRules.nextStep(status) == PermissionStep.EXPLAIN_THEN_ASK,
                onAction = { if (PermissionRules.nextStep(status) == PermissionStep.EXPLAIN_THEN_ASK) request() else center.openSettings(context, permission) },
                onDismiss = { dismissed = dismissed + permission.name },
            )
        }
    }
}

/** バー: 文言 [許可する / 設定を開く] [×]。まだ聞ける時は「許可する」(Android は断られても 1 回は聞き直せる)。 */
@Composable
fun PermissionOffBar(permission: AppPermission, canAsk: Boolean, onAction: () -> Unit, onDismiss: () -> Unit, modifier: Modifier = Modifier) {
    Surface(color = MaterialTheme.colorScheme.surfaceContainerHigh, modifier = modifier.fillMaxWidth()) {
        Column {
            Row(
                Modifier.padding(start = 16.dp, end = 4.dp).semantics { liveRegion = LiveRegionMode.Polite },
                verticalAlignment = Alignment.CenterVertically,
            ) {
                Icon(Icons.Filled.Warning, contentDescription = null, tint = MaterialTheme.colorScheme.tertiary, modifier = Modifier.size(20.dp))
                Text(
                    stringResource(PermissionCopy.off(permission)),
                    style = MaterialTheme.typography.bodySmall.copy(fontWeight = FontWeight.Medium),
                    modifier = Modifier.weight(1f).padding(horizontal = 10.dp, vertical = 8.dp),
                )
                OutlinedButton(onClick = onAction, modifier = Modifier.heightIn(min = 44.dp)) {
                    Text(
                        stringResource(if (canAsk) R.string.permission_allow else R.string.permission_open_settings),
                        style = MaterialTheme.typography.labelMedium.copy(fontWeight = FontWeight.SemiBold),
                    )
                }
                IconButton(onClick = onDismiss, modifier = Modifier.size(48.dp)) {
                    Icon(Icons.Filled.Close, contentDescription = stringResource(R.string.permission_dismiss))
                }
            }
            HorizontalDivider()
        }
    }
}
