package jp.kz2601.metamon.desktop

import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import androidx.compose.foundation.focusable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableIntStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.input.key.Key
import androidx.compose.ui.input.key.KeyEventType
import androidx.compose.ui.input.key.key
import androidx.compose.ui.input.key.onKeyEvent
import androidx.compose.ui.input.key.type
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.semantics.stateDescription
import androidx.compose.ui.unit.dp
import jp.kz2601.metamon.desktop.i18n.Strings
import jp.kz2601.metamon.dsl.Component

/** carousel の送り(端で止まる・回らない)。 */
object CarouselPager {
    fun clamp(index: Int, count: Int): Int = if (count <= 0) 0 else index.coerceIn(0, count - 1)

    fun prev(index: Int, count: Int): Int = clamp(index - 1, count)

    fun next(index: Int, count: Int): Int = clamp(index + 1, count)

    fun canPrev(index: Int, count: Int): Boolean = clamp(index, count) > 0

    fun canNext(index: Int, count: Int): Boolean = clamp(index, count) < count - 1

    /** ← → で送った先。他のキーは null(流す)。 */
    fun onKey(key: Key, index: Int, count: Int): Int? = when (key) {
        Key.DirectionLeft -> prev(index, count)
        Key.DirectionRight -> next(index, count)
        else -> null
    }

    /** 読み上げ「2 / 5 枚目」。 */
    fun pageLabel(index: Int, count: Int): String = Strings.format("desktop_carousel_page", clamp(index, count) + 1, count)
}

/**
 * `carousel`: 1 枚ずつ見せ、‹ ›・← →・ページの点で送る(iOS と同じ)。動きは付けず、その場で替える(動きを減らす設定でも同じ)。
 * [pages] は隠す子を外した後の子。描く物は [child] が描く(DesktopApp の Node)。
 */
@Composable
fun CarouselContainer(c: Component, pages: List<Component>, child: @Composable (Component) -> Unit) {
    val count = pages.size
    var index by remember(c) { mutableIntStateOf(0) }
    val at = CarouselPager.clamp(index, count)
    Column(Modifier.fillMaxWidth(), verticalArrangement = Arrangement.spacedBy(8.dp)) {
        c.title?.let { Text(it, style = MaterialTheme.typography.titleMedium) }
        if (count == 0) return@Column
        Row(
            Modifier
                .fillMaxWidth()
                .semantics { stateDescription = CarouselPager.pageLabel(at, count) }
                .onKeyEvent { e ->
                    if (e.type != KeyEventType.KeyDown) return@onKeyEvent false
                    CarouselPager.onKey(e.key, at, count)?.let { index = it; true } ?: false
                }
                .focusable(),
            verticalAlignment = Alignment.CenterVertically,
        ) {
            val prev = Strings["desktop_carousel_prev"]
            val next = Strings["desktop_carousel_next"]
            TextButton(
                onClick = { index = CarouselPager.prev(at, count) },
                enabled = CarouselPager.canPrev(at, count),
                modifier = Modifier.semantics { contentDescription = prev },
            ) { Text("‹") }
            Box(Modifier.weight(1f)) { child(pages[at]) }
            TextButton(
                onClick = { index = CarouselPager.next(at, count) },
                enabled = CarouselPager.canNext(at, count),
                modifier = Modifier.semantics { contentDescription = next },
            ) { Text("›") }
        }
        if (count > 1) {
            Row(Modifier.align(Alignment.CenterHorizontally), horizontalArrangement = Arrangement.spacedBy(6.dp)) {
                repeat(count) { i ->
                    val go = Strings.format("desktop_carousel_go", i + 1)
                    val color = if (i == at) MaterialTheme.colorScheme.primary else MaterialTheme.colorScheme.outlineVariant
                    Box(
                        Modifier
                            .size(8.dp)
                            .background(color, CircleShape)
                            .clickable { index = i }
                            .semantics { contentDescription = go },
                    )
                }
            }
        }
    }
}
