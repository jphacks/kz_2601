package jp.kz2601.metamon.renderer.components.location

import android.Manifest
import android.content.Intent
import android.content.pm.PackageManager
import android.net.Uri
import android.os.Build
import android.provider.Settings
import androidx.activity.compose.rememberLauncherForActivityResult
import androidx.activity.result.contract.ActivityResultContracts
import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.collectAsState
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableLongStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import jp.kz2601.metamon.dsl.Component
import jp.kz2601.metamon.dsl.records.Track
import jp.kz2601.metamon.renderer.R
import jp.kz2601.metamon.renderer.components.UnavailableComponent
import jp.kz2601.metamon.renderer.components.actions.ActionButton
import jp.kz2601.metamon.renderer.components.actions.ActionButtonKind
import jp.kz2601.metamon.renderer.components.dashcam.LocalDashcamAppName
import jp.kz2601.metamon.renderer.registry.RenderContext
import jp.kz2601.metamon.renderer.theme.RenderTheme
import jp.kz2601.metamon.renderer.theme.colors
import kotlinx.coroutines.delay

/**
 * `locationTracker`(#869・iOS の `LocationTrackerLeaf`): 「記録を開始 / 記録を停止」と状態(経過時間・距離・点の数)。
 * 開始を押した間だけ位置を `entity` に 1 点 1 レコードで足す(`locationField` = "lat,lng"、`dateField` = 日時)。
 * 記録そのものは [LocationTracking](画面を移っても・アプリを閉じても続く)。`seconds` で自動停止(既定 1 時間)。
 */
@Composable
fun LocationTrackerLeaf(c: Component, ctx: RenderContext, modifier: Modifier = Modifier) {
    val appName = LocalDashcamAppName.current
    val target = TrackingTarget.of(c, appName)
    if (target == null) {
        UnavailableComponent(c, modifier)
        return
    }
    val context = LocalContext.current
    val state by LocationTracking.state.collectAsState()
    val autoStop = Track.autoStopSeconds(c.seconds)
    val runtime = ctx.runtime

    fun begin() {
        // 正確な位置が無いと点が粗すぎて捨てるだけなので、記録は正確な位置の許可がある時だけ
        if (context.checkSelfPermission(Manifest.permission.ACCESS_FINE_LOCATION) != PackageManager.PERMISSION_GRANTED) {
            LocationTracking.refused(target, autoStop)
            return
        }
        LocationTracking.start(context, target, autoStop, { runtime.entityModel(target.entity) }) { data ->
            runtime.addRecord(target.entity, data)
        }
    }

    // 通知の許可(13+)は聞くだけ。断られても記録はする(通知は OS のタスクマネージャーに出る)
    val askNotifications = rememberLauncherForActivityResult(ActivityResultContracts.RequestPermission()) { begin() }
    val askLocation = LocalLocationPermission.current { granted ->
        when {
            !granted -> LocationTracking.refused(target, autoStop)
            Build.VERSION.SDK_INT >= Build.VERSION_CODES.TIRAMISU &&
                context.checkSelfPermission(Manifest.permission.POST_NOTIFICATIONS) != PackageManager.PERMISSION_GRANTED ->
                askNotifications.launch(Manifest.permission.POST_NOTIFICATIONS)
            else -> begin()
        }
    }

    Column(modifier.fillMaxWidth(), verticalArrangement = Arrangement.spacedBy(RenderTheme.Spacing.sm)) {
        when {
            state.isRecording(target) -> {
                TrackerStatus(state)
                ActionButton(
                    label = stringResource(R.string.renderer_tracker_stop),
                    kind = ActionButtonKind.DESTRUCTIVE,
                    icon = "stop.fill",
                    onClick = { LocationTracking.stop() },
                    modifier = Modifier.testTag("locationTracker.stop"),
                )
            }
            state.isActive -> {
                // 別のアプリ・別の部品で記録中(同時に 2 本は取らない)
                Text(stringResource(R.string.renderer_tracker_other_active), style = MaterialTheme.typography.bodySmall, color = RenderTheme.colors.secondaryLabel)
                ActionButton(
                    label = stringResource(R.string.renderer_tracker_stop_other),
                    kind = ActionButtonKind.SECONDARY,
                    icon = "stop.fill",
                    onClick = { LocationTracking.stop() },
                )
            }
            else -> {
                ActionButton(
                    label = c.label ?: stringResource(R.string.renderer_tracker_start),
                    kind = ActionButtonKind.PRIMARY,
                    icon = "record.circle",
                    onClick = askLocation,
                    modifier = Modifier.testTag("locationTracker.start"),
                )
                TrackerFootnote(state, target, autoStop)
            }
        }
    }
}

/** 記録中: ● 記録中 12:34 ・ 1.23 km / 18 点を記録・あと 47 分で自動停止 */
@Composable
private fun TrackerStatus(state: TrackingState) {
    var now by remember { mutableLongStateOf(System.currentTimeMillis()) }
    LaunchedEffect(Unit) {
        while (true) {
            now = System.currentTimeMillis()
            delay(1_000)
        }
    }
    Column(Modifier.semantics(mergeDescendants = true) {}) {
        Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(6.dp)) {
            RecordingDot()
            Text(
                stringResource(if (state.phase == TrackingPhase.REQUESTING) R.string.renderer_tracker_requesting else R.string.renderer_tracker_recording),
                style = MaterialTheme.typography.bodyMedium,
                fontWeight = FontWeight.SemiBold,
            )
            Spacer(Modifier.weight(1f))
            Text(
                "${TrackingRules.clock(state.status.elapsedSeconds(now))} ・ ${Track.formatDistance(state.distance)}",
                style = MaterialTheme.typography.bodyMedium,
            )
        }
        Text(
            if (state.savedCount == 0) stringResource(R.string.renderer_tracker_waiting)
            else stringResource(R.string.renderer_tracker_progress, state.savedCount, TrackingRules.remainingMinutes(state, now).toInt()),
            style = MaterialTheme.typography.bodySmall,
            color = RenderTheme.colors.secondaryLabel,
        )
    }
}

@Composable
private fun TrackerFootnote(state: TrackingState, target: TrackingTarget, autoStop: Int) {
    val context = LocalContext.current
    val mine = state.target == target
    when {
        mine && state.phase == TrackingPhase.DENIED -> Column(verticalArrangement = Arrangement.spacedBy(RenderTheme.Spacing.xs)) {
            Text(stringResource(R.string.renderer_tracker_denied), style = MaterialTheme.typography.bodySmall, color = RenderTheme.colors.negative)
            TextButton(onClick = {
                context.startActivity(
                    Intent(Settings.ACTION_APPLICATION_DETAILS_SETTINGS, Uri.fromParts("package", context.packageName, null))
                        .addFlags(Intent.FLAG_ACTIVITY_NEW_TASK),
                )
            }) { Text(stringResource(R.string.renderer_tracker_open_settings)) }
        }
        mine && state.stopReason == TrackingStopReason.SAVE_FAILED ->
            Text(stringResource(R.string.renderer_tracker_save_failed), style = MaterialTheme.typography.bodySmall, color = RenderTheme.colors.negative)
        mine && state.stopReason == TrackingStopReason.UNAVAILABLE ->
            Text(stringResource(R.string.renderer_tracker_unavailable), style = MaterialTheme.typography.bodySmall, color = RenderTheme.colors.negative)
        mine && state.stopReason == TrackingStopReason.AUTO_STOP ->
            Text(stringResource(R.string.renderer_tracker_auto_stopped, state.autoStopSeconds / 60), style = MaterialTheme.typography.bodySmall, color = RenderTheme.colors.secondaryLabel)
        else ->
            Text(stringResource(R.string.renderer_tracker_footnote, autoStop / 60), style = MaterialTheme.typography.bodySmall, color = RenderTheme.colors.secondaryLabel)
    }
}

/** 記録中の赤い点。 */
@Composable
internal fun RecordingDot() {
    Box(Modifier.size(8.dp).background(Color.Red, CircleShape))
}
