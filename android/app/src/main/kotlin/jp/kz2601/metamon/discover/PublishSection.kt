package jp.kz2601.metamon.discover

import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.Button
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.unit.dp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import jp.kz2601.metamon.R
import jp.kz2601.metamon.renderer.a11y.TalkBack
import jp.kz2601.metamon.renderer.a11y.rememberAnnouncer
import jp.kz2601.metamon.renderer.a11y.rememberTalkBackRunning
import jp.kz2601.metamon.services.discover.PublishFailure
import jp.kz2601.metamon.services.discover.PublishModel
import jp.kz2601.metamon.services.discover.PublishNotice
import jp.kz2601.metamon.services.discover.PublishState
import jp.kz2601.metamon.services.social.AuthorProfile
import kotlinx.coroutines.delay
import kotlinx.coroutines.launch

/**
 * 共有シートの下半分「みんなに公開」(iOS の `ShareAppSheet.publishSection`・#902 / #1239 の残り)。
 * 公開する前に確認(誰でも入れられる・個人の情報を書かない・表示名)を挟む。[LocalPublishEnv] が無ければ何も出さない。
 */
@Composable
fun PublishSection(appId: String, appName: String) {
    val env = LocalPublishEnv.current ?: return
    val model = remember(appId, env) { PublishModel(appId, env.client, env.profile) }
    val state by model.state.collectAsStateWithLifecycle()
    val scope = rememberCoroutineScope()
    val announce = rememberAnnouncer()
    val talkBack = rememberTalkBackRunning()
    var confirmUnpublish by rememberSaveable { mutableStateOf(false) }
    LaunchedEffect(model) { model.load() }

    HorizontalDivider(Modifier.padding(vertical = 16.dp))
    Column(Modifier.fillMaxWidth().testTag("publish.section")) {
        Text(stringResource(R.string.publish_header), style = MaterialTheme.typography.titleMedium)
        Spacer(Modifier.height(8.dp))
        val listing = state.listing
        when {
            !state.loaded -> Box(Modifier.fillMaxWidth().height(44.dp), contentAlignment = Alignment.Center) {
                val label = stringResource(R.string.discover_loading)
                CircularProgressIndicator(Modifier.size(24.dp).semantics { contentDescription = label })
            }
            listing != null -> {
                Text(
                    stringResource(R.string.publish_live, listing.installs),
                    style = MaterialTheme.typography.bodyMedium,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                    modifier = Modifier.testTag("publish.live"),
                )
                if (listing.hidden == true) {
                    Text(
                        listing.hiddenReason?.let { stringResource(R.string.discover_hidden_reason, it) } ?: stringResource(R.string.discover_hidden),
                        style = MaterialTheme.typography.labelMedium,
                        color = MaterialTheme.colorScheme.error,
                        modifier = Modifier.padding(top = 4.dp),
                    )
                }
                OutlinedButton(
                    onClick = { confirmUnpublish = true },
                    enabled = !state.busy,
                    modifier = Modifier.fillMaxWidth().padding(top = 8.dp).testTag("publish.unpublish"),
                ) { Text(stringResource(R.string.publish_unpublish), color = MaterialTheme.colorScheme.error) }
            }
            else -> PublishForm(state, model, onPublish = model::requestPublish)
        }
        state.notice?.let { notice ->
            val text = noticeText(notice, state)
            Text(
                text,
                style = MaterialTheme.typography.bodySmall,
                color = if (notice.isFailure) MaterialTheme.colorScheme.error else MaterialTheme.colorScheme.onSurfaceVariant,
                modifier = Modifier.fillMaxWidth().padding(top = 8.dp).testTag("publish.notice"),
            )
            LaunchedEffect(notice, text) {
                announce(text)
                delay(TalkBack.holdMillis(NOTICE_MILLIS, talkBack))
                model.consumeNotice()
            }
        }
    }

    state.confirm?.let { confirm ->
        PublishConfirmDialog(
            appName = appName.ifEmpty { stringResource(R.string.share_untitled) },
            initialName = confirm.displayName,
            onPublish = { name -> scope.launch { model.confirmPublish(name) } },
            onDismiss = model::cancelPublish,
        )
    }
    if (confirmUnpublish) {
        AlertDialog(
            onDismissRequest = { confirmUnpublish = false },
            title = { Text(stringResource(R.string.publish_unpublish_title)) },
            text = { Text(stringResource(R.string.publish_unpublish_message)) },
            confirmButton = {
                TextButton(onClick = {
                    confirmUnpublish = false
                    scope.launch { model.unpublish() }
                }, modifier = Modifier.testTag("publish.unpublish.confirm")) {
                    Text(stringResource(R.string.publish_unpublish), color = MaterialTheme.colorScheme.error)
                }
            },
            dismissButton = { TextButton(onClick = { confirmUnpublish = false }) { Text(stringResource(R.string.publish_cancel)) } },
        )
    }
}

@Composable
private fun PublishForm(state: PublishState, model: PublishModel, onPublish: () -> Unit) {
    Text(
        stringResource(R.string.publish_intro),
        style = MaterialTheme.typography.bodySmall,
        color = MaterialTheme.colorScheme.onSurfaceVariant,
    )
    // 説明は複数行、タグは 1 行(iOS #196)
    OutlinedTextField(
        value = state.description,
        onValueChange = model::setDescription,
        label = { Text(stringResource(R.string.publish_description_label)) },
        isError = state.flaggedField == "description",
        minLines = 2,
        maxLines = 5,
        modifier = Modifier.fillMaxWidth().padding(top = 8.dp).testTag("publish.description"),
    )
    OutlinedTextField(
        value = state.tags,
        onValueChange = model::setTags,
        label = { Text(stringResource(R.string.publish_tags_label)) },
        placeholder = { Text(stringResource(R.string.publish_tags_placeholder)) },
        isError = state.flaggedField == "tags",
        singleLine = true,
        modifier = Modifier.fillMaxWidth().padding(top = 8.dp).testTag("publish.tags"),
    )
    Button(
        onClick = onPublish,
        enabled = !state.busy,
        modifier = Modifier.fillMaxWidth().padding(top = 12.dp).height(44.dp).testTag("publish.publish"),
    ) { Text(stringResource(R.string.publish_action)) }
}

/** 公開してよいかの確認と、「探す」に出す名前。 */
@Composable
private fun PublishConfirmDialog(appName: String, initialName: String, onPublish: (String) -> Unit, onDismiss: () -> Unit) {
    var name by rememberSaveable { mutableStateOf(initialName) }
    AlertDialog(
        onDismissRequest = onDismiss,
        modifier = Modifier.testTag("publish.confirm"),
        title = { Text(stringResource(R.string.publish_confirm_title, appName)) },
        text = {
            Column(Modifier.verticalScroll(rememberScrollState())) {
                Text(stringResource(R.string.publish_confirm_points_header), style = MaterialTheme.typography.titleSmall)
                listOf(
                    R.string.publish_confirm_point_anyone,
                    R.string.publish_confirm_point_records,
                    R.string.publish_confirm_point_privacy,
                    R.string.publish_confirm_point_stop,
                ).forEach { res ->
                    Text(stringResource(R.string.share_join_point, stringResource(res)), style = MaterialTheme.typography.bodyMedium)
                }
                OutlinedTextField(
                    value = name,
                    onValueChange = { name = it.take(AuthorProfile.MAX_LENGTH) },
                    label = { Text(stringResource(R.string.publish_confirm_name_label)) },
                    placeholder = { Text(stringResource(R.string.publish_confirm_name_placeholder)) },
                    singleLine = true,
                    modifier = Modifier.fillMaxWidth().padding(top = 12.dp).testTag("publish.confirm.name"),
                )
                Text(
                    stringResource(R.string.publish_confirm_name_footer),
                    style = MaterialTheme.typography.bodySmall,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                    modifier = Modifier.padding(top = 4.dp),
                )
            }
        },
        confirmButton = {
            TextButton(onClick = { onPublish(name) }, modifier = Modifier.testTag("publish.confirm.publish")) {
                Text(stringResource(R.string.publish_action))
            }
        },
        dismissButton = { TextButton(onClick = onDismiss) { Text(stringResource(R.string.publish_cancel)) } },
    )
}

private val PublishNotice.isFailure: Boolean
    get() = this == PublishNotice.PUBLISH_FAILED || this == PublishNotice.UNPUBLISH_FAILED

@Composable
private fun noticeText(notice: PublishNotice, state: PublishState): String = when (notice) {
    PublishNotice.PUBLISHED -> stringResource(R.string.publish_published)
    PublishNotice.UNPUBLISHED -> stringResource(R.string.publish_unpublished)
    PublishNotice.UNPUBLISH_FAILED -> stringResource(R.string.publish_unpublish_failed)
    PublishNotice.PUBLISH_FAILED -> when (state.failure) {
        PublishFailure.TEXT_NOT_ALLOWED -> fieldName(state.flaggedField)?.let { stringResource(R.string.publish_text_not_allowed, it) }
            ?: stringResource(R.string.publish_text_not_allowed_generic)
        PublishFailure.NOT_OWNER -> stringResource(R.string.publish_failed_not_owner)
        PublishFailure.BLANK_APP -> stringResource(R.string.publish_failed_blank)
        PublishFailure.OFFLINE -> stringResource(R.string.publish_failed_offline)
        PublishFailure.UNAVAILABLE -> stringResource(R.string.discover_failure_unavailable)
        PublishFailure.OTHER, null -> stringResource(R.string.publish_failed)
    }
}

/** 表示できない言葉があった欄の名前(iOS の `ShareAppSheet.textNotAllowedMessage`)。 */
@Composable
private fun fieldName(field: String?): String? = when (field) {
    "description" -> stringResource(R.string.publish_field_description)
    "tags" -> stringResource(R.string.publish_field_tags)
    "name" -> stringResource(R.string.publish_field_name)
    "authorName" -> stringResource(R.string.publish_field_author)
    else -> null
}

private const val NOTICE_MILLIS = 4_000L
