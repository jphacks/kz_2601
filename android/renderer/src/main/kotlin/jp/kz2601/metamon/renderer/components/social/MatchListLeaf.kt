package jp.kz2601.metamon.renderer.components.social

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.Button
import androidx.compose.material3.DropdownMenu
import androidx.compose.material3.DropdownMenuItem
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.collectAsState
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.hapticfeedback.HapticFeedbackType
import androidx.compose.ui.platform.LocalHapticFeedback
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.CustomAccessibilityAction
import androidx.compose.ui.semantics.heading
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import jp.kz2601.metamon.dsl.Component
import jp.kz2601.metamon.dsl.records.MatchBox
import jp.kz2601.metamon.dsl.records.MatchInterest
import jp.kz2601.metamon.dsl.records.Record
import jp.kz2601.metamon.renderer.ActionContext
import jp.kz2601.metamon.renderer.R
import jp.kz2601.metamon.renderer.components.lists.EmptyPlaceholder
import jp.kz2601.metamon.renderer.components.lists.ListRow
import jp.kz2601.metamon.renderer.components.lists.rememberLeafData
import jp.kz2601.metamon.renderer.registry.RenderContext
import jp.kz2601.metamon.renderer.theme.RenderTheme
import jp.kz2601.metamon.renderer.theme.colors
import jp.kz2601.metamon.renderer.theme.renderCard
import kotlinx.coroutines.launch

/**
 * `matchList`(iOS の MatchListLeaf・#507): 届いた「つながりたい」(承認 / 見送る)・送った「つながりたい」(取り下げる)・
 * つながった人(長押しで解除)。行は相手のプロフィールのレコード(`profileRecordId`)を [ListRow] で描く。
 * どの行も長押しで通報・ブロック([ModerationHost])。操作はホスト([MatchingHost])が楽観的に反映し、失敗したら戻して理由を出す。
 */
@Composable
fun MatchListLeaf(c: Component, ctx: RenderContext, modifier: Modifier = Modifier) {
    val rt = ctx.runtime
    val box = MatchBox.of(c.show)
    val entity = c.entity?.let(rt::entityModel)
    if (box == null || entity == null || !entity.isMatchProfile) {
        Text(stringResource(R.string.renderer_unavailable_here), modifier, style = MaterialTheme.typography.labelSmall)
        return
    }
    val host = rt as? MatchingHost
    // 一覧・送信中が変わったら描き直す
    host?.matchingVersion?.collectAsState()?.value
    val data = rememberLeafData(rt)
    val scope = rememberCoroutineScope()
    val haptics = LocalHapticFeedback.current
    var unmatching by remember { mutableStateOf<MatchInterest?>(null) }

    LaunchedEffect(entity.id, box) {
        rt.loadRecords(setOf(entity.id))
        host?.refreshMatches(setOf(box))
    }

    fun act(body: suspend MatchingHost.() -> Unit) {
        val h = host ?: return
        haptics.performHapticFeedback(HapticFeedbackType.TextHandleMove)
        scope.launch { h.body() }
    }

    Column(modifier.fillMaxWidth(), verticalArrangement = Arrangement.spacedBy(RenderTheme.Spacing.sm)) {
        c.title?.takeIf { it.isNotEmpty() }?.let {
            Text(it, style = MaterialTheme.typography.titleMedium.copy(fontWeight = FontWeight.SemiBold), modifier = Modifier.semantics { heading() })
        }
        val items = host?.matchItems(box)
        val rows = MatchListRows.rows(items, data.records(entity.id))
        if (rows.isEmpty()) {
            when (MatchListRows.emptyState(host != null, items, host?.matchLoadFailed(box) ?: false)) {
                MatchListRows.EmptyState.LOADING -> EmptyPlaceholder(MatchListRows.icon(box), stringResource(R.string.social_match_loading))
                MatchListRows.EmptyState.FAILED -> Column(horizontalAlignment = Alignment.CenterHorizontally, modifier = Modifier.fillMaxWidth()) {
                    EmptyPlaceholder("exclamationmark.triangle", stringResource(R.string.social_match_failed))
                    TextButton(onClick = { act { refreshMatches(setOf(box)) } }) { Text(stringResource(R.string.social_match_retry)) }
                }
                MatchListRows.EmptyState.EMPTY -> EmptyPlaceholder(MatchListRows.icon(box), c.emptyText?.takeIf { it.isNotEmpty() } ?: emptyText(box))
            }
            return@Column
        }
        Column(Modifier.renderCard(padding = 0.dp).testTag("matchList.${box.wire}")) {
            rows.forEachIndexed { index, row ->
                if (index > 0) HorizontalDivider(Modifier.padding(start = RenderTheme.Spacing.md), color = RenderTheme.colors.hairline)
                MatchRow(
                    row = row,
                    box = box,
                    c = c,
                    ctx = ctx,
                    entity = entity.id,
                    hasHost = host != null,
                    onAccept = { act { acceptInterest(row.item) } },
                    onDecline = { act { declineInterest(row.item, box) } },
                    onUnmatch = { unmatching = row.item },
                )
            }
        }
    }

    unmatching?.let { item ->
        AlertDialog(
            onDismissRequest = { unmatching = null },
            title = { Text(stringResource(R.string.social_unmatch_title)) },
            text = { Text(stringResource(R.string.social_unmatch_message)) },
            confirmButton = {
                TextButton(onClick = {
                    unmatching = null
                    act { unmatch(item) }
                }, modifier = Modifier.testTag("matchList.unmatch.confirm")) {
                    Text(stringResource(R.string.social_unmatch_confirm), color = MaterialTheme.colorScheme.error)
                }
            },
            dismissButton = { TextButton(onClick = { unmatching = null }) { Text(stringResource(R.string.social_cancel)) } },
        )
    }
}

@Composable
private fun MatchRow(
    row: MatchListRows.Row,
    box: MatchBox,
    c: Component,
    ctx: RenderContext,
    entity: String,
    hasHost: Boolean,
    onAccept: () -> Unit,
    onDecline: () -> Unit,
    onUnmatch: () -> Unit,
) {
    var menu by remember { mutableStateOf(false) }
    val profile = row.profile
    val moderation = profile?.let { p -> (ctx.runtime as? ModerationHost)?.takeIf { it.canReport(entity, p) } }
    val hasMenu = (box == MatchBox.MATCHES && hasHost) || moderation != null
    val labels = MatchListRows.actions(box).associateWith { actionLabel(it) }
    val perform: (MatchListRows.RowAction) -> Unit = {
        when (it) {
            MatchListRows.RowAction.ACCEPT -> onAccept()
            MatchListRows.RowAction.DECLINE, MatchListRows.RowAction.WITHDRAW -> onDecline()
            MatchListRows.RowAction.UNMATCH -> onUnmatch()
        }
    }
    val accessory: (@Composable () -> Unit)? = if (hasHost) {
        { RowButtons(box, labels, perform) }
    } else {
        null
    }
    val a11y = if (hasHost) labels.map { (action, label) -> CustomAccessibilityAction(label) { perform(action); true } } else emptyList()
    Box {
        if (profile != null) {
            val data = rememberLeafData(ctx.runtime)
            ListRow(
                item = c.item,
                record = profile,
                entity = ctx.runtime.entityModel(entity),
                data = data,
                showsChevron = c.onItemTap != null,
                onTap = c.onItemTap?.let { a -> { ctx.dispatch(a, ActionContext(ctx.route, ctx.form, recordId = profile.id)) } },
                accessory = accessory,
                accessoryActions = a11y,
                onLongPress = if (hasMenu) ({ menu = true }) else null,
            )
        } else {
            // プロフィールが消された・まだ読めていない人。操作だけ出す
            Row(
                Modifier.fillMaxWidth().padding(horizontal = RenderTheme.Spacing.md, vertical = RenderTheme.Spacing.row),
                verticalAlignment = Alignment.CenterVertically,
            ) {
                Text(
                    stringResource(R.string.social_match_missing_profile),
                    style = RenderTheme.Fonts.rowSubtitle,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                    modifier = Modifier.weight(1f),
                )
                accessory?.invoke()
            }
        }
        DropdownMenu(expanded = menu, onDismissRequest = { menu = false }) {
            if (box == MatchBox.MATCHES && hasHost) {
                DropdownMenuItem(
                    text = { Text(stringResource(R.string.social_unmatch), color = MaterialTheme.colorScheme.error) },
                    onClick = {
                        menu = false
                        onUnmatch()
                    },
                )
            }
            if (moderation != null && profile != null) {
                DropdownMenuItem(text = { Text(stringResource(R.string.social_report)) }, onClick = {
                    menu = false
                    moderation.reportRecord(entity, profile)
                })
                DropdownMenuItem(text = { Text(stringResource(R.string.social_block), color = MaterialTheme.colorScheme.error) }, onClick = {
                    menu = false
                    moderation.blockAuthor(entity, profile)
                })
            }
        }
    }
}

/** 行の右端: incoming = 見送る・承認 / outgoing = 取り下げる / matches = なし(長押しで解除)。 */
@Composable
private fun RowButtons(box: MatchBox, labels: Map<MatchListRows.RowAction, String>, perform: (MatchListRows.RowAction) -> Unit) {
    val pad = PaddingValues(horizontal = 12.dp, vertical = 0.dp)
    val small = Modifier.heightIn(min = 32.dp)
    Row(horizontalArrangement = Arrangement.spacedBy(RenderTheme.Spacing.xs)) {
        when (box) {
            MatchBox.INCOMING -> {
                OutlinedButton(onClick = { perform(MatchListRows.RowAction.DECLINE) }, contentPadding = pad, modifier = small.testTag("matchList.decline")) {
                    Text(labels.getValue(MatchListRows.RowAction.DECLINE), maxLines = 1)
                }
                Button(onClick = { perform(MatchListRows.RowAction.ACCEPT) }, contentPadding = pad, modifier = small.testTag("matchList.accept")) {
                    Text(labels.getValue(MatchListRows.RowAction.ACCEPT), maxLines = 1)
                }
            }
            MatchBox.OUTGOING -> OutlinedButton(
                onClick = { perform(MatchListRows.RowAction.WITHDRAW) },
                contentPadding = pad,
                modifier = small.testTag("matchList.withdraw"),
            ) { Text(labels.getValue(MatchListRows.RowAction.WITHDRAW), maxLines = 1) }
            MatchBox.MATCHES -> Unit
        }
    }
}

@Composable
private fun actionLabel(action: MatchListRows.RowAction): String = stringResource(
    when (action) {
        MatchListRows.RowAction.ACCEPT -> R.string.social_match_accept
        MatchListRows.RowAction.DECLINE -> R.string.social_match_decline
        MatchListRows.RowAction.WITHDRAW -> R.string.social_match_withdraw
        MatchListRows.RowAction.UNMATCH -> R.string.social_unmatch
    },
)

@Composable
private fun emptyText(box: MatchBox): String = stringResource(
    when (box) {
        MatchBox.INCOMING -> R.string.social_match_empty_incoming
        MatchBox.OUTGOING -> R.string.social_match_empty_outgoing
        MatchBox.MATCHES -> R.string.social_match_empty_matches
    },
)

/** 行の並び・空の形・行の操作(描画から切り離して JVM で試す)。 */
object MatchListRows {
    data class Row(val item: MatchInterest, val profile: Record?)

    enum class EmptyState { EMPTY, LOADING, FAILED }

    enum class RowAction { ACCEPT, DECLINE, WITHDRAW, UNMATCH }

    /** サーバーの並び(新しい順)のまま、相手のプロフィールのレコードを引く。同じ人は 1 行。 */
    fun rows(items: List<MatchInterest>?, profiles: List<Record>): List<Row> {
        val byId = profiles.associateBy { it.id }
        return items.orEmpty().distinctBy { it.uid }.map { Row(it, it.profileRecordId?.let(byId::get)) }
    }

    /** ホストが無い(モック・プレビュー)= 誰もいない。読んだ = 空。読めなかった = 失敗。まだ = 読み込み中。 */
    fun emptyState(hasHost: Boolean, items: List<MatchInterest>?, failed: Boolean): EmptyState = when {
        !hasHost || items != null -> EmptyState.EMPTY
        failed -> EmptyState.FAILED
        else -> EmptyState.LOADING
    }

    /** 行の操作(右端のボタン・読み上げ)。並びは読み上げの順。 */
    fun actions(box: MatchBox): List<RowAction> = when (box) {
        MatchBox.INCOMING -> listOf(RowAction.ACCEPT, RowAction.DECLINE)
        MatchBox.OUTGOING -> listOf(RowAction.WITHDRAW)
        MatchBox.MATCHES -> listOf(RowAction.UNMATCH)
    }

    fun icon(box: MatchBox): String = when (box) {
        MatchBox.INCOMING -> "tray"
        MatchBox.OUTGOING -> "paperplane"
        MatchBox.MATCHES -> "person.2"
    }
}
