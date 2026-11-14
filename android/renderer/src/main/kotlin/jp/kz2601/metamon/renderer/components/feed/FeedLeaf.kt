package jp.kz2601.metamon.renderer.components.feed

import androidx.compose.foundation.Canvas
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.KeyboardArrowDown
import androidx.compose.material.icons.filled.Refresh
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.Icon
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableIntStateOf
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.geometry.Offset
import androidx.compose.ui.graphics.Path
import androidx.compose.ui.graphics.StrokeCap
import androidx.compose.ui.graphics.StrokeJoin
import androidx.compose.ui.graphics.drawscope.Stroke
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.Role
import androidx.compose.ui.semantics.clearAndSetSemantics
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.heading
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.semantics.testTag
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import jp.kz2601.metamon.dsl.Component
import jp.kz2601.metamon.dsl.feed.FeedItem
import jp.kz2601.metamon.dsl.feed.FeedQuery
import jp.kz2601.metamon.dsl.feed.FeedResult
import jp.kz2601.metamon.renderer.R
import jp.kz2601.metamon.renderer.components.UnavailableComponent
import jp.kz2601.metamon.renderer.components.actions.openExternal
import jp.kz2601.metamon.renderer.components.lists.EmptyPlaceholder
import jp.kz2601.metamon.renderer.registry.RenderContext
import jp.kz2601.metamon.renderer.theme.RenderTheme
import jp.kz2601.metamon.renderer.theme.SymbolIcon
import jp.kz2601.metamon.renderer.theme.colors
import jp.kz2601.metamon.renderer.theme.renderCard
import kotlinx.coroutines.CancellationException

/** 部品の表示の状態(iOS の `FeedLeaf` の `result` / `failed`)。 */
data class FeedViewState(val result: FeedResult? = null, val failed: Boolean = false) {
    enum class Phase { SHOWN, LOADING, EMPTY, UNAVAILABLE }

    val phase: Phase
        get() = when {
            result != null && result.items.isNotEmpty() -> Phase.SHOWN
            failed -> Phase.UNAVAILABLE
            result == null -> Phase.LOADING
            else -> Phase.EMPTY
        }
}

/** 読み込みの手順(Compose に依らない・JVM で試す)。 */
object FeedLoading {
    /** 取りに行く前に、覚えている行を先に出す。 */
    suspend fun initial(host: FeedHost?, query: FeedQuery): FeedViewState =
        FeedViewState(result = runCatching { host?.cachedFeed(query) }.getOrNull())

    /** 取れたら差し替え。取れなければ出している行は残し、何も無い時だけ「読み込めませんでした」(iOS と同じ)。 */
    suspend fun load(host: FeedHost?, query: FeedQuery, force: Boolean, current: FeedViewState): FeedViewState {
        if (host == null) return current.copy(failed = true)
        return try {
            FeedViewState(result = host.feed(query, force), failed = false)
        } catch (e: CancellationException) {
            throw e
        } catch (_: Exception) {
            current.copy(failed = current.result == null)
        }
    }
}

/**
 * `feed`(#348・#876): 天気予報・警報・気圧の予報・祝日・許可したお知らせ・arXiv の新着・国会会議録の発言を読むだけの一覧(iOS の `FeedLeaf`)。
 * 行: 日付・見出し・(天気)アイコン / 最高・最低 / 降水確率、(気圧)小さな折れ線・最低〜最高 hPa・低下の印、
 * (お知らせ・論文・発言)タップでページをアプリ内のブラウザで開く。下に出典を必ず出す。
 */
@Composable
fun FeedLeaf(c: Component, ctx: RenderContext, modifier: Modifier = Modifier) {
    val query = remember(c) { FeedQuery.of(c) }
    if (query == null) {
        UnavailableComponent(c, modifier)
        return
    }
    val host = ctx.runtime.feeds ?: LocalFeedHost.current
    var state by remember(query) { mutableStateOf(FeedViewState()) }
    var reload by remember(query) { mutableIntStateOf(0) }
    LaunchedEffect(query, host, reload) {
        if (reload == 0 && state.result == null) state = FeedLoading.initial(host, query)
        state = FeedLoading.load(host, query, force = reload > 0, current = state)
    }

    val result = state.result
    val heading = c.title?.takeIf { it.isNotEmpty() } ?: result?.title.orEmpty()
    Column(
        modifier.fillMaxWidth().renderCard().semantics { testTag = "feed.${c.id ?: query.provider}" },
        verticalArrangement = Arrangement.spacedBy(RenderTheme.Spacing.sm),
    ) {
        if (heading.isNotEmpty()) {
            Text(heading, style = RenderTheme.Fonts.cardTitle, color = RenderTheme.colors.label, modifier = Modifier.semantics { heading() })
        }
        when (state.phase) {
            FeedViewState.Phase.SHOWN -> {
                val r = requireNotNull(result)
                Column {
                    r.items.forEachIndexed { index, item ->
                        if (index > 0) HorizontalDivider(color = RenderTheme.colors.hairline)
                        FeedRow(item, r.provider)
                    }
                }
                if (r.stale) StaleNote()
            }
            FeedViewState.Phase.EMPTY -> EmptyPlaceholder(FeedFormat.icon(query.provider), c.emptyText ?: stringResource(R.string.feed_empty))
            FeedViewState.Phase.LOADING -> LoadingPlaceholder()
            FeedViewState.Phase.UNAVAILABLE -> UnavailablePlaceholder(FeedFormat.icon(query.provider)) {
                state = state.copy(failed = false)
                reload++
            }
        }
        if (result != null) {
            val attribution = stringResource(R.string.feed_attribution, result.source.name, result.source.license)
            val spoken = stringResource(R.string.feed_attribution_spoken, result.source.name)
            Text(
                attribution,
                style = MaterialTheme.typography.labelSmall,
                color = RenderTheme.colors.secondaryLabel,
                modifier = Modifier.semantics { contentDescription = spoken },
            )
        }
    }
}

@Composable
private fun FeedRow(item: FeedItem, provider: String) {
    val context = LocalContext.current
    val link = item.link
    val tap = if (link != null) Modifier.clickable(role = Role.Button) { openExternal(context, listOf(link)) } else Modifier
    Row(
        Modifier.fillMaxWidth().then(tap).padding(vertical = RenderTheme.Spacing.sm).semantics(mergeDescendants = true) {},
        verticalAlignment = Alignment.CenterVertically,
        horizontalArrangement = Arrangement.spacedBy(RenderTheme.Spacing.row),
    ) {
        item.icon?.let { SymbolIcon(it, 24.dp, RenderTheme.colors.brandA, Modifier.width(28.dp)) }
        Column(Modifier.weight(1f), verticalArrangement = Arrangement.spacedBy(2.dp)) {
            FeedFormat.dateText(item.date, provider)?.let {
                Text(it, style = MaterialTheme.typography.labelMedium, color = RenderTheme.colors.secondaryLabel)
            }
            Text(item.title, style = RenderTheme.Fonts.rowTitle, color = RenderTheme.colors.label, maxLines = 3)
            val sub = item.subtitle
            if (!sub.isNullOrEmpty() && sub != item.title) {
                Text(sub, style = RenderTheme.Fonts.rowSubtitle, color = RenderTheme.colors.secondaryLabel, maxLines = 2)
            }
        }
        if (provider == "pressure") {
            item.series?.takeIf { it.size > 1 }?.let { PressureSparkline(it) }
            FeedFormat.pressureRange(item)?.let { Text(it, style = MaterialTheme.typography.labelMedium) }
            if (item.drop == true) DropBadge()
        } else {
            FeedFormat.temps(item)?.let { Text(it, style = RenderTheme.Fonts.rowTrailing, color = RenderTheme.colors.label) }
        }
        item.pop?.let { pop ->
            val spoken = stringResource(R.string.feed_pop_spoken, pop)
            Row(
                verticalAlignment = Alignment.CenterVertically,
                modifier = Modifier.clearAndSetSemantics { contentDescription = spoken },
            ) {
                SymbolIcon("cloud.rain", 14.dp, RenderTheme.colors.secondaryLabel)
                Text("$pop%", style = MaterialTheme.typography.labelMedium, color = RenderTheme.colors.secondaryLabel)
            }
        }
        if (link != null) SymbolIcon("arrow.up.right.square", 18.dp, RenderTheme.colors.secondaryLabel)
    }
}

@Composable
private fun DropBadge() {
    val spoken = stringResource(R.string.feed_pressure_drop_spoken)
    Row(verticalAlignment = Alignment.CenterVertically, modifier = Modifier.clearAndSetSemantics { contentDescription = spoken }) {
        Icon(Icons.Filled.KeyboardArrowDown, contentDescription = null, tint = RenderTheme.colors.warning, modifier = Modifier.size(16.dp))
        Text(
            stringResource(R.string.feed_pressure_drop),
            style = MaterialTheme.typography.labelMedium.copy(fontWeight = FontWeight.Bold),
            color = RenderTheme.colors.warning,
        )
    }
}

/** 気圧の行の小さな折れ線(iOS の `PressureSparkline`)。読み上げない(隣の文字が持つ)。 */
@Composable
private fun PressureSparkline(values: List<Double>) {
    val color = RenderTheme.colors.secondaryLabel
    Canvas(Modifier.size(width = 44.dp, height = 18.dp).clearAndSetSemantics {}) {
        val points = FeedFormat.sparkline(values)
        if (points.isEmpty()) return@Canvas
        val path = Path()
        points.forEachIndexed { i, (x, y) ->
            val p = Offset(x * size.width, y * size.height)
            if (i == 0) path.moveTo(p.x, p.y) else path.lineTo(p.x, p.y)
        }
        drawPath(path, color, style = Stroke(width = 1.5.dp.toPx(), cap = StrokeCap.Round, join = StrokeJoin.Round))
    }
}

@Composable
private fun StaleNote() {
    Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(4.dp)) {
        Icon(Icons.Filled.Refresh, contentDescription = null, tint = RenderTheme.colors.secondaryLabel, modifier = Modifier.size(14.dp))
        Text(stringResource(R.string.feed_stale), style = MaterialTheme.typography.labelSmall, color = RenderTheme.colors.secondaryLabel)
    }
}

@Composable
private fun LoadingPlaceholder() {
    Column(
        Modifier.fillMaxWidth().padding(vertical = RenderTheme.Spacing.lg),
        horizontalAlignment = Alignment.CenterHorizontally,
        verticalArrangement = Arrangement.spacedBy(RenderTheme.Spacing.sm),
    ) {
        CircularProgressIndicator(Modifier.size(24.dp), strokeWidth = 2.dp)
        Text(stringResource(R.string.feed_loading), style = MaterialTheme.typography.bodyMedium, color = MaterialTheme.colorScheme.onSurfaceVariant)
    }
}

@Composable
private fun UnavailablePlaceholder(icon: String, onRetry: () -> Unit) {
    Column(
        Modifier.fillMaxWidth().padding(vertical = RenderTheme.Spacing.md),
        horizontalAlignment = Alignment.CenterHorizontally,
        verticalArrangement = Arrangement.spacedBy(RenderTheme.Spacing.sm),
    ) {
        SymbolIcon(icon, 32.dp, RenderTheme.colors.tertiaryLabel)
        Text(stringResource(R.string.feed_unavailable), style = MaterialTheme.typography.bodyMedium, color = MaterialTheme.colorScheme.onSurfaceVariant)
        TextButton(onClick = onRetry) { Text(stringResource(R.string.feed_retry)) }
    }
}
