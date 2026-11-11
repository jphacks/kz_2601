package jp.kz2601.metamon.share

import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.Button
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.ModalBottomSheet
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.material3.rememberModalBottomSheetState
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.ProvidableCompositionLocal
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.setValue
import androidx.compose.runtime.staticCompositionLocalOf
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.pluralStringResource
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.unit.dp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import jp.kz2601.metamon.R
import jp.kz2601.metamon.renderer.a11y.rememberAnnouncer
import jp.kz2601.metamon.services.sharing.MemberActions
import jp.kz2601.metamon.services.sharing.MembersFailure
import jp.kz2601.metamon.services.sharing.MembersModel
import jp.kz2601.metamon.services.sharing.MembersPhase
import jp.kz2601.metamon.services.sharing.SharedMember
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.launch

/** 共有シートの下に足す欄(参加している人・#903)。共有シートは呼び出し側が渡したものを描くだけ。 */
val LocalShareSheetExtras: ProvidableCompositionLocal<@Composable () -> Unit> = staticCompositionLocalOf { {} }

/**
 * 共有シートの「参加している人」(iOS の `SharedMembersSection`・#903)。
 * 作成者は各行に「外す」、管理者は「管理者にする / 外す」、参加者は「この共有から抜ける」。外す・抜けるは確認を取る。
 * @param onRevokeLink 外した後の「リンクを取り消す」。null なら誘導の文だけ出す
 * @param onLeft 抜けられた後(呼び出し側がシートを閉じ、別のアプリへ切り替えて後片付けする)
 * @param leaveScope 抜ける通信を走らせる scope。シートを閉じても止めない(止めると抜けたのに後片付けされない)
 */
@Composable
fun MembersSection(model: MembersModel, onRevokeLink: (suspend () -> Unit)?, onLeft: () -> Unit, leaveScope: CoroutineScope? = null) {
    val state by model.state.collectAsStateWithLifecycle()
    val scope = rememberCoroutineScope()
    val announce = rememberAnnouncer()
    var confirmRemove by remember { mutableStateOf<SharedMember?>(null) }
    var confirmLeave by remember { mutableStateOf(false) }
    var failure by remember { mutableStateOf<MembersFailure?>(null) }
    LaunchedEffect(model) { model.load() }
    if (!state.isVisible) return

    Column(
        Modifier.fillMaxWidth().padding(top = 24.dp)
            .background(MaterialTheme.colorScheme.surfaceVariant, RoundedCornerShape(16.dp))
            .padding(16.dp)
            .testTag("members.section"),
        verticalArrangement = Arrangement.spacedBy(12.dp),
    ) {
        val list = state.list
        Title(count = list?.items?.size?.takeIf { state.phase == MembersPhase.LOADED })
        when (state.phase) {
            MembersPhase.LOADING -> {
                val label = stringResource(R.string.members_loading)
                Column(Modifier.semantics { contentDescription = label }, verticalArrangement = Arrangement.spacedBy(8.dp)) {
                    repeat(3) {
                        Spacer(Modifier.fillMaxWidth().height(36.dp).background(MaterialTheme.colorScheme.surface, RoundedCornerShape(6.dp)))
                    }
                }
            }
            MembersPhase.NOT_MEMBER -> Note(stringResource(R.string.share_failure_not_member))
            MembersPhase.FAILED -> {
                Note(stringResource(R.string.members_load_failed))
                OutlinedButton(onClick = { scope.launch { model.load() } }) { Text(stringResource(R.string.share_retry)) }
            }
            MembersPhase.LEFT -> Note(stringResource(R.string.members_left), Modifier.testTag("members.left"))
            MembersPhase.LOADED -> if (list != null) {
                Note(stringResource(R.string.members_footer))
                Column {
                    list.items.forEachIndexed { i, member ->
                        if (i > 0) HorizontalDivider()
                        MemberRow(
                            member,
                            canRemove = MemberActions.canRemove(list, member),
                            canSetAdmin = MemberActions.canSetAdmin(list, member),
                            onRemove = { confirmRemove = member },
                            onToggleAdmin = { scope.launch { failure = model.setAdmin(member, !member.isAdmin) } },
                        )
                    }
                }
                if (list.isFull) Note(stringResource(R.string.members_full))
                state.justRemoved?.let { RemovedNotice(it, onRevokeLink, model) }
                if (MemberActions.canLeave(list)) {
                    OutlinedButton(
                        onClick = { confirmLeave = true },
                        enabled = !state.leaving,
                        modifier = Modifier.fillMaxWidth().testTag("members.leave"),
                    ) { Text(stringResource(R.string.members_leave), color = MaterialTheme.colorScheme.error) }
                }
            }
            MembersPhase.PROBING, MembersPhase.HIDDEN -> Unit
        }
        failure?.let { f ->
            val text = stringResource(f.textRes)
            LaunchedEffect(f) { announce(text) }
            Text(text, style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.error, modifier = Modifier.testTag("members.error"))
        }
    }

    confirmRemove?.let { target ->
        ConfirmDialog(
            title = stringResource(R.string.members_remove_title, target.label()),
            message = stringResource(R.string.members_remove_message),
            confirm = stringResource(R.string.members_remove),
            tag = "members.remove.confirm",
            onDismiss = { confirmRemove = null },
        ) { scope.launch { failure = model.remove(target) } }
    }
    if (confirmLeave) {
        ConfirmDialog(
            title = stringResource(R.string.members_leave_title),
            message = stringResource(R.string.members_leave_message),
            confirm = stringResource(R.string.members_leave_confirm),
            tag = "members.leave.confirm",
            onDismiss = { confirmLeave = false },
        ) {
            (leaveScope ?: scope).launch {
                val result = model.leave()
                failure = result
                if (result == null) onLeft()
            }
        }
    }
}

/** 設定の「参加している人」から開くシート(共有シートと同じ欄だけ)。 */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun MembersSheet(model: MembersModel, onLeft: () -> Unit, onDismiss: () -> Unit, leaveScope: CoroutineScope? = null) {
    ModalBottomSheet(
        onDismissRequest = onDismiss,
        sheetState = rememberModalBottomSheetState(skipPartiallyExpanded = true),
        modifier = Modifier.testTag("members.sheet"),
    ) {
        Column(Modifier.verticalScroll(rememberScrollState()).padding(horizontal = 16.dp).padding(bottom = 24.dp)) {
            Row(verticalAlignment = Alignment.CenterVertically) {
                Spacer(Modifier.weight(1f))
                TextButton(onClick = onDismiss) { Text(stringResource(R.string.share_close)) }
            }
            MembersSection(model, onRevokeLink = null, onLeft = onLeft, leaveScope = leaveScope)
        }
    }
}

/** 設定の「参加している人」の行。共有の記録が無いアプリ・使えない構成では出さない(呼び出し側が null を渡す)。 */
@Composable
fun MembersSettingsRow(onOpen: (() -> Unit)?) {
    onOpen ?: return
    Column(Modifier.fillMaxWidth().clickable(onClick = onOpen).padding(horizontal = 16.dp, vertical = 12.dp).testTag("settings.members")) {
        Text(stringResource(R.string.members_title), style = MaterialTheme.typography.bodyLarge)
        Text(
            stringResource(R.string.members_settings_footer),
            style = MaterialTheme.typography.bodySmall,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
        )
    }
}

@Composable
private fun Title(count: Int?) {
    Row(verticalAlignment = Alignment.CenterVertically) {
        Text(stringResource(R.string.members_title), style = MaterialTheme.typography.titleMedium, modifier = Modifier.weight(1f))
        count?.let { Text(pluralStringResource(R.plurals.members_count, it, it), style = MaterialTheme.typography.bodyMedium) }
    }
}

@Composable
private fun Note(text: String, modifier: Modifier = Modifier) {
    Text(text, style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant, modifier = modifier)
}

@Composable
private fun MemberRow(member: SharedMember, canRemove: Boolean, canSetAdmin: Boolean, onRemove: () -> Unit, onToggleAdmin: () -> Unit) {
    Row(Modifier.fillMaxWidth().padding(vertical = 10.dp), verticalAlignment = Alignment.CenterVertically) {
        Column(Modifier.weight(1f)) {
            Text(member.label(), style = MaterialTheme.typography.bodyLarge, modifier = Modifier.testTag("members.row"))
            val badges = buildList {
                if (member.isOwner && member.displayName.isNotEmpty()) add(stringResource(R.string.members_creator))
                if (member.isAdmin) add(stringResource(R.string.members_admin)) // 作成者も管理者
                if (member.you) add(stringResource(R.string.members_you))
            }
            if (badges.isNotEmpty()) {
                Text(badges.joinToString("  "), style = MaterialTheme.typography.labelSmall, color = MaterialTheme.colorScheme.onSurfaceVariant)
            }
        }
        val name = member.label()
        if (canSetAdmin) {
            // 読み上げでどの人の操作か分かるように名前を添える
            val adminLabel = stringResource(if (member.isAdmin) R.string.members_remove_admin_label else R.string.members_make_admin_label, name)
            TextButton(onClick = onToggleAdmin, modifier = Modifier.semantics { contentDescription = adminLabel }.testTag("members.admin")) {
                Text(stringResource(if (member.isAdmin) R.string.members_remove_admin else R.string.members_make_admin))
            }
        }
        if (canRemove) {
            val removeLabel = stringResource(R.string.members_remove_label, name)
            TextButton(onClick = onRemove, modifier = Modifier.semantics { contentDescription = removeLabel }.testTag("members.remove")) {
                Text(stringResource(R.string.members_remove), color = MaterialTheme.colorScheme.error)
            }
        }
    }
}

@Composable
private fun RemovedNotice(removed: SharedMember, onRevokeLink: (suspend () -> Unit)?, model: MembersModel) {
    val scope = rememberCoroutineScope()
    Column(
        Modifier.fillMaxWidth().background(MaterialTheme.colorScheme.surface, RoundedCornerShape(12.dp)).padding(12.dp).testTag("members.removedNotice"),
        verticalArrangement = Arrangement.spacedBy(8.dp),
    ) {
        Text(stringResource(R.string.members_removed_notice, removed.label()), style = MaterialTheme.typography.bodySmall)
        Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
            if (onRevokeLink != null) {
                Button(onClick = {
                    scope.launch {
                        onRevokeLink()
                        model.dismissRemoved()
                    }
                }, modifier = Modifier.testTag("members.revokeLink")) { Text(stringResource(R.string.share_revoke)) }
            }
            OutlinedButton(onClick = model::dismissRemoved) { Text(stringResource(R.string.members_keep_link)) }
        }
    }
}

@Composable
private fun ConfirmDialog(title: String, message: String, confirm: String, tag: String, onDismiss: () -> Unit, onConfirm: () -> Unit) {
    AlertDialog(
        onDismissRequest = onDismiss,
        title = { Text(title) },
        text = { Text(message) },
        confirmButton = {
            TextButton(onClick = {
                onDismiss()
                onConfirm()
            }, modifier = Modifier.testTag(tag)) { Text(confirm, color = MaterialTheme.colorScheme.error) }
        },
        dismissButton = { TextButton(onClick = onDismiss) { Text(stringResource(R.string.members_cancel)) } },
    )
}

/** 一覧の名前。作成者は表示名を持たない(backend の仕様)ので「作成者」。 */
@Composable
private fun SharedMember.label(): String = when {
    displayName.isNotEmpty() -> displayName
    isOwner -> stringResource(R.string.members_creator)
    else -> stringResource(R.string.members_no_name)
}

private val MembersFailure.textRes: Int
    get() = when (this) {
        MembersFailure.REMOVE -> R.string.members_remove_failed
        MembersFailure.SET_ADMIN -> R.string.members_admin_failed
        MembersFailure.LEAVE -> R.string.members_leave_failed
        MembersFailure.OFFLINE -> R.string.members_offline
    }
