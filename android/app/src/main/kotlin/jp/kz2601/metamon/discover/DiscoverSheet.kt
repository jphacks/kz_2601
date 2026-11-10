package jp.kz2601.metamon.discover

import androidx.compose.foundation.background
import androidx.compose.foundation.horizontalScroll
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.MoreVert
import androidx.compose.material.icons.filled.Search
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.Button
import androidx.compose.material3.Card
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.DropdownMenu
import androidx.compose.material3.DropdownMenuItem
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.FilterChip
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.ModalBottomSheet
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.SegmentedButton
import androidx.compose.material3.SegmentedButtonDefaults
import androidx.compose.material3.SingleChoiceSegmentedButtonRow
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
import androidx.compose.ui.semantics.clearAndSetSemantics
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import jp.kz2601.metamon.R
import jp.kz2601.metamon.renderer.a11y.rememberAnnouncer
import jp.kz2601.metamon.services.discover.DiscoverFailure
import jp.kz2601.metamon.services.discover.DiscoverModel
import jp.kz2601.metamon.services.discover.DiscoverModerationResult
import jp.kz2601.metamon.services.discover.DiscoverSort
import jp.kz2601.metamon.services.discover.PublicApp
import jp.kz2601.metamon.services.social.ReportReason
import kotlinx.coroutines.delay
import kotlinx.coroutines.launch
import java.text.BreakIterator
import jp.kz2601.metamon.renderer.R as RendererR

/**
 * 「探す」(iOS の `DiscoverSheet`・#902): 検索・並び順・カテゴリ(タグ)・インストール・通報・掲載者のブロック。
 * インストールしたら [onInstall] に渡す(シートを閉じてそのアプリに切り替えるのは呼び出し側)。
 */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun DiscoverSheet(model: DiscoverModel, onInstall: (PublicApp) -> Unit, onDismiss: () -> Unit) {
    val state by model.state.collectAsStateWithLifecycle()
    val scope = rememberCoroutineScope()
    val announce = rememberAnnouncer()
    var reporting by remember { mutableStateOf<PublicApp?>(null) }
    var blocking by remember { mutableStateOf<PublicApp?>(null) }
    var result by remember { mutableStateOf<Pair<DiscoverModerationResult, String>?>(null) }

    // 入力中の連打を間引く(iOS と同じ 250ms)
    LaunchedEffect(state.query, state.sort) {
        if (state.query.isNotEmpty()) delay(SEARCH_DEBOUNCE_MILLIS)
        model.search()
    }

    ModalBottomSheet(
        onDismissRequest = onDismiss,
        sheetState = rememberModalBottomSheetState(skipPartiallyExpanded = true),
        modifier = Modifier.testTag("discover.sheet"),
    ) {
        Column(Modifier.padding(horizontal = 16.dp)) {
            Row(verticalAlignment = Alignment.CenterVertically) {
                Text(stringResource(R.string.discover_title), style = MaterialTheme.typography.titleLarge, modifier = Modifier.weight(1f))
                TextButton(onClick = onDismiss, modifier = Modifier.testTag("discover.close")) { Text(stringResource(R.string.discover_close)) }
            }
            OutlinedTextField(
                value = state.query,
                onValueChange = model::setQuery,
                placeholder = { Text(stringResource(R.string.discover_search_placeholder)) },
                leadingIcon = { Icon(Icons.Filled.Search, contentDescription = null) },
                singleLine = true,
                modifier = Modifier.fillMaxWidth().padding(top = 8.dp).testTag("discover.search"),
            )
            SortRow(state.sort, model::setSort)
            if (state.categories.isNotEmpty()) {
                CategoryRow(state.categories, state.category, model::toggleCategory)
            }
        }
        val items = state.visibleItems
        val failure = state.failure
        when {
            state.loading && items.isEmpty() -> Box(Modifier.fillMaxWidth().padding(vertical = 48.dp), contentAlignment = Alignment.Center) {
                val label = stringResource(R.string.discover_loading)
                CircularProgressIndicator(Modifier.semantics { contentDescription = label })
            }
            failure != null && items.isEmpty() -> Column(
                Modifier.fillMaxWidth().padding(vertical = 32.dp, horizontal = 16.dp),
                horizontalAlignment = Alignment.CenterHorizontally,
            ) {
                val text = stringResource(failure.loadTextRes)
                LaunchedEffect(failure) { announce(text) }
                Text(text, textAlign = TextAlign.Center, style = MaterialTheme.typography.bodyMedium, modifier = Modifier.testTag("discover.error"))
                if (failure != DiscoverFailure.UNAVAILABLE) {
                    OutlinedButton(onClick = { scope.launch { model.search() } }, modifier = Modifier.padding(top = 8.dp)) {
                        Text(stringResource(R.string.discover_retry))
                    }
                }
            }
            items.isEmpty() -> Column(Modifier.fillMaxWidth().padding(vertical = 32.dp), horizontalAlignment = Alignment.CenterHorizontally) {
                Icon(Icons.Filled.Search, contentDescription = null, tint = MaterialTheme.colorScheme.primary)
                Text(
                    if (state.query.isBlank()) stringResource(R.string.discover_empty) else stringResource(R.string.discover_no_match, state.query.trim()),
                    style = MaterialTheme.typography.bodyMedium,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                    modifier = Modifier.padding(top = 8.dp).testTag("discover.empty"),
                )
            }
            else -> LazyColumn(
                Modifier.fillMaxWidth().testTag("discover.list"),
                verticalArrangement = Arrangement.spacedBy(12.dp),
                contentPadding = androidx.compose.foundation.layout.PaddingValues(16.dp),
            ) {
                items(items, key = { it.id }) { item ->
                    PublicAppCard(
                        item = item,
                        installing = state.installing == item.id,
                        enabled = state.installing == null,
                        canModerate = model.canModerate && !item.mine,
                        onInstall = { onInstall(item) },
                        onReport = { reporting = item },
                        onBlock = { blocking = item },
                    )
                }
            }
        }
    }

    reporting?.let { item ->
        ReportReasonDialog(
            onPick = { reason ->
                reporting = null
                scope.launch { result = model.report(item, reason) to item.authorName }
            },
            onDismiss = { reporting = null },
        )
    }
    blocking?.let { item ->
        val name = item.authorName.ifEmpty { stringResource(R.string.discover_anonymous) }
        AlertDialog(
            onDismissRequest = { blocking = null },
            title = { Text(stringResource(RendererR.string.social_block_title, name)) },
            text = { Text(stringResource(RendererR.string.social_block_message)) },
            confirmButton = {
                TextButton(onClick = {
                    blocking = null
                    scope.launch { result = model.blockPublisher(item) to name }
                }, modifier = Modifier.testTag("discover.block.confirm")) {
                    Text(stringResource(RendererR.string.social_block_confirm), color = MaterialTheme.colorScheme.error)
                }
            },
            dismissButton = { TextButton(onClick = { blocking = null }) { Text(stringResource(R.string.publish_cancel)) } },
        )
    }
    result?.let { (kind, name) ->
        val (title, message) = moderationText(kind, name)
        LaunchedEffect(kind, name) { announce(title) }
        AlertDialog(
            onDismissRequest = { result = null },
            title = { Text(title) },
            text = message?.let { { Text(it) } },
            confirmButton = { TextButton(onClick = { result = null }) { Text(stringResource(R.string.discover_close)) } },
        )
    }
}

@OptIn(ExperimentalMaterial3Api::class)
@Composable
private fun SortRow(sort: DiscoverSort, onSort: (DiscoverSort) -> Unit) {
    val options = listOf(DiscoverSort.POPULAR to R.string.discover_sort_popular, DiscoverSort.NEW to R.string.discover_sort_new)
    val label = stringResource(R.string.discover_sort)
    SingleChoiceSegmentedButtonRow(Modifier.fillMaxWidth().padding(top = 8.dp).semantics { contentDescription = label }) {
        options.forEachIndexed { i, (value, res) ->
            SegmentedButton(
                selected = sort == value,
                onClick = { onSort(value) },
                shape = SegmentedButtonDefaults.itemShape(i, options.size),
                modifier = Modifier.testTag("discover.sort.${value.wire}"),
            ) { Text(stringResource(res)) }
        }
    }
}

/** カテゴリ(結果のタグ)。選ぶとそのタグの掲載だけにする。 */
@Composable
private fun CategoryRow(categories: List<String>, selected: String?, onToggle: (String) -> Unit) {
    val label = stringResource(R.string.discover_categories)
    Row(
        Modifier.fillMaxWidth().horizontalScroll(rememberScrollState()).padding(top = 8.dp).semantics { contentDescription = label },
        horizontalArrangement = Arrangement.spacedBy(8.dp),
    ) {
        FilterChip(
            selected = selected == null,
            onClick = { selected?.let(onToggle) },
            label = { Text(stringResource(R.string.discover_category_all)) },
        )
        categories.forEach { tag ->
            FilterChip(
                selected = selected.equals(tag, ignoreCase = true),
                onClick = { onToggle(tag) },
                label = { Text("#$tag") },
                modifier = Modifier.testTag("discover.category.$tag"),
            )
        }
    }
}

/** 検索結果のカード: 名前・作者・利用者数・説明・部品・タグ・入れるボタン(iOS の `PublicAppCard`)。 */
@Composable
private fun PublicAppCard(
    item: PublicApp,
    installing: Boolean,
    enabled: Boolean,
    canModerate: Boolean,
    onInstall: () -> Unit,
    onReport: () -> Unit,
    onBlock: () -> Unit,
) {
    var menu by remember { mutableStateOf(false) }
    Card(Modifier.fillMaxWidth().testTag("discover.card.${item.id}")) {
        Column(Modifier.padding(12.dp)) {
            Row(verticalAlignment = Alignment.Top) {
                // 頭文字は飾り(読み上げは名前の行で足りる)
                Box(
                    Modifier.size(48.dp).background(MaterialTheme.colorScheme.primary, RoundedCornerShape(12.dp)).clearAndSetSemantics {},
                    contentAlignment = Alignment.Center,
                ) {
                    Text(
                        initialOf(item.name),
                        color = MaterialTheme.colorScheme.onPrimary,
                        style = MaterialTheme.typography.titleLarge,
                        fontWeight = FontWeight.Bold,
                    )
                }
                Column(Modifier.weight(1f).padding(horizontal = 12.dp)) {
                    Text(item.name, style = MaterialTheme.typography.titleMedium, maxLines = 1, overflow = TextOverflow.Ellipsis)
                    val author = item.authorName.ifEmpty { stringResource(R.string.discover_anonymous) }
                    Text(
                        stringResource(if (item.mine) R.string.discover_byline_mine else R.string.discover_byline, author, item.installs),
                        style = MaterialTheme.typography.bodySmall,
                        color = MaterialTheme.colorScheme.onSurfaceVariant,
                        maxLines = 1,
                        overflow = TextOverflow.Ellipsis,
                    )
                }
                val installLabel = stringResource(R.string.discover_install_label, item.name)
                Button(
                    onClick = onInstall,
                    enabled = enabled,
                    modifier = Modifier.semantics { contentDescription = installLabel }.testTag("discover.install.${item.id}"),
                ) {
                    if (installing) {
                        CircularProgressIndicator(Modifier.size(16.dp), strokeWidth = 2.dp, color = MaterialTheme.colorScheme.onPrimary)
                    } else {
                        Text(stringResource(if (item.mine) R.string.discover_copy else R.string.discover_get))
                    }
                }
                if (canModerate) {
                    Box {
                        IconButton(onClick = { menu = true }, modifier = Modifier.testTag("discover.more.${item.id}")) {
                            Icon(Icons.Filled.MoreVert, contentDescription = stringResource(R.string.discover_more, item.name))
                        }
                        DropdownMenu(expanded = menu, onDismissRequest = { menu = false }) {
                            DropdownMenuItem(text = { Text(stringResource(R.string.discover_report)) }, onClick = {
                                menu = false
                                onReport()
                            })
                            DropdownMenuItem(
                                text = { Text(stringResource(R.string.discover_block), color = MaterialTheme.colorScheme.error) },
                                onClick = {
                                    menu = false
                                    onBlock()
                                },
                            )
                        }
                    }
                }
            }
            if (item.hidden == true) {
                Text(
                    item.hiddenReason?.let { stringResource(R.string.discover_hidden_reason, it) } ?: stringResource(R.string.discover_hidden),
                    style = MaterialTheme.typography.labelMedium,
                    color = MaterialTheme.colorScheme.error,
                    modifier = Modifier.padding(top = 8.dp),
                )
            }
            if (item.description.isNotEmpty()) {
                Text(
                    item.description,
                    style = MaterialTheme.typography.bodyMedium,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                    maxLines = 2,
                    overflow = TextOverflow.Ellipsis,
                    modifier = Modifier.padding(top = 8.dp),
                )
            }
            val tags = item.tags.take(2)
            if (tags.isNotEmpty()) {
                Spacer(Modifier.size(4.dp))
                Text(
                    tags.joinToString(" ") { "#" + it.removePrefix("#") },
                    style = MaterialTheme.typography.labelSmall,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
            }
        }
    }
}

/** 通報の理由を選ぶ(並びは iOS と同じ)。 */
@Composable
private fun ReportReasonDialog(onPick: (ReportReason) -> Unit, onDismiss: () -> Unit) {
    AlertDialog(
        onDismissRequest = onDismiss,
        title = { Text(stringResource(RendererR.string.social_report_title)) },
        text = {
            Column {
                Text(stringResource(RendererR.string.social_report_message), style = MaterialTheme.typography.bodyMedium)
                ReportReason.entries.forEach { reason ->
                    TextButton(onClick = { onPick(reason) }, modifier = Modifier.fillMaxWidth().testTag("discover.report.${reason.wire}")) {
                        Text(stringResource(reason.labelRes), modifier = Modifier.fillMaxWidth())
                    }
                }
            }
        },
        confirmButton = {},
        dismissButton = { TextButton(onClick = onDismiss) { Text(stringResource(R.string.publish_cancel)) } },
    )
}

private val ReportReason.labelRes: Int
    get() = when (this) {
        ReportReason.SPAM -> RendererR.string.social_reason_spam
        ReportReason.SEXUAL -> RendererR.string.social_reason_sexual
        ReportReason.VIOLENCE -> RendererR.string.social_reason_violence
        ReportReason.HARASSMENT -> RendererR.string.social_reason_harassment
        ReportReason.ILLEGAL -> RendererR.string.social_reason_illegal
        ReportReason.PRIVACY -> RendererR.string.social_reason_privacy
        ReportReason.OTHER -> RendererR.string.social_reason_other
    }

/** 通報・ブロックの結果(見出しと本文)。 */
@Composable
private fun moderationText(kind: DiscoverModerationResult, name: String): Pair<String, String?> = when (kind) {
    DiscoverModerationResult.REPORTED ->
        stringResource(RendererR.string.social_reported_title) to stringResource(RendererR.string.social_reported_message)
    DiscoverModerationResult.REPORT_LIMITED ->
        stringResource(RendererR.string.social_report_failed_title) to stringResource(RendererR.string.social_report_limited)
    DiscoverModerationResult.REPORT_FAILED -> stringResource(RendererR.string.social_report_failed_title) to null
    DiscoverModerationResult.BLOCKED ->
        stringResource(RendererR.string.social_blocked_title) to stringResource(RendererR.string.social_blocked_message, name)
    DiscoverModerationResult.BLOCK_FAILED -> stringResource(RendererR.string.social_block_failed_title) to null
}

private val DiscoverFailure.loadTextRes: Int
    get() = when (this) {
        DiscoverFailure.UNAVAILABLE -> R.string.discover_failure_unavailable
        DiscoverFailure.OFFLINE -> R.string.discover_failure_offline
        else -> R.string.discover_failure_load
    }

/** インストールの失敗の一言。 */
val DiscoverFailure.installTextRes: Int
    get() = when (this) {
        DiscoverFailure.UNAVAILABLE -> R.string.discover_failure_unavailable
        DiscoverFailure.UNPUBLISHED, DiscoverFailure.NOT_FOUND -> R.string.discover_install_unpublished
        DiscoverFailure.OFFLINE -> R.string.discover_install_offline
        DiscoverFailure.OTHER -> R.string.discover_install_failed
    }

/** 名前の頭の 1 文字(絵文字・結合文字を途中で切らない。iOS の `prefix(1)` と同じ)。 */
internal fun initialOf(name: String): String {
    val text = name.trim()
    if (text.isEmpty()) return ""
    val it = BreakIterator.getCharacterInstance().apply { setText(text) }
    val end = it.next()
    return if (end == BreakIterator.DONE) text else text.substring(0, end)
}

private const val SEARCH_DEBOUNCE_MILLIS = 250L
