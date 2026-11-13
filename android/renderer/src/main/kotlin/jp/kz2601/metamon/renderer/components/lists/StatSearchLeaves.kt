package jp.kz2601.metamon.renderer.components.lists

import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.defaultMinSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.text.BasicText
import androidx.compose.foundation.text.BasicTextField
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.foundation.text.TextAutoSize
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Clear
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.alpha
import androidx.compose.ui.graphics.SolidColor
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.clearAndSetSemantics
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.stateDescription
import androidx.compose.ui.text.input.ImeAction
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import jp.kz2601.metamon.dsl.Component
import jp.kz2601.metamon.renderer.R
import jp.kz2601.metamon.renderer.components.UnavailableComponent
import jp.kz2601.metamon.renderer.components.inputs.inputChrome
import jp.kz2601.metamon.renderer.registry.RenderContext
import jp.kz2601.metamon.renderer.theme.RenderTheme
import jp.kz2601.metamon.renderer.theme.SymbolIcon
import jp.kz2601.metamon.renderer.theme.colors
import jp.kz2601.metamon.renderer.theme.renderCard

/**
 * `stat`: entity の集計値(count|sum|avg|min|max|latest|earliest)、または `value`(式)の結果を 1 枚のカードに。iOS の `StatLeaf`。
 * 数値は大きく等幅で、セルに収まるまで縮める(省略記号は出さない)。マイナスは負の色。読み上げは「ラベル, 値」の 1 要素。
 */
@Composable
fun StatLeaf(c: Component, ctx: RenderContext, modifier: Modifier = Modifier) {
    val data = rememberLeafData(ctx.runtime)
    val v = StatValue.resolve(c, data)
    val colors = RenderTheme.colors
    val label = c.label.orEmpty()
    val iconName = StatIcon.name(c, c.field?.let { data.entity(c.entity)?.field(it) })
    val unavailable = stringResource(R.string.lists_unavailable)
    val loading = stringResource(R.string.load_state_loading)
    Column(
        modifier
            .renderCard()
            .clearAndSetSemantics {
                contentDescription = label
                stateDescription = when {
                    v.loading -> loading
                    v.unavailable -> unavailable
                    else -> v.text
                }
            },
        verticalArrangement = Arrangement.spacedBy(RenderTheme.Spacing.xs),
    ) {
        Row(Modifier.defaultMinSize(minHeight = 18.dp), horizontalArrangement = Arrangement.spacedBy(6.dp), verticalAlignment = Alignment.CenterVertically) {
            iconName?.let {
                Box(Modifier.size(18.dp).background(colors.brandSoft, RoundedCornerShape(6.dp)), contentAlignment = Alignment.Center) {
                    SymbolIcon(it, 12.dp, colors.brandA)
                }
            }
            Text(label, style = RenderTheme.Fonts.statLabel, color = MaterialTheme.colorScheme.onSurfaceVariant, maxLines = 1, overflow = TextOverflow.Ellipsis)
        }
        BasicText(
            v.text,
            // 初回の読み込み中は 0 を見せず「—」を明滅(#1116)
            if (v.loading) Modifier.alpha(skeletonPulse()) else Modifier,
            style = RenderTheme.Fonts.statValue.copy(
                // 予算の残りが赤字など、マイナスは負の色(#131)
                color = when {
                    v.loading || v.unavailable -> colors.tertiaryLabel
                    v.text.startsWith("-") -> colors.negative
                    else -> MaterialTheme.colorScheme.onSurface
                },
            ),
            maxLines = 1,
            // iOS の minimumScaleFactor 相当。セルに収まるまで段階的に縮める(省略記号は出さない)
            autoSize = TextAutoSize.StepBased(minFontSize = 11.sp, maxFontSize = RenderTheme.Fonts.statValue.fontSize, stepSize = 2.sp),
        )
    }
}

/**
 * `searchBar`: 画面ローカルの検索文字列(`field` キー)。同じ画面の `list.searchField` がこの値で絞り込む。iOS の `SearchBarLeaf`。
 * 記録には保存しない([ScreenLocals])。
 */
@Composable
fun SearchBarLeaf(c: Component, ctx: RenderContext, modifier: Modifier = Modifier) {
    val locals = LocalScreenLocals.current
    val key = c.field
    if (locals == null || key == null) {
        UnavailableComponent(c, modifier)
        return
    }
    val text = locals.values[key].orEmpty()
    val placeholder = c.placeholder ?: stringResource(R.string.lists_search)
    Row(
        modifier.fillMaxWidth().inputChrome(),
        verticalAlignment = Alignment.CenterVertically,
        horizontalArrangement = Arrangement.spacedBy(RenderTheme.Spacing.sm + 2.dp),
    ) {
        SymbolIcon("magnifyingglass", 20.dp, RenderTheme.colors.secondaryLabel)
        Box(Modifier.weight(1f)) {
            if (text.isEmpty()) Text(placeholder, style = RenderTheme.Fonts.inputValue, color = RenderTheme.colors.tertiaryLabel, maxLines = 1)
            BasicTextField(
                value = text,
                onValueChange = { locals.values[key] = it },
                modifier = Modifier.fillMaxWidth(),
                singleLine = true,
                textStyle = RenderTheme.Fonts.inputValue.copy(color = MaterialTheme.colorScheme.onSurface),
                cursorBrush = SolidColor(RenderTheme.colors.brandA),
                keyboardOptions = KeyboardOptions(imeAction = ImeAction.Search, autoCorrectEnabled = false),
            )
        }
        if (text.isNotEmpty()) {
            IconButton(onClick = { locals.values[key] = "" }, modifier = Modifier.size(32.dp)) {
                Icon(Icons.Filled.Clear, contentDescription = stringResource(R.string.lists_clear), tint = RenderTheme.colors.tertiaryLabel)
            }
        }
    }
}
