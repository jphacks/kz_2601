package jp.kz2601.metamon.share

import android.content.Intent
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.Button
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.ModalBottomSheet
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.material3.rememberModalBottomSheetState
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.unit.dp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import jp.kz2601.metamon.AppContainer
import jp.kz2601.metamon.R
import jp.kz2601.metamon.renderer.a11y.rememberAnnouncer
import jp.kz2601.metamon.runtime.AppRuntime
import jp.kz2601.metamon.runtime.UiText
import jp.kz2601.metamon.services.InstallResult
import jp.kz2601.metamon.services.sharing.PendingAdd
import jp.kz2601.metamon.services.sharing.PendingJoin
import jp.kz2601.metamon.services.sharing.ShareLinkInbox
import jp.kz2601.metamon.services.sharing.ShareLinkOpener
import jp.kz2601.metamon.services.sharing.ShareOpenFailure
import jp.kz2601.metamon.services.sharing.ShareOpenResult
import jp.kz2601.metamon.services.social.AuthorProfile
import kotlinx.coroutines.launch

/**
 * `tailorit://import/<token>` の受け口(#901)。Activity の起動・`onNewIntent` から [offer] する。
 * サインイン画面の時に届いた分もここに残り、[ShareLinkHost] が準備できてから開く(iOS の `ShareLinkReceiver`)。
 */
object ShareLinkIntents {
    val inbox = ShareLinkInbox()

    fun offer(intent: Intent?) {
        intent ?: return
        offer(intent.action, intent.dataString, intent.flags)
    }

    /** 最近使ったアプリから開き直した時は、前に開いたリンクの intent がもう一度届くので開かない。 */
    internal fun offer(action: String?, data: String?, flags: Int): Boolean {
        if (action != Intent.ACTION_VIEW || flags and Intent.FLAG_ACTIVITY_LAUNCHED_FROM_HISTORY != 0) return false
        return inbox.receive(data)
    }
}

/**
 * 受け取ったシェアリンクを開く(iOS の `SharingHooks` / `AppRuntime.importShared`)。サインイン後の本体に 1 回置く。
 * 記録を共有するアプリは参加の確認([JoinShareSheet])、それ以外は追加の確認([AddShareSheet])の後に取り込んで切り替える。
 */
@Composable
fun ShareLinkHost(container: AppContainer, canShare: Boolean) {
    val runtime = container.runtime
    val user by container.accountGuard.active.collectAsStateWithLifecycle()
    val consentShowing by container.consentPrompt.visible.collectAsStateWithLifecycle()
    val consentState by container.consent.state.collectAsStateWithLifecycle()
    val pending by ShareLinkIntents.inbox.pending.collectAsStateWithLifecycle()
    var bootstrapped by remember(runtime) { mutableStateOf(false) }
    var join by remember { mutableStateOf<PendingJoin?>(null) }
    var add by remember { mutableStateOf<PendingAdd?>(null) }
    // 開く・切り替えはシートを閉じた後も続くので、ここの scope で走らせる
    val scope = rememberCoroutineScope()
    val opener = remember(container, canShare) {
        ShareLinkOpener(container.appConfig.takeIf { canShare }, container.authorProfile)
    }

    // 起動の読み込み(MetamonApp も呼ぶ。同じ job を待つだけ)
    LaunchedEffect(runtime) {
        runtime.bootstrap()
        bootstrapped = true
    }
    val ready = ShareLinkInbox.ready(
        signedIn = user != null,
        bootstrapped = bootstrapped,
        consentShowing = consentShowing,
        // 状態の値が変わらない「あとで」(出し直しの版で後回し)もあるので、覚えずに毎回読む
        consentPending = consentState.let { container.consent.shouldPromptOnLaunch },
    )
    LaunchedEffect(ready, pending) {
        val token = ShareLinkIntents.inbox.take(ready) ?: return@LaunchedEffect
        // take で pending が変わりこの effect は作り直されるので、開く処理は外の scope で
        scope.launch {
            when (val result = opener.open(token)) {
                is ShareOpenResult.AskToJoin -> join = result.join
                is ShareOpenResult.AskToAdd -> add = result.add
                is ShareOpenResult.Added -> adopt(container, runtime, result)
                is ShareOpenResult.Failed -> runtime.showToast(UiText.Res(result.reason.textRes))
            }
        }
    }

    join?.let { current ->
        JoinShareSheet(
            join = current,
            initialName = remember(current) { opener.initialDisplayName() },
            onJoin = { name ->
                when (val result = opener.join(current, name)) {
                    is ShareOpenResult.Added -> {
                        join = null
                        scope.launch { adopt(container, runtime, result) }
                        null
                    }
                    is ShareOpenResult.Failed -> result.reason
                    is ShareOpenResult.AskToJoin, is ShareOpenResult.AskToAdd -> null
                }
            },
            onDismiss = { join = null },
        )
    }

    add?.let { current ->
        AddShareSheet(
            add = current,
            onAdd = {
                when (val result = opener.add(current)) {
                    is ShareOpenResult.Added -> {
                        add = null
                        scope.launch { adopt(container, runtime, result) }
                        null
                    }
                    is ShareOpenResult.Failed -> result.reason
                    is ShareOpenResult.AskToJoin, is ShareOpenResult.AskToAdd -> null
                }
            },
            onDismiss = { add = null },
        )
    }
}

/** 取り込んだアプリに切り替えて一言出す(iOS の `adopt`)。参加した時は他の人の行動の通知も有効にする(#339)。 */
private suspend fun adopt(container: AppContainer, runtime: AppRuntime, result: ShareOpenResult.Added) {
    val app: InstallResult = result.app
    if (!runtime.switchApp(app.id)) return // 読めなければ切り替え側がトーストを出す
    // ランタイムは Context を持たないので文言は UiText で渡す(#1232)。トーストは TalkBack にも読まれる(#1238)
    val name: Any = app.name.ifEmpty { null } ?: UiText.Res(R.string.share_untitled)
    val head = UiText.Res(if (result.viaJoin) R.string.share_open_joined else R.string.share_open_added, name)
    val extra: Any = when {
        app.member -> UiText.Res(R.string.share_open_extra_members)
        app.joined -> UiText.Res(R.string.share_open_extra_public)
        else -> ""
    }
    runtime.showToast(UiText.Res(R.string.share_open_message, head, extra))
    if (result.viaJoin) container.push?.let { push -> push.consent.change(true, push.currentUid(), app.id) {} }
    runtime.appsList()
}

internal val ShareOpenFailure.textRes: Int
    get() = when (this) {
        ShareOpenFailure.UNAVAILABLE -> R.string.share_open_unavailable
        ShareOpenFailure.REVOKED -> R.string.share_open_revoked
        ShareOpenFailure.INVITER_LEFT -> R.string.share_open_inviter_left
        ShareOpenFailure.MEMBERS_FULL -> R.string.share_open_members_full
        ShareOpenFailure.NAME_REJECTED -> R.string.share_join_name_rejected
        ShareOpenFailure.OFFLINE -> R.string.share_open_offline
        ShareOpenFailure.OTHER -> R.string.share_open_failed
    }

/**
 * 記録を共有するアプリに参加する前の確認(iOS の `JoinShareSheet`・#254): 参加すると何が見えるかと、表示名(任意)。
 * @param onJoin 参加する。失敗したら理由(シートは開いたまま)、うまくいけば null
 */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
private fun JoinShareSheet(
    join: PendingJoin,
    initialName: String,
    onJoin: suspend (String) -> ShareOpenFailure?,
    onDismiss: () -> Unit,
) {
    val scope = rememberCoroutineScope()
    val announce = rememberAnnouncer()
    var name by remember(join) { mutableStateOf(initialName) }
    var busy by remember(join) { mutableStateOf(false) }
    var failure by remember(join) { mutableStateOf<ShareOpenFailure?>(null) }
    val nameRejected = failure == ShareOpenFailure.NAME_REJECTED
    val title = join.preview.name.ifEmpty { stringResource(R.string.share_untitled) }

    ModalBottomSheet(
        onDismissRequest = { if (!busy) onDismiss() },
        sheetState = rememberModalBottomSheetState(skipPartiallyExpanded = true, confirmValueChange = { !busy }),
        modifier = Modifier.testTag("join.sheet"),
    ) {
        Column(
            Modifier.verticalScroll(rememberScrollState()).padding(horizontal = 16.dp).padding(bottom = 24.dp),
            verticalArrangement = Arrangement.spacedBy(16.dp),
        ) {
            Column(verticalArrangement = Arrangement.spacedBy(4.dp)) {
                Text(stringResource(R.string.share_join_title, title), style = MaterialTheme.typography.titleLarge, modifier = Modifier.testTag("join.title"))
                join.preview.description?.takeIf { it.isNotBlank() }?.let {
                    Text(it, style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant)
                }
            }
            Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
                Text(stringResource(R.string.share_join_points_header), style = MaterialTheme.typography.titleSmall)
                JOIN_POINTS.forEach { Text(stringResource(R.string.share_join_point, stringResource(it)), style = MaterialTheme.typography.bodyMedium) }
            }
            Column(verticalArrangement = Arrangement.spacedBy(4.dp)) {
                Text(stringResource(R.string.share_join_name_label), style = MaterialTheme.typography.titleSmall)
                OutlinedTextField(
                    value = name,
                    onValueChange = {
                        name = it.take(AuthorProfile.MAX_LENGTH)
                        if (nameRejected) failure = null // 直し始めたら警告を消す
                    },
                    singleLine = true,
                    isError = nameRejected,
                    placeholder = { Text(stringResource(R.string.share_join_name_placeholder)) },
                    modifier = Modifier.fillMaxWidth().testTag("join.name"),
                )
                Text(
                    stringResource(R.string.share_join_name_footer),
                    style = MaterialTheme.typography.bodySmall,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
            }
            failure?.let {
                val text = stringResource(it.textRes)
                LaunchedEffect(it) { announce(text) } // 参加できなかった理由を TalkBack にも読ませる(H8)
                Text(
                    text,
                    style = MaterialTheme.typography.bodySmall,
                    color = MaterialTheme.colorScheme.error,
                    modifier = Modifier.testTag("join.error"),
                )
            }
            Button(
                onClick = {
                    scope.launch {
                        busy = true
                        failure = null
                        try {
                            failure = onJoin(name)
                        } finally {
                            busy = false
                        }
                    }
                },
                enabled = !busy,
                modifier = Modifier.fillMaxWidth().height(48.dp).testTag("join.confirm"),
            ) {
                Row(verticalAlignment = Alignment.CenterVertically) {
                    if (busy) {
                        CircularProgressIndicator(Modifier.size(18.dp), strokeWidth = 2.dp)
                        Spacer(Modifier.size(8.dp))
                    }
                    Text(stringResource(R.string.share_join_confirm))
                }
            }
            TextButton(
                onClick = onDismiss,
                enabled = !busy,
                modifier = Modifier.align(Alignment.CenterHorizontally).testTag("join.cancel"),
            ) { Text(stringResource(R.string.share_join_cancel)) }
        }
    }
}

private val JOIN_POINTS = listOf(
    R.string.share_join_point_see,
    R.string.share_join_point_visible,
    R.string.share_join_point_list,
    R.string.share_join_point_notify,
    R.string.share_join_point_leave,
)
