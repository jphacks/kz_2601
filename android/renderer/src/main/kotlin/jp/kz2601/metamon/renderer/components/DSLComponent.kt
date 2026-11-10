package jp.kz2601.metamon.renderer.components

import androidx.annotation.StringRes
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.defaultMinSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.border
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.CompositionLocalProvider
import androidx.compose.runtime.compositionLocalOf
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.semantics.testTag
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.unit.dp
import jp.kz2601.metamon.dsl.Component
import jp.kz2601.metamon.dsl.ComponentType
import jp.kz2601.metamon.renderer.R
import jp.kz2601.metamon.renderer.registry.LocalComponentRegistry
import jp.kz2601.metamon.renderer.registry.RenderContext
import jp.kz2601.metamon.renderer.theme.RenderTheme
import jp.kz2601.metamon.renderer.theme.colors

/** 入れ子の上限(iOS の `Component.maxNestingDepth`)。これより深い所は描かない。 */
const val MAX_NESTING_DEPTH = 8

private val LocalNestingDepth = compositionLocalOf { 0 }

/** Component → Composable。レジストリを type で引くだけ(iOS の `DSLView(component:)`)。 */
@Composable
fun DSLComponent(component: Component, ctx: RenderContext, modifier: Modifier = Modifier) {
    val depth = LocalNestingDepth.current
    if (depth >= MAX_NESTING_DEPTH) return
    val build = LocalComponentRegistry.current.builder(component.type)
    CompositionLocalProvider(LocalNestingDepth provides depth + 1) {
        if (build != null) build(component, ctx, modifier) else UnavailableComponent(component, modifier)
    }
}

/** 描けない理由(iOS の `UnavailableComponent.Reason`)。 */
enum class UnavailableReason(@StringRes val message: Int) {
    /** この版が知らない type(新しい版の DSL)。 */
    NEEDS_UPDATE(R.string.renderer_unavailable_needs_update),

    /** type は知っているが、このビルド・端末では描けない。 */
    UNAVAILABLE_HERE(R.string.renderer_unavailable_here),
    ;

    companion object {
        fun of(c: Component): UnavailableReason =
            if (c.type == ComponentType.UNKNOWN) NEEDS_UPDATE else UNAVAILABLE_HERE
    }
}

/** レジストリに無い部品の場所に出す控えめな枠(#536)。グリッドに穴を開けず、理由だけ短く出す。部品名は出さない。 */
@Composable
fun UnavailableComponent(component: Component, modifier: Modifier = Modifier) {
    val message = stringResource(UnavailableReason.of(component).message)
    Column(
        modifier
            .fillMaxWidth()
            .defaultMinSize(minHeight = 44.dp)
            .border(1.dp, RenderTheme.colors.hairline, RoundedCornerShape(RenderTheme.Radius.inner))
            .padding(RenderTheme.Spacing.sm)
            .semantics { contentDescription = message; testTag = "component.unavailable" },
        horizontalAlignment = Alignment.CenterHorizontally,
        verticalArrangement = Arrangement.Center,
    ) {
        Text(
            message,
            style = MaterialTheme.typography.labelSmall,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
            textAlign = TextAlign.Center,
            maxLines = 2,
        )
    }
}
