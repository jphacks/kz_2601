package jp.kz2601.metamon.nearby

import androidx.activity.compose.LocalActivity
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.defaultMinSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.navigationBarsPadding
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.ModalBottomSheet
import androidx.compose.material3.Switch
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.material3.rememberModalBottomSheetState
import androidx.compose.runtime.Composable
import androidx.compose.runtime.DisposableEffect
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.collectAsState
import androidx.compose.runtime.getValue
import androidx.compose.runtime.remember
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.Role
import androidx.compose.ui.semantics.heading
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.unit.dp
import androidx.lifecycle.Lifecycle
import androidx.lifecycle.compose.LocalLifecycleOwner
import jp.kz2601.metamon.R
import jp.kz2601.metamon.dsl.nearby.NearbyShare
import jp.kz2601.metamon.permissions.PermissionCenter
import jp.kz2601.metamon.permissions.rememberPermissionRequest
import jp.kz2601.metamon.runtime.AppRuntime
import jp.kz2601.metamon.runtime.UiText
import jp.kz2601.metamon.runtime.resolve
import jp.kz2601.metamon.services.permissions.AppPermission
import jp.kz2601.metamon.services.permissions.PermissionStatus

/**
 * 近距離共有(#1382)の画面: 送信シート(相手選び)と受信の確認ダイアログ。iOS の `NearbySheets`。
 * 前面にいて、今のアプリが送る nearbyShare を使い、近くのデバイスの権限がある間だけ待ち受ける。
 * 権限がまだ聞かれていなければ、この起動で 1 回だけ理由 → OS のダイアログで聞く(iOS のローカルネットワークの確認に当たる)。
 */
@Composable
fun NearbyHostUi(controller: NearbyShareController, runtime: AppRuntime) {
    val context = LocalContext.current
    val activity = LocalActivity.current
    val center = remember(context) { PermissionCenter.get(context) }
    val statuses by center.statuses.collectAsState()
    val lifecycle = LocalLifecycleOwner.current.lifecycle
    val lifecycleState by lifecycle.currentStateFlow.collectAsState()
    val state by runtime.state.collectAsState()
    val active = lifecycleState.isAtLeast(Lifecycle.State.STARTED)
    val uses = state.dsl.usesNearbyShare
    val granted = statuses[AppPermission.NEARBY] == PermissionStatus.GRANTED ||
        center.status(AppPermission.NEARBY, activity) == PermissionStatus.GRANTED

    val ask = rememberPermissionRequest(AppPermission.NEARBY) { }
    LaunchedEffect(active, uses) {
        if (active && uses && !controller.askedForPermission && center.status(AppPermission.NEARBY, activity) == PermissionStatus.NOT_ASKED) {
            controller.askedForPermission = true
            ask()
        }
    }
    LaunchedEffect(active, uses, granted) { controller.updateAdvertising(active && granted) }
    DisposableEffect(controller) { onDispose { controller.updateAdvertising(false) } }

    val request by controller.request.collectAsState()
    request?.let { NearbySendSheet(controller, it.payload, onDismiss = controller::dismissRequest) }

    val incoming by controller.service.incoming.collectAsState()
    incoming?.let { IncomingDialog(controller.service, it, appName = state.dsl.app.name) }
}

@Composable
private fun IncomingDialog(service: NearbyService, item: NearbyService.Incoming, appName: String) {
    val h = item.header
    val title = UiText.Plural(R.plurals.nearby_incoming_title, h.count, listOf(NearbyService.label(h.sender), h.entityName, h.count)).resolve()
    val accept = item.decision is NearbyShare.Decision.Accept
    AlertDialog(
        onDismissRequest = { service.answer(item, accept = false) },
        title = { Text(title) },
        text = {
            Text(
                when (val d = item.decision) {
                    is NearbyShare.Decision.Accept -> stringResource(R.string.nearby_incoming_body, appName)
                    is NearbyShare.Decision.Reject -> d.reason
                },
            )
        },
        confirmButton = {
            if (accept) {
                TextButton(onClick = { service.answer(item, accept = true) }, modifier = Modifier.testTag("nearby.incoming.accept")) {
                    Text(stringResource(R.string.nearby_accept))
                }
            } else {
                TextButton(onClick = { service.answer(item, accept = false) }) { Text(stringResource(R.string.nearby_close)) }
            }
        },
        dismissButton = if (accept) {
            { TextButton(onClick = { service.answer(item, accept = false) }) { Text(stringResource(R.string.nearby_decline)) } }
        } else {
            null
        },
    )
}

/** 送信シート: 近くの Tailor It を一覧し、選んだ相手に送る。開いている間だけ探す。 */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
private fun NearbySendSheet(controller: NearbyShareController, payload: NearbyShare.Payload, onDismiss: () -> Unit) {
    val service = controller.service
    val peers by service.peers.collectAsState()
    val sendState by service.sendState.collectAsState()
    val anonymous by controller.anonymous.collectAsState()
    val myName by service.myName.collectAsState()
    DisposableEffect(service) {
        service.startBrowsing()
        onDispose { service.stopBrowsing() }
    }
    ModalBottomSheet(onDismissRequest = onDismiss, sheetState = rememberModalBottomSheetState()) {
        Column(
            Modifier.fillMaxWidth().verticalScroll(rememberScrollState()).padding(horizontal = 20.dp).navigationBarsPadding(),
            verticalArrangement = Arrangement.spacedBy(12.dp),
        ) {
            Text(
                stringResource(R.string.nearby_sheet_title),
                style = MaterialTheme.typography.titleMedium,
                modifier = Modifier.semantics { heading() },
            )
            Labeled(stringResource(R.string.nearby_what), UiText.Plural(R.plurals.nearby_what_records, payload.header.count, listOf(payload.header.entityName, payload.header.count)).resolve())
            Row(
                Modifier.fillMaxWidth().defaultMinSize(minHeight = 48.dp).testTag("nearby.send.anonymous"),
                verticalAlignment = Alignment.CenterVertically,
            ) {
                Text(stringResource(R.string.nearby_hide_name), Modifier.weight(1f), style = MaterialTheme.typography.bodyLarge)
                Switch(checked = anonymous, onCheckedChange = controller::setAnonymous, enabled = !sendState.isBusy)
            }
            Labeled(stringResource(R.string.nearby_public_name), nameText(myName), Modifier.testTag("nearby.send.publicName"))
            Text(stringResource(R.string.nearby_hide_name_footer), style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant)
            HorizontalDivider()
            Text(stringResource(R.string.nearby_devices), style = MaterialTheme.typography.titleSmall, modifier = Modifier.semantics { heading() })
            if (peers.isEmpty()) {
                Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(12.dp)) {
                    CircularProgressIndicator(Modifier.size(20.dp), strokeWidth = 2.dp)
                    Text(stringResource(R.string.nearby_searching), color = MaterialTheme.colorScheme.onSurfaceVariant)
                }
            }
            peers.forEach { peer ->
                Text(
                    nameText(peer.name),
                    style = MaterialTheme.typography.bodyLarge,
                    color = if (sendState.isBusy) MaterialTheme.colorScheme.onSurfaceVariant else MaterialTheme.colorScheme.primary,
                    modifier = Modifier
                        .fillMaxWidth()
                        .defaultMinSize(minHeight = 48.dp)
                        .clickable(enabled = !sendState.isBusy, role = Role.Button) { service.send(payload, peer) }
                        .padding(vertical = 12.dp)
                        .testTag("nearby.peer.${peer.id}"),
                )
            }
            Text(stringResource(R.string.nearby_devices_footer), style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant)
            SendStatus(sendState)
            Spacer(Modifier.height(8.dp))
        }
    }
}

@Composable
private fun SendStatus(state: NearbyService.SendState) {
    val (text, spinning) = when (state) {
        NearbyService.SendState.Idle -> return
        is NearbyService.SendState.Inviting -> UiText.Res(R.string.nearby_waiting, NearbyService.label(state.name)) to true
        is NearbyService.SendState.Sending -> UiText.Res(R.string.nearby_sending, NearbyService.label(state.name)) to true
        is NearbyService.SendState.Done -> state.message to false
        is NearbyService.SendState.Failed -> state.message to false
    }
    Row(
        Modifier.fillMaxWidth().testTag("nearby.send.status"),
        verticalAlignment = Alignment.CenterVertically,
        horizontalArrangement = Arrangement.spacedBy(12.dp),
    ) {
        if (spinning) CircularProgressIndicator(Modifier.size(20.dp), strokeWidth = 2.dp)
        Text(
            text.resolve(),
            color = when (state) {
                is NearbyService.SendState.Failed -> MaterialTheme.colorScheme.error
                is NearbyService.SendState.Done -> MaterialTheme.colorScheme.primary
                else -> MaterialTheme.colorScheme.onSurface
            },
        )
    }
}

@Composable
private fun Labeled(label: String, value: String, modifier: Modifier = Modifier) {
    Row(modifier.fillMaxWidth(), horizontalArrangement = Arrangement.spacedBy(12.dp)) {
        Text(label, Modifier.weight(1f), style = MaterialTheme.typography.bodyLarge)
        Text(value, style = MaterialTheme.typography.bodyLarge, color = MaterialTheme.colorScheme.onSurfaceVariant)
    }
}

/** 相手・自分の名前を画面の形に(匿名の印は今の言語の「近くの人」)。 */
@Composable
private fun nameText(name: String): String = when (val l = NearbyService.label(name)) {
    is UiText -> l.resolve()
    else -> l.toString()
}
