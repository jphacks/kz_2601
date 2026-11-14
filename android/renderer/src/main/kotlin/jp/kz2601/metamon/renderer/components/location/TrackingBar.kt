package jp.kz2601.metamon.renderer.components.location

import androidx.compose.animation.AnimatedVisibility
import androidx.compose.animation.expandVertically
import androidx.compose.animation.fadeIn
import androidx.compose.animation.fadeOut
import androidx.compose.animation.shrinkVertically
import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.collectAsState
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableLongStateOf
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import jp.kz2601.metamon.renderer.R
import jp.kz2601.metamon.renderer.theme.RenderTheme
import jp.kz2601.metamon.renderer.theme.colors
import kotlinx.coroutines.delay

/** 「記録中」バーの出し分け(iOS の `TrackingBarModel`)。 */
object TrackingBarModel {
    fun isVisible(status: TrackingStatus): Boolean = status.isTracking
}

/**
 * 位置の記録中(`locationTracker`)だけ、アプリの上に出す「● 記録中 12:34 [停止]」バー(#869・iOS の `TrackingBarHost`)。
 * 停止は確認してから。
 */
@Composable
fun LocationTrackingBarHost(modifier: Modifier = Modifier) {
    val state by LocationTracking.state.collectAsState()
    val status = state.status
    AnimatedVisibility(
        visible = TrackingBarModel.isVisible(status),
        enter = expandVertically() + fadeIn(),
        exit = shrinkVertically() + fadeOut(),
        modifier = modifier,
    ) {
        TrackingBar(status, onStop = { LocationTracking.stop() })
    }
}

@Composable
fun TrackingBar(status: TrackingStatus, onStop: () -> Unit, modifier: Modifier = Modifier) {
    var confirms by remember { mutableStateOf(false) }
    var now by remember { mutableLongStateOf(System.currentTimeMillis()) }
    LaunchedEffect(Unit) {
        while (true) {
            now = System.currentTimeMillis()
            delay(1_000)
        }
    }
    val elapsed = TrackingRules.clock(status.elapsedSeconds(now))
    val requesting = stringResource(R.string.renderer_tracker_requesting)
    val stopLabel = stringResource(R.string.renderer_tracker_bar_stop_label)
    val spoken = if (status.isRequesting) requesting else stringResource(R.string.renderer_tracker_bar_spoken, elapsed)
    Row(
        modifier
            .fillMaxWidth()
            .background(RenderTheme.colors.surfaceElevated)
            .padding(horizontal = 16.dp, vertical = 2.dp)
            .testTag("trackingBar"),
        verticalAlignment = Alignment.CenterVertically,
        horizontalArrangement = Arrangement.spacedBy(8.dp),
    ) {
        Row(
            Modifier.weight(1f).semantics(mergeDescendants = true) { contentDescription = spoken },
            verticalAlignment = Alignment.CenterVertically,
            horizontalArrangement = Arrangement.spacedBy(8.dp),
        ) {
            RecordingDot()
            Text(
                if (status.isRequesting) requesting else stringResource(R.string.renderer_tracker_recording),
                style = MaterialTheme.typography.bodyMedium,
                fontWeight = FontWeight.SemiBold,
                maxLines = 1,
            )
            if (!status.isRequesting) {
                Text(elapsed, style = MaterialTheme.typography.bodyMedium, color = RenderTheme.colors.secondaryLabel)
            }
        }
        OutlinedButton(
            onClick = { confirms = true },
            modifier = Modifier.heightIn(min = 48.dp).testTag("trackingBar.stop")
                .semantics { contentDescription = stopLabel },
        ) {
            Text(stringResource(R.string.renderer_tracker_stop_short), color = Color.Red, fontWeight = FontWeight.SemiBold)
        }
    }
    if (confirms) {
        AlertDialog(
            onDismissRequest = { confirms = false },
            title = { Text(stringResource(R.string.renderer_tracker_confirm_title)) },
            text = { Text(stringResource(R.string.renderer_tracker_confirm_message)) },
            confirmButton = {
                TextButton(onClick = {
                    confirms = false
                    onStop()
                }) { Text(stringResource(R.string.renderer_tracker_stop_short), color = RenderTheme.colors.negative) }
            },
            dismissButton = {
                TextButton(onClick = { confirms = false }) { Text(stringResource(R.string.renderer_tracker_cancel)) }
            },
        )
    }
}
