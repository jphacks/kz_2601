package jp.kz2601.metamon.release

import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.CheckCircle
import androidx.compose.material3.Button
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.Icon
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.ModalBottomSheet
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Text
import androidx.compose.material3.rememberModalBottomSheetState
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.unit.dp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import jp.kz2601.metamon.R
import jp.kz2601.metamon.services.AppSummary
import jp.kz2601.metamon.services.release.ReleaseRequest
import jp.kz2601.metamon.services.release.ReleaseRequestLimits
import jp.kz2601.metamon.services.release.ReleaseStatus
import kotlinx.coroutines.launch

/** 状態の言葉(iOS の `ReleaseStatus.label`) */
@Composable
fun releaseStatusLabel(status: ReleaseStatus): String = stringResource(
    when (status) {
        ReleaseStatus.RECEIVED -> R.string.release_status_received
        ReleaseStatus.REVIEWING -> R.string.release_status_reviewing
        ReleaseStatus.PLANNED -> R.string.release_status_planned
        ReleaseStatus.DECLINED -> R.string.release_status_declined
    },
)

/** 変身一覧のカードの右上に出す状態の小さなバッジ(iOS の `ReleaseStatusBadge`)。 */
@Composable
fun ReleaseStatusBadge(status: ReleaseStatus, modifier: Modifier = Modifier) {
    val label = releaseStatusLabel(status)
    val a11y = stringResource(R.string.release_badge_a11y, label)
    Text(
        label,
        style = MaterialTheme.typography.labelSmall.copy(fontWeight = FontWeight.Bold),
        color = Color.White,
        modifier = modifier
            .background(Color.Black.copy(alpha = if (status == ReleaseStatus.DECLINED) 0.35f else 0.55f), CircleShape)
            .padding(horizontal = 8.dp, vertical = 4.dp)
            .semantics { contentDescription = a11y }
            .testTag("release.badge.${status.wire}"),
    )
}

/**
 * 「単体アプリとしてリリースを申請」のシート(#1254・iOS の `ReleaseRequestSheet`)。用途の一言(必須)と連絡先(任意)を送る。
 * 申請中(見送り以外)があればフォームの代わりに状況を出す。
 */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun ReleaseRequestSheet(app: AppSummary, model: ReleaseRequestsModel, onDismiss: () -> Unit) {
    val byApp by model.byApp.collectAsStateWithLifecycle()
    val scope = rememberCoroutineScope()
    var purpose by rememberSaveable { mutableStateOf("") }
    var contact by rememberSaveable { mutableStateOf("") }
    var busy by rememberSaveable { mutableStateOf(false) }
    var failed by rememberSaveable { mutableStateOf(false) }
    var justSent by rememberSaveable { mutableStateOf(false) }
    LaunchedEffect(app.id) { model.refresh() }
    val open: ReleaseRequest? = byApp[app.id]?.takeIf { it.kind != ReleaseStatus.DECLINED }
    val name = app.name.ifEmpty { stringResource(R.string.switcher_untitled) }

    ModalBottomSheet(
        onDismissRequest = onDismiss,
        sheetState = rememberModalBottomSheetState(skipPartiallyExpanded = true),
        modifier = Modifier.testTag("release.sheet"),
    ) {
        Column(
            Modifier.verticalScroll(rememberScrollState()).padding(horizontal = 24.dp).padding(bottom = 32.dp),
            verticalArrangement = Arrangement.spacedBy(16.dp),
        ) {
            Text(stringResource(R.string.release_title, name), style = MaterialTheme.typography.titleLarge.copy(fontWeight = FontWeight.Bold))
            Text(stringResource(R.string.release_intro), style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant)
            if (open != null) {
                Column(Modifier.fillMaxWidth(), horizontalAlignment = Alignment.CenterHorizontally, verticalArrangement = Arrangement.spacedBy(12.dp)) {
                    if (justSent) {
                        Icon(Icons.Filled.CheckCircle, contentDescription = null, tint = MaterialTheme.colorScheme.primary, modifier = Modifier.size(44.dp))
                    }
                    Text(
                        stringResource(if (justSent) R.string.release_received else R.string.release_requested),
                        style = MaterialTheme.typography.titleMedium.copy(fontWeight = FontWeight.Bold),
                        modifier = Modifier.testTag("release.status.title"),
                    )
                    Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                        Text(stringResource(R.string.release_status_now), style = MaterialTheme.typography.bodyMedium, color = MaterialTheme.colorScheme.onSurfaceVariant)
                        ReleaseStatusBadge(open.kind)
                    }
                    if (open.purpose.isNotEmpty()) {
                        Text(stringResource(R.string.release_purpose_quote, open.purpose), style = MaterialTheme.typography.bodySmall, textAlign = TextAlign.Center)
                    }
                    Text(
                        stringResource(R.string.release_status_updates),
                        style = MaterialTheme.typography.labelSmall,
                        color = MaterialTheme.colorScheme.onSurfaceVariant,
                        textAlign = TextAlign.Center,
                    )
                    OutlinedButton(onClick = onDismiss, modifier = Modifier.fillMaxWidth()) { Text(stringResource(R.string.release_close)) }
                }
            } else {
                Text(stringResource(R.string.release_purpose_question), style = MaterialTheme.typography.titleSmall)
                OutlinedTextField(
                    value = purpose,
                    onValueChange = { purpose = it.take(ReleaseRequestLimits.MAX_PURPOSE) },
                    placeholder = { Text(stringResource(R.string.release_purpose_placeholder)) },
                    minLines = 2,
                    maxLines = 5,
                    modifier = Modifier.fillMaxWidth().testTag("release.purpose"),
                )
                OutlinedTextField(
                    value = contact,
                    onValueChange = { contact = it.take(ReleaseRequestLimits.MAX_CONTACT) },
                    placeholder = { Text(stringResource(R.string.release_contact_placeholder)) },
                    singleLine = true,
                    modifier = Modifier.fillMaxWidth().testTag("release.contact"),
                )
                Text(stringResource(R.string.release_note), style = MaterialTheme.typography.labelSmall, color = MaterialTheme.colorScheme.onSurfaceVariant)
                if (failed) {
                    Text(stringResource(R.string.release_failed), style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.error)
                }
                Button(
                    onClick = {
                        busy = true
                        failed = false
                        scope.launch {
                            val r = model.send(app.id, purpose, contact)
                            busy = false
                            if (r == null) failed = true else justSent = true
                        }
                    },
                    enabled = purpose.isNotBlank() && !busy,
                    modifier = Modifier.fillMaxWidth().heightIn(min = 44.dp).testTag("release.send"),
                ) {
                    if (busy) CircularProgressIndicator(Modifier.size(18.dp), strokeWidth = 2.dp) else Text(stringResource(R.string.release_send))
                }
            }
        }
    }
}
