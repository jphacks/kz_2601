package jp.kz2601.metamon.renderer.components

import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.BoxWithConstraints
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.ColumnScope
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.defaultMinSize
import androidx.compose.foundation.layout.fillMaxHeight
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.lazy.LazyRow
import androidx.compose.foundation.lazy.itemsIndexed
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.semantics.Role
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.semantics.stateDescription
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.Dp
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import jp.kz2601.metamon.dsl.Component
import jp.kz2601.metamon.dsl.ComponentType
import jp.kz2601.metamon.renderer.layout.localGridPlacement
import jp.kz2601.metamon.renderer.registry.RenderContext
import jp.kz2601.metamon.renderer.screen.LocalGrid
import jp.kz2601.metamon.renderer.theme.RenderTheme
import jp.kz2601.metamon.renderer.theme.renderCard

// 7 つの Container。iOS の Components/Containers/*.swift と同じ並べ方。

/** container の `spacing`(none | small | medium | large)。 */
val Component.containerSpacing: Dp
    get() = when (spacing) {
        "none" -> 0.dp
        "small" -> 6.dp
        "large" -> 24.dp
        else -> 12.dp
    }

/** 自分で面(カード)を描く部品。section がこれだけを子に持つ時は二重カードにしない。 */
val Component.rendersOwnSurface: Boolean
    get() = type in OWN_SURFACE_TYPES

private val OWN_SURFACE_TYPES = setOf(
    ComponentType.LIST, ComponentType.TABLE, ComponentType.TIMELINE, ComponentType.CARD, ComponentType.STAT,
    ComponentType.CHART, ComponentType.CALENDAR, ComponentType.GRID_TABLE, ComponentType.RANDOM_PICKER,
    ComponentType.BRACKET, ComponentType.FLASHCARDS, ComponentType.FEED, ComponentType.ROUTE_GUIDE,
    ComponentType.PLACE_SEARCH, ComponentType.DASHCAM, ComponentType.PROGRESS_BAR, ComponentType.GAUGE,
    ComponentType.TIMER_DISPLAY, ComponentType.DISCLOSURE, ComponentType.CAROUSEL, ComponentType.MAP_VIEW,
    ComponentType.EMPTY_STATE,
)

/** 子を縦に積む。子が全員 area を持てばローカルグリッド。 */
@Composable
private fun ColumnScope.Children(c: Component, ctx: RenderContext) {
    val placed = c.localGridPlacement
    if (placed != null) {
        LocalGrid(placed, ctx)
    } else {
        c.children.forEach { DSLComponent(it, ctx) }
    }
}

/** `vstack`: 子を縦に積む。 */
@Composable
fun VStackContainer(c: Component, ctx: RenderContext, modifier: Modifier = Modifier) {
    Column(modifier.fillMaxWidth(), verticalArrangement = Arrangement.spacedBy(c.containerSpacing)) {
        Children(c, ctx)
    }
}

/** `hstack`: 子を横に等幅で並べる。 */
@Composable
fun HStackContainer(c: Component, ctx: RenderContext, modifier: Modifier = Modifier) {
    Row(modifier.fillMaxWidth(), horizontalArrangement = Arrangement.spacedBy(c.containerSpacing), verticalAlignment = Alignment.Top) {
        c.children.forEach { DSLComponent(it, ctx, Modifier.weight(1f)) }
    }
}

/** `section`: 見出し + 角丸カードの中に子を縦に積む。自前の面を持つ子だけなら見出し + そのまま。 */
@Composable
fun SectionContainer(c: Component, ctx: RenderContext, modifier: Modifier = Modifier) {
    Column(modifier.fillMaxWidth(), verticalArrangement = Arrangement.spacedBy(RenderTheme.Spacing.sm)) {
        c.title?.takeIf { it.isNotEmpty() }?.let {
            Text(
                it,
                style = MaterialTheme.typography.labelMedium.copy(fontWeight = FontWeight.SemiBold, letterSpacing = 0.3.sp),
                color = MaterialTheme.colorScheme.onSurfaceVariant,
                modifier = Modifier.padding(horizontal = RenderTheme.Spacing.xs + 2.dp),
            )
        }
        val surface = if (c.children.all { it.rendersOwnSurface }) Modifier.fillMaxWidth() else Modifier.renderCard()
        Column(surface, verticalArrangement = Arrangement.spacedBy(c.containerSpacing)) { Children(c, ctx) }
    }
}

/** `card`: 角丸背景 + 影の 1 枚カード。任意の title を上に、子を縦に積む。 */
@Composable
fun CardContainer(c: Component, ctx: RenderContext, modifier: Modifier = Modifier) {
    Column(modifier.renderCard(), verticalArrangement = Arrangement.spacedBy(c.containerSpacing)) {
        c.title?.takeIf { it.isNotEmpty() }?.let { Text(it, style = MaterialTheme.typography.titleMedium) }
        Children(c, ctx)
    }
}

/** disclosure の見出し(既定「詳細」)。描画の木と同じ値。 */
val Component.disclosureTitle: String get() = title ?: "詳細"

/** `disclosure`: 折りたたみ。title がトグル行、expanded が初期状態(既定 false)。 */
@Composable
fun DisclosureContainer(c: Component, ctx: RenderContext, modifier: Modifier = Modifier) {
    var expanded by rememberSaveable(c.id) { mutableStateOf(c.expanded ?: false) }
    Column(modifier.renderCard()) {
        Row(
            Modifier
                .fillMaxWidth()
                .clickable(role = Role.Button) { expanded = !expanded }
                .semantics { stateDescription = if (expanded) "開いています" else "閉じています" },
            verticalAlignment = Alignment.CenterVertically,
        ) {
            Text(c.disclosureTitle, style = MaterialTheme.typography.titleMedium, modifier = Modifier.weight(1f))
            Text(if (expanded) "▲" else "▼", color = MaterialTheme.colorScheme.primary)
        }
        if (expanded) {
            Column(
                Modifier.padding(top = RenderTheme.Spacing.sm),
                verticalArrangement = Arrangement.spacedBy(c.containerSpacing),
            ) {
                c.children.forEach { DSLComponent(it, ctx) }
            }
        }
    }
}

/** carousel の 1 枚の幅(small 40% / medium 70% / large 90%)。 */
val Component.carouselWidthRatio: Float
    get() = when (itemWidth) {
        "small" -> 0.4f
        "large" -> 0.9f
        else -> 0.7f
    }

/** `carousel`: 横スクロールの帯。各子は itemWidth の等幅カード。 */
@Composable
fun CarouselContainer(c: Component, ctx: RenderContext, modifier: Modifier = Modifier) {
    BoxWithConstraints(modifier.fillMaxWidth().defaultMinSize(minHeight = 96.dp)) {
        val itemWidth = maxWidth * c.carouselWidthRatio
        LazyRow(horizontalArrangement = Arrangement.spacedBy(c.containerSpacing)) {
            itemsIndexed(c.children) { _, child ->
                Column(Modifier.width(itemWidth).fillMaxHeight().renderCard()) { DSLComponent(child, ctx) }
            }
        }
    }
}

/** grid の列数(2〜4 に丸める・既定 2)。 */
val Component.gridColumns: Int get() = (columnCount ?: 2).coerceIn(2, 4)

/** `grid`: 等幅 columnCount 列のタイル。 */
@Composable
fun GridContainer(c: Component, ctx: RenderContext, modifier: Modifier = Modifier) {
    val n = c.gridColumns
    Column(modifier.fillMaxWidth(), verticalArrangement = Arrangement.spacedBy(c.containerSpacing)) {
        c.children.chunked(n).forEach { row ->
            Row(Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.spacedBy(c.containerSpacing)) {
                row.forEach { DSLComponent(it, ctx, Modifier.weight(1f)) }
                repeat(n - row.size) { Spacer(Modifier.weight(1f)) }
            }
        }
    }
}
