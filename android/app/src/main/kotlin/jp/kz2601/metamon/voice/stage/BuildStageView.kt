package jp.kz2601.metamon.voice.stage

import androidx.compose.animation.AnimatedVisibility
import androidx.compose.animation.core.Spring
import androidx.compose.animation.core.animateDpAsState
import androidx.compose.animation.core.animateFloatAsState
import androidx.compose.animation.core.spring
import androidx.compose.animation.core.tween
import androidx.compose.animation.fadeIn
import androidx.compose.animation.fadeOut
import androidx.compose.animation.scaleIn
import androidx.compose.animation.scaleOut
import androidx.compose.animation.slideInVertically
import androidx.compose.foundation.Canvas
import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.BoxWithConstraints
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.aspectRatio
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.offset
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.widthIn
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.key
import androidx.compose.runtime.mutableLongStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.runtime.withFrameMillis
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.draw.scale
import androidx.compose.ui.geometry.CornerRadius
import androidx.compose.ui.geometry.Offset
import androidx.compose.ui.geometry.Size
import androidx.compose.ui.graphics.Brush
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.drawscope.DrawScope
import androidx.compose.ui.graphics.drawscope.Stroke
import androidx.compose.ui.semantics.clearAndSetSemantics
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.Dp
import androidx.compose.ui.unit.IntOffset
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import jp.kz2601.metamon.renderer.layout.GridArea
import jp.kz2601.metamon.renderer.theme.LocalReduceMotion
import jp.kz2601.metamon.renderer.theme.RenderTheme
import jp.kz2601.metamon.renderer.theme.colors
import jp.kz2601.metamon.voice.stage.BuildChoreography.AssemblyPlan
import jp.kz2601.metamon.voice.stage.BuildChoreography.LiveDraft
import jp.kz2601.metamon.voice.stage.BuildChoreography.PartKind
import jp.kz2601.metamon.voice.stage.BuildChoreography.PlacedPart
import jp.kz2601.metamon.voice.stage.BuildChoreography.WaitingScript

/**
 * 生成中の「組み立て台」(#894・iOS の `BuildStageView`)。上に空の画面(6 列 × 12 行)、下にパーツ箱。
 * - 待っている間: [WaitingScript] のコマに合わせて、箱の部品が持ち上がり → 画面に置かれる・入れ替わる・並べ直される
 * - SSE で本物の部品が届いたら([live]): 届いた順に下書きと置き換える(仮置き)
 * - 結果が届いた後([plan] あり): 本物の結果の部品を上から 1 つずつ置く
 * 「アニメーションを削除」では位置を動かさずフェードだけ。読み上げからは隠す(状態は生成中の文言で伝える)。
 */
@Composable
fun BuildStageView(
    script: WaitingScript,
    elapsedSeconds: Double,
    plan: AssemblyPlan?,
    live: LiveDraft?,
    modifier: Modifier = Modifier,
) {
    val reduceMotion = LocalReduceMotion.current
    Box(modifier.clearAndSetSemantics {}, contentAlignment = Alignment.Center) {
        if (plan != null) {
            // 組み上げの開始からの経過(ms)。動きを減らす設定では全部を一度に出す
            var revealMillis by remember(plan) { mutableLongStateOf(0L) }
            if (!reduceMotion) {
                LaunchedEffect(plan) {
                    val start = withFrameMillis { it }
                    while (revealMillis / 1000.0 < plan.total) {
                        withFrameMillis { revealMillis = it - start }
                    }
                }
            }
            val t = revealMillis / 1000.0
            val placed = plan.steps.filter { reduceMotion || it.delay <= t }.map { it.part }
            val picking = plan.steps.firstOrNull { !reduceMotion && it.delay > t }?.part?.kind
            Stage(placed, plan.rows, plan.tray, picking, plan.tabs, reduceMotion)
        } else {
            val step = WaitingScript.step(elapsedSeconds, reduceMotion)
            val frame = script.frame(step, live)
            Stage(frame.placed, frame.rows, live?.tray(script.tray) ?: script.tray, frame.picking, live?.tabs.orEmpty(), reduceMotion)
        }
    }
}

@Composable
private fun Stage(parts: List<PlacedPart>, rows: Int, tray: List<PartKind>, picking: PartKind?, tabs: List<String>, reduceMotion: Boolean) {
    Column(horizontalAlignment = Alignment.CenterHorizontally, verticalArrangement = Arrangement.spacedBy(12.dp)) {
        BuildCanvas(parts, rows, if (tabs.size >= 2) tabs else emptyList(), reduceMotion, Modifier.widthIn(max = 220.dp).fillMaxWidth().aspectRatio(6f / 10f))
        PartTray(tray, picking, reduceMotion)
    }
}

/** 空の画面に部品のシルエットを置く。部品の id が同じなら場所の変化を動かす(並べ直し)。 */
@Composable
private fun BuildCanvas(parts: List<PlacedPart>, rows: Int, tabs: List<String>, reduceMotion: Boolean, modifier: Modifier) {
    val colors = RenderTheme.colors
    val shape = RoundedCornerShape(RenderTheme.Radius.inner)
    Column(
        modifier
            .clip(shape)
            .background(colors.surface.copy(alpha = 0.7f))
            .border(1.dp, colors.hairline, shape),
    ) {
        BoxWithConstraints(Modifier.weight(1f).fillMaxWidth().padding(8.dp)) {
            val cw = maxWidth / GridArea.COLUMNS
            val ch = maxHeight / maxOf(rows, GridArea.VISIBLE_ROWS)
            val dot = colors.brandA.copy(alpha = 0.18f)
            // 画面の地(ドットのグリッド)
            Canvas(Modifier.fillMaxSize()) {
                val cwPx = size.width / GridArea.COLUMNS
                val chPx = size.height / GridArea.VISIBLE_ROWS
                for (r in 0 until GridArea.VISIBLE_ROWS) {
                    for (c in 0 until GridArea.COLUMNS) {
                        drawCircle(dot, radius = 1.5.dp.toPx(), center = Offset(c * cwPx + cwPx / 2, r * chPx + chPx / 2))
                    }
                }
            }
            val gap = 5.dp
            parts.forEach { p ->
                key(p.id) {
                    PlacedSilhouette(p, cw, ch, gap, reduceMotion)
                }
            }
        }
        if (tabs.isNotEmpty()) TabStrip(tabs)
    }
}

@Composable
private fun PlacedSilhouette(p: PlacedPart, cw: Dp, ch: Dp, gap: Dp, reduceMotion: Boolean) {
    val move = if (reduceMotion) tween<Dp>(400) else spring(dampingRatio = 0.6f, stiffness = Spring.StiffnessMediumLow)
    val x by animateDpAsState(cw * p.area.col1 + gap / 2, move, label = "x")
    val y by animateDpAsState(ch * (p.area.row1 - 1) + gap / 2, move, label = "y")
    val w by animateDpAsState(maxOf(0.dp, cw * p.area.colSpan - gap), move, label = "w")
    val h by animateDpAsState(maxOf(0.dp, ch * p.area.rowSpan - gap), move, label = "h")
    // 置いた瞬間: 下から少し小さく持ち上がって入る(動きを減らす設定ではフェードだけ)
    val shown = remember { androidx.compose.animation.core.MutableTransitionState(false) }.apply { targetState = true }
    AnimatedVisibility(
        visibleState = shown,
        enter = if (reduceMotion) fadeIn(tween(400)) else fadeIn() + scaleIn(initialScale = 0.7f) + slideInVertically { it / 3 },
        exit = if (reduceMotion) fadeOut() else fadeOut() + scaleOut(targetScale = 0.85f),
        modifier = Modifier.offset { IntOffset(x.roundToPx(), y.roundToPx()) }.size(w, h),
    ) {
        PartSilhouette(p.kind, p.caption, Modifier.fillMaxSize())
    }
}

/** 組み立て台の下のタブの帯。本物のアプリのタブの見出しを小さく並べる。 */
@Composable
private fun TabStrip(titles: List<String>) {
    val colors = RenderTheme.colors
    Row(
        Modifier.fillMaxWidth().border(0.5.dp, colors.hairline).padding(horizontal = 6.dp, vertical = 5.dp),
        horizontalArrangement = Arrangement.spacedBy(2.dp),
    ) {
        titles.take(5).forEachIndexed { i, title ->
            Column(Modifier.weight(1f), horizontalAlignment = Alignment.CenterHorizontally) {
                Box(Modifier.size(5.dp).clip(CircleShape).background(if (i == 0) colors.brandA else colors.brandA.copy(alpha = 0.3f)))
                Text(
                    title,
                    fontSize = 7.sp,
                    fontWeight = if (i == 0) FontWeight.Bold else FontWeight.Medium,
                    color = if (i == 0) colors.label else colors.secondaryLabel,
                    maxLines = 1,
                    overflow = TextOverflow.Ellipsis,
                )
            }
        }
    }
}

/** パーツ箱。今選んでいる部品が持ち上がって光る。 */
@Composable
private fun PartTray(kinds: List<PartKind>, picking: PartKind?, reduceMotion: Boolean) {
    val colors = RenderTheme.colors
    Row(
        Modifier.clip(CircleShape).background(colors.surface.copy(alpha = 0.85f)).padding(8.dp),
        horizontalArrangement = Arrangement.spacedBy(8.dp),
    ) {
        kinds.forEach { kind ->
            key(kind) {
                val active = kind == picking
                val lift by animateFloatAsState(
                    if (active && !reduceMotion) 1f else 0f,
                    if (reduceMotion) tween(300) else spring(dampingRatio = 0.5f, stiffness = Spring.StiffnessMedium),
                    label = "lift",
                )
                val shape = RoundedCornerShape(9.dp)
                Box(
                    Modifier
                        .offset { IntOffset(0, ((-6).dp * lift).roundToPx()) }
                        .scale(1f + 0.15f * lift)
                        .size(34.dp)
                        .clip(shape)
                        .background(if (active) Brush.linearGradient(listOf(colors.brandFillStart, colors.brandFillEnd)) else Brush.linearGradient(listOf(colors.surfaceElevated, colors.surfaceElevated)))
                        .border(1.dp, colors.hairline, shape)
                        .padding(7.dp),
                ) {
                    PartSilhouette(kind, caption = null, Modifier.fillMaxSize(), mini = true, onAccent = active)
                }
            }
        }
    }
}

/**
 * 実在する部品の形のシルエット。本物の結果では部品の名前(caption)を添える。
 * [mini] はパーツ箱の小さい絵(枠を描かない)。[onAccent] はブランド色の上に描く時(白で描く)。
 */
@Composable
fun PartSilhouette(kind: PartKind, caption: String?, modifier: Modifier = Modifier, mini: Boolean = false, onAccent: Boolean = false) {
    val colors = RenderTheme.colors
    val tint = if (onAccent) Color.White else colors.brandA
    val shape = RoundedCornerShape(6.dp)
    val base = if (mini) {
        modifier
    } else if (kind == PartKind.BUTTON) {
        modifier.clip(shape).background(Brush.linearGradient(listOf(colors.brandFillStart, colors.brandFillEnd))).padding(4.dp)
    } else {
        modifier.clip(shape).background(colors.surfaceElevated).border(1.dp, colors.brandB.copy(alpha = 0.35f), shape).padding(4.dp)
    }
    Box(base, contentAlignment = Alignment.Center) {
        val showCaption = caption != null && !mini
        Canvas(Modifier.fillMaxSize()) { drawKind(kind, if (kind == PartKind.BUTTON && !mini) Color.White else tint, skipFirstBar = showCaption) }
        if (showCaption) {
            Text(
                caption.orEmpty(),
                modifier = Modifier.align(if (kind == PartKind.BUTTON) Alignment.Center else Alignment.TopStart),
                fontSize = if (kind == PartKind.HEADING) 9.sp else 7.sp,
                fontWeight = if (kind == PartKind.HEADING || kind == PartKind.BUTTON) FontWeight.Bold else FontWeight.Medium,
                color = if (kind == PartKind.BUTTON) Color.White else colors.secondaryLabel,
                maxLines = 1,
                overflow = TextOverflow.Ellipsis,
            )
        }
    }
}

/** 種類ごとの中身の絵(iOS の PartSilhouette の content)。 */
private fun DrawScope.drawKind(kind: PartKind, tint: Color, skipFirstBar: Boolean) {
    val w = size.width
    val h = size.height
    val soft = tint.copy(alpha = 0.3f)
    fun bar(y: Float, width: Float, height: Float = 2.5.dp.toPx()) =
        drawRoundRect(soft, Offset(0f, y), Size(w * width, height), CornerRadius(height / 2))
    val line = 6.dp.toPx()
    when (kind) {
        PartKind.HEADING -> {
            if (!skipFirstBar) drawRoundRect(soft, Offset(0f, h / 2 - line), Size(w * 0.7f, 6.dp.toPx()), CornerRadius(3.dp.toPx()))
            bar(h / 2 + 2.dp.toPx(), 0.4f, 3.dp.toPx())
        }
        PartKind.TEXT, PartKind.CARD -> {
            if (!skipFirstBar) bar(0f, 0.8f)
            bar(line, 0.6f)
            bar(line * 2, 0.7f)
        }
        PartKind.BUTTON -> if (!skipFirstBar) {
            drawRoundRect(tint.copy(alpha = 0.85f), Offset(w / 2 - 14.dp.toPx(), h / 2 - 1.5.dp.toPx()), Size(28.dp.toPx().coerceAtMost(w), 3.dp.toPx()), CornerRadius(1.5.dp.toPx()))
        }
        PartKind.FIELD -> {
            if (!skipFirstBar) bar(0f, 0.35f, 3.dp.toPx())
            val top = if (skipFirstBar) 10.dp.toPx() else line
            drawRoundRect(tint.copy(alpha = 0.4f), Offset(0f, top), Size(w, minOf(14.dp.toPx(), h - top).coerceAtLeast(0f)), CornerRadius(3.dp.toPx()), style = Stroke(1.dp.toPx()))
        }
        PartKind.TOGGLE -> {
            val r = 4.dp.toPx()
            drawRoundRect(tint.copy(alpha = 0.7f), Offset(0f, h / 2 - r), Size(r * 2, r * 2), CornerRadius(2.dp.toPx()))
            drawRoundRect(tint.copy(alpha = 0.5f), Offset(w - 14.dp.toPx(), h / 2 - r), Size(14.dp.toPx(), r * 2), CornerRadius(r))
        }
        PartKind.LIST -> for (i in 0 until 3) {
            val y = i * 10.dp.toPx() + 3.dp.toPx()
            if (y > h) break
            drawCircle(soft, 3.dp.toPx(), Offset(3.dp.toPx(), y))
            drawRoundRect(soft, Offset(9.dp.toPx(), y - 1.25.dp.toPx()), Size((w - 9.dp.toPx()) * 0.75f, 2.5.dp.toPx()), CornerRadius(1.25.dp.toPx()))
        }
        PartKind.CHART -> {
            val values = listOf(0.4f, 0.7f, 0.5f, 0.9f, 0.6f)
            val gap = 3.dp.toPx()
            val bw = (w - gap * (values.size - 1)) / values.size
            values.forEachIndexed { i, v ->
                drawRoundRect(tint.copy(alpha = 0.6f), Offset(i * (bw + gap), h * (1 - v)), Size(bw, h * v), CornerRadius(1.5.dp.toPx()))
            }
        }
        PartKind.STAT -> {
            if (!skipFirstBar) bar(h / 2 - 8.dp.toPx(), 0.4f, 3.dp.toPx())
            drawRoundRect(tint.copy(alpha = 0.7f), Offset(0f, h / 2), Size(w * 0.45f, 7.dp.toPx().coerceAtMost(h / 2)), CornerRadius(2.dp.toPx()))
        }
        PartKind.CALENDAR -> {
            val stepX = w / 7
            for (r in 0 until 3) for (c in 0 until 7) {
                drawCircle(soft, 1.5.dp.toPx(), Offset(c * stepX + stepX / 2, h / 2 + (r - 1) * 5.dp.toPx()))
            }
        }
        PartKind.MAP -> {
            val r = minOf(w, h) / 6
            drawCircle(tint.copy(alpha = 0.7f), r, Offset(w / 2, h / 2 - r / 2))
            drawOval(soft, Offset(w / 2 - r * 1.5f, h / 2 + r * 0.8f), Size(r * 3, r))
        }
        PartKind.MEDIA -> {
            val s = minOf(w, h) / 2.5f
            drawRoundRect(tint.copy(alpha = 0.7f), Offset(w / 2 - s / 2, h / 2 - s / 2), Size(s, s * 0.8f), CornerRadius(2.dp.toPx()), style = Stroke(1.2.dp.toPx()))
            drawCircle(tint.copy(alpha = 0.7f), s / 8, Offset(w / 2 - s / 5, h / 2 - s / 6))
        }
    }
}
