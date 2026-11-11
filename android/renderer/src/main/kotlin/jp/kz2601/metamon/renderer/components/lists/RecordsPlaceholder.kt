package jp.kz2601.metamon.renderer.components.lists

import androidx.compose.animation.core.RepeatMode
import androidx.compose.animation.core.animateFloat
import androidx.compose.animation.core.infiniteRepeatable
import androidx.compose.animation.core.rememberInfiniteTransition
import androidx.compose.animation.core.tween
import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.alpha
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.LiveRegionMode
import androidx.compose.ui.semantics.clearAndSetSemantics
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.liveRegion
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import jp.kz2601.metamon.renderer.DSLRuntime
import jp.kz2601.metamon.renderer.EntityLoadState
import jp.kz2601.metamon.renderer.R
import jp.kz2601.metamon.renderer.theme.LocalReduceMotion
import jp.kz2601.metamon.renderer.theme.RenderTheme
import jp.kz2601.metamon.renderer.theme.SymbolIcon
import jp.kz2601.metamon.renderer.theme.colors
import kotlinx.coroutines.launch

/** 記録が 0 件の時に出す形(iOS の `EmptyState`・#1116)。 */
enum class RecordsPlaceholder {
    /** 読み込み中: 骨組み(空の案内とは見分けられる形) */
    SKELETON,

    /** 取得失敗: 「読み込めませんでした」と再試行 */
    UNAVAILABLE,

    /** 本当に空: 部品の空の案内 */
    EMPTY,
    ;

    companion object {
        fun of(state: EntityLoadState): RecordsPlaceholder = when (state) {
            EntityLoadState.LOADING -> SKELETON
            EntityLoadState.UNAVAILABLE -> UNAVAILABLE
            EntityLoadState.LOADED -> EMPTY
        }
    }
}

/**
 * list / table / timeline / chart の 0 件の表示。[kind] で骨組み・取得失敗・空の案内を出し分ける。
 * 読み上げ: 骨組みは「読み込み中…」の 1 要素、取得失敗は文言を polite で読み、再試行ボタンを続ける。
 */
@Composable
internal fun RecordsPlaceholderView(
    kind: RecordsPlaceholder,
    icon: String,
    emptyText: String,
    runtime: DSLRuntime,
    entity: String?,
    modifier: Modifier = Modifier,
) {
    when (kind) {
        RecordsPlaceholder.SKELETON -> RecordsSkeleton(modifier)
        RecordsPlaceholder.EMPTY -> EmptyPlaceholder(icon, emptyText, modifier)
        RecordsPlaceholder.UNAVAILABLE -> {
            val scope = rememberCoroutineScope()
            val text = stringResource(R.string.load_state_unavailable)
            Column(
                modifier.fillMaxWidth().testTag("records.unavailable"),
                horizontalAlignment = Alignment.CenterHorizontally,
            ) {
                EmptyPlaceholder(
                    "exclamationmark.triangle",
                    text,
                    Modifier.semantics { liveRegion = LiveRegionMode.Polite },
                )
                if (!entity.isNullOrEmpty()) {
                    TextButton(onClick = { scope.launch { runtime.retryRecords(setOf(entity)) } }) {
                        Text(stringResource(R.string.load_state_retry))
                    }
                }
            }
        }
    }
}

/** 読み込み中の骨組み(行の形の薄い帯が 3 本・ゆっくり明滅)。読み上げは「読み込み中…」だけ。 */
@Composable
internal fun RecordsSkeleton(modifier: Modifier = Modifier) {
    val loading = stringResource(R.string.load_state_loading)
    val pulse = skeletonPulse()
    Column(
        modifier
            .fillMaxWidth()
            .padding(vertical = RenderTheme.Spacing.md, horizontal = RenderTheme.Spacing.md)
            .testTag("records.skeleton")
            .clearAndSetSemantics {
                contentDescription = loading
                liveRegion = LiveRegionMode.Polite
            },
        verticalArrangement = Arrangement.spacedBy(RenderTheme.Spacing.md),
    ) {
        SKELETON_WIDTHS.forEach { width ->
            Column(Modifier.alpha(pulse), verticalArrangement = Arrangement.spacedBy(6.dp)) {
                SkeletonBar(width)
                SkeletonBar(width * 0.55f, height = 10)
            }
        }
    }
}

/** 骨組みの明滅の透明度(0.45〜1)。動きを減らす設定では明滅させない。 */
@Composable
internal fun skeletonPulse(): Float {
    if (LocalReduceMotion.current) return SKELETON_STILL_ALPHA
    val pulse by rememberInfiniteTransition(label = "skeleton").animateFloat(
        initialValue = 0.45f,
        targetValue = 1f,
        animationSpec = infiniteRepeatable(tween(durationMillis = 900), RepeatMode.Reverse),
        label = "skeletonAlpha",
    )
    return pulse
}

private const val SKELETON_STILL_ALPHA = 0.7f

/**
 * calendar / gridTable の 1 行の読み込み状態(iOS の caption の文言と再試行)。
 * 骨組みは細い帯 1 本(読み上げ「読み込み中…」)、取得失敗は「読み込めませんでした」+ 再試行、空は [empty]。
 */
@Composable
internal fun InlineRecordsState(
    kind: RecordsPlaceholder,
    runtime: DSLRuntime,
    entity: String?,
    modifier: Modifier = Modifier,
    empty: @Composable () -> Unit = {},
) {
    when (kind) {
        RecordsPlaceholder.EMPTY -> empty()
        RecordsPlaceholder.SKELETON -> {
            val loading = stringResource(R.string.load_state_loading)
            Box(
                modifier
                    .fillMaxWidth()
                    .padding(vertical = 4.dp)
                    .testTag("records.skeleton")
                    .clearAndSetSemantics {
                        contentDescription = loading
                        liveRegion = LiveRegionMode.Polite
                    }
                    .alpha(skeletonPulse()),
            ) { SkeletonBar(0.5f, height = 12) }
        }
        RecordsPlaceholder.UNAVAILABLE -> {
            val scope = rememberCoroutineScope()
            Row(
                modifier.fillMaxWidth().testTag("records.unavailable"),
                verticalAlignment = Alignment.CenterVertically,
                horizontalArrangement = Arrangement.spacedBy(RenderTheme.Spacing.sm),
            ) {
                Text(
                    stringResource(R.string.load_state_unavailable),
                    Modifier.semantics { liveRegion = LiveRegionMode.Polite },
                    style = MaterialTheme.typography.labelMedium,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
                if (!entity.isNullOrEmpty()) {
                    TextButton(onClick = { scope.launch { runtime.retryRecords(setOf(entity)) } }) {
                        Text(stringResource(R.string.load_state_retry), style = MaterialTheme.typography.labelMedium.copy(fontWeight = FontWeight.SemiBold))
                    }
                }
            }
        }
    }
}

@Composable
private fun SkeletonBar(fraction: Float, height: Int = 14) {
    Box(
        Modifier
            .fillMaxWidth(fraction)
            .height(height.dp)
            .background(RenderTheme.colors.tertiaryLabel.copy(alpha = 0.22f), RoundedCornerShape(6.dp)),
    )
}

private val SKELETON_WIDTHS = listOf(0.85f, 0.7f, 0.78f)

/** 部品の中の 0 件の表示を、その部品の entity の状態で。 */
@Composable
internal fun RecordsPlaceholderView(
    data: LeafData,
    entity: String?,
    icon: String,
    emptyText: String,
    modifier: Modifier = Modifier,
) = RecordsPlaceholderView(data.placeholder(entity), icon, emptyText, data.runtime, entity, modifier)
