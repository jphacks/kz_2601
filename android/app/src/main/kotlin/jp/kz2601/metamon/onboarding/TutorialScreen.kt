package jp.kz2601.metamon.onboarding

import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.safeDrawingPadding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.widthIn
import androidx.compose.foundation.pager.HorizontalPager
import androidx.compose.foundation.pager.rememberPagerState
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.Button
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.alpha
import androidx.compose.ui.platform.testTag
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.heading
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import androidx.compose.ui.window.Dialog
import androidx.compose.ui.window.DialogProperties
import jp.kz2601.metamon.R
import jp.kz2601.metamon.renderer.theme.RenderTheme
import jp.kz2601.metamon.renderer.theme.colors
import jp.kz2601.metamon.ui.CraneMascot
import kotlinx.coroutines.launch

/**
 * 初回のチュートリアル(#900・iOS の `TutorialView`)。全画面で出し、スキップでき、設定の「使い方を見る」から何度でも見られる。
 * スキップ・「はじめる」・戻るのどれでも [onFinish](見た扱い)。
 */
@Composable
fun TutorialScreen(onFinish: () -> Unit) {
    Dialog(
        onDismissRequest = onFinish,
        properties = DialogProperties(usePlatformDefaultWidth = false, dismissOnClickOutside = false),
    ) {
        Surface(Modifier.fillMaxSize().testTag("tutorial.screen"), color = MaterialTheme.colorScheme.background) {
            TutorialContent(FirstRunTutorial.cards, onFinish)
        }
    }
}

@Composable
private fun TutorialContent(cards: List<FirstRunTutorial.Card>, onFinish: () -> Unit) {
    val pager = rememberPagerState { cards.size }
    val scope = rememberCoroutineScope()
    val isLast = pager.currentPage >= cards.size - 1
    Column(Modifier.fillMaxSize().safeDrawingPadding(), horizontalAlignment = Alignment.CenterHorizontally) {
        Row(Modifier.fillMaxWidth().padding(horizontal = 8.dp)) {
            Box(Modifier.weight(1f))
            TextButton(
                onClick = onFinish,
                enabled = !isLast,
                modifier = Modifier.alpha(if (isLast) 0f else 1f).testTag("tutorial.skip"),
            ) {
                Text(stringResource(R.string.onboarding_skip), color = MaterialTheme.colorScheme.onSurfaceVariant)
            }
        }
        HorizontalPager(pager, Modifier.weight(1f).fillMaxWidth()) { index ->
            TutorialCard(cards[index])
        }
        if (cards.size > 1) { // 1 枚(H1)の時は点を出さない
            PageDots(pager.currentPage, cards.size)
        }
        Button(
            onClick = {
                if (isLast) onFinish() else scope.launch { pager.animateScrollToPage(pager.currentPage + 1) }
            },
            modifier = Modifier
                .padding(horizontal = 24.dp, vertical = 24.dp)
                .widthIn(max = 480.dp)
                .fillMaxWidth()
                .height(52.dp)
                .testTag(if (isLast) "tutorial.start" else "tutorial.next"),
        ) {
            Text(stringResource(if (isLast) R.string.onboarding_start else R.string.onboarding_next))
        }
    }
}

@Composable
private fun TutorialCard(card: FirstRunTutorial.Card) {
    Column(
        Modifier
            .fillMaxSize()
            .verticalScroll(rememberScrollState())
            .padding(horizontal = 24.dp)
            .testTag("tutorial.card.${card.id}"),
        horizontalAlignment = Alignment.CenterHorizontally,
        verticalArrangement = Arrangement.spacedBy(20.dp),
    ) {
        CraneMascot(card.pose, Modifier.padding(top = 32.dp), size = 140.dp, sway = 6f)
        Text(
            stringResource(card.title),
            fontSize = 26.sp,
            fontWeight = FontWeight.Bold,
            textAlign = TextAlign.Center,
            modifier = Modifier.widthIn(max = 480.dp).semantics { heading() },
        )
        Text(
            stringResource(card.body),
            style = MaterialTheme.typography.bodyLarge,
            textAlign = TextAlign.Center,
            modifier = Modifier.widthIn(max = 480.dp),
        )
        card.note?.let { note ->
            Text(
                stringResource(note),
                style = MaterialTheme.typography.bodySmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
                modifier = Modifier
                    .widthIn(max = 480.dp)
                    .background(RenderTheme.colors.brandSoft, RoundedCornerShape(RenderTheme.Radius.card))
                    .padding(16.dp),
            )
        }
    }
}

@Composable
private fun PageDots(current: Int, count: Int) {
    val label = stringResource(R.string.onboarding_page, current + 1, count)
    Row(
        Modifier.padding(bottom = 8.dp).semantics { contentDescription = label },
        horizontalArrangement = Arrangement.spacedBy(8.dp),
    ) {
        repeat(count) { index ->
            Box(
                Modifier
                    .size(height = 8.dp, width = if (index == current) 22.dp else 8.dp)
                    .background(
                        if (index == current) RenderTheme.colors.brandA else RenderTheme.colors.hairline,
                        CircleShape,
                    ),
            )
        }
    }
}
