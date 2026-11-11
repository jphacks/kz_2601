package jp.kz2601.metamon.renderer.components.actions

import androidx.compose.foundation.BorderStroke
import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.clickable
import androidx.compose.foundation.interaction.MutableInteractionSource
import androidx.compose.foundation.interaction.collectIsPressedAsState
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.defaultMinSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.remember
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.alpha
import androidx.compose.ui.draw.scale
import androidx.compose.ui.draw.shadow
import androidx.compose.ui.graphics.Brush
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.semantics.Role
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import jp.kz2601.metamon.dsl.ComponentStyle
import jp.kz2601.metamon.renderer.theme.LocalSymbolIcons
import jp.kz2601.metamon.renderer.theme.RenderTheme
import jp.kz2601.metamon.renderer.theme.SymbolIcon
import jp.kz2601.metamon.renderer.theme.colors

/** ボタンの見た目(iOS の `RenderButtonStyle.Kind`)。 */
enum class ActionButtonKind {
    PRIMARY, SECONDARY, DESTRUCTIVE, PLAIN;

    companion object {
        /** DSL の style → 見た目。省略・未知は [default]。 */
        fun of(style: ComponentStyle?, default: ActionButtonKind = PRIMARY): ActionButtonKind = when (style) {
            ComponentStyle.PRIMARY -> PRIMARY
            ComponentStyle.SECONDARY -> SECONDARY
            ComponentStyle.DESTRUCTIVE -> DESTRUCTIVE
            ComponentStyle.PLAIN -> PLAIN
            else -> default
        }
    }
}

/**
 * iOS の `RenderButtonStyle` の写し: 高さ 52(plain は 44)・角丸 14・primary はブランドのグラデーション + 影、
 * secondary はブランドの薄塗り + 枠、destructive は赤の薄塗り + 枠。押している間は少し縮む。
 * 読み上げは「ボタン」+ ラベル(アイコンは読まない)。
 */
@Composable
fun ActionButton(
    label: String,
    kind: ActionButtonKind,
    onClick: () -> Unit,
    modifier: Modifier = Modifier,
    icon: String? = null,
    enabled: Boolean = true,
    running: Boolean = false,
    fillWidth: Boolean = true,
) {
    val colors = RenderTheme.colors
    val shape = RoundedCornerShape(RenderTheme.Radius.button)
    val interaction = remember { MutableInteractionSource() }
    val pressed by interaction.collectIsPressedAsState()
    val foreground = when (kind) {
        ActionButtonKind.PRIMARY -> Color.White
        ActionButtonKind.SECONDARY, ActionButtonKind.PLAIN -> colors.brandText
        ActionButtonKind.DESTRUCTIVE -> colors.negative
    }
    var m = modifier
    if (fillWidth) m = m.fillMaxWidth()
    m = m.defaultMinSize(minHeight = if (kind == ActionButtonKind.PLAIN) 44.dp else 52.dp)
        .scale(if (pressed && enabled) RenderTheme.Motion.PRESS_SCALE else 1f)
    if (kind == ActionButtonKind.PRIMARY && enabled) m = m.shadow(RenderTheme.Shadow.brandElevation, shape, clip = false)
    m = when (kind) {
        ActionButtonKind.PRIMARY -> m.background(Brush.horizontalGradient(listOf(colors.brandFillStart, colors.brandFillEnd)), shape)
            .border(BorderStroke(1.dp, Color.White.copy(alpha = 0.18f)), shape)
        ActionButtonKind.SECONDARY -> m.background(colors.brandSoft, shape).border(BorderStroke(1.dp, colors.brandSoftStroke), shape)
        ActionButtonKind.DESTRUCTIVE -> m.background(colors.negative.copy(alpha = 0.10f), shape)
            .border(BorderStroke(1.dp, colors.negative.copy(alpha = 0.18f)), shape)
        ActionButtonKind.PLAIN -> m
    }
    m = m.alpha(if (!enabled) 0.45f else if (pressed) 0.9f else 1f)
        .clickable(interaction, indication = null, enabled = enabled && !running, role = Role.Button, onClick = onClick)
        .padding(horizontal = RenderTheme.Spacing.md, vertical = RenderTheme.Spacing.xs)
    Row(m, horizontalArrangement = Arrangement.spacedBy(RenderTheme.Spacing.xs, Alignment.CenterHorizontally), verticalAlignment = Alignment.CenterVertically) {
        if (running) {
            CircularProgressIndicator(Modifier.size(18.dp), color = foreground, strokeWidth = 2.dp)
        } else if (icon != null && LocalSymbolIcons.current(icon) != null) {
            // 表に無い名前は出さない(ボタンの中に細い円が並ぶより文字だけの方が読みやすい)
            SymbolIcon(icon, 18.dp, foreground)
        }
        Text(
            label,
            style = RenderTheme.Fonts.button,
            color = foreground,
            textAlign = TextAlign.Center,
            maxLines = 2,
            overflow = TextOverflow.Ellipsis,
        )
    }
}

/** 開けない URL など、作者の書き方の誤りを示す控えめな枠(iOS の `UnsupportedComponent(label:)`)。 */
@Composable
fun ActionNotice(message: String, modifier: Modifier = Modifier) {
    Text(
        message,
        modifier
            .fillMaxWidth()
            .defaultMinSize(minHeight = 44.dp)
            .border(1.dp, RenderTheme.colors.hairline, RoundedCornerShape(RenderTheme.Radius.inner))
            .padding(RenderTheme.Spacing.sm),
        style = MaterialTheme.typography.labelSmall,
        color = MaterialTheme.colorScheme.onSurfaceVariant,
        textAlign = TextAlign.Center,
        maxLines = 2,
    )
}
