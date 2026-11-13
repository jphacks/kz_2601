package jp.kz2601.metamon.desktop

import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.mutableStateMapOf
import androidx.compose.runtime.staticCompositionLocalOf
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.input.key.Key
import androidx.compose.ui.input.key.KeyEventType
import androidx.compose.ui.input.key.key
import androidx.compose.ui.input.key.onPreviewKeyEvent
import androidx.compose.ui.input.key.type
import androidx.compose.ui.semantics.Role
import androidx.compose.ui.semantics.clearAndSetSemantics
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.semantics.stateDescription
import androidx.compose.ui.unit.dp
import jp.kz2601.metamon.desktop.i18n.Strings
import jp.kz2601.metamon.dsl.Component

/** disclosure の開け閉めの規則(Compose に依存しない部分)。iOS の DisclosureContainer と同じく、既定は `expanded`(無ければ閉じる)。 */
object Disclosure {
    fun initiallyExpanded(c: Component): Boolean = c.expanded ?: false

    /** 覚える鍵: 画面 + 部品の id(無ければ見出し)。 */
    fun key(screen: String, c: Component): String = "$screen#${c.id ?: c.title.orEmpty()}"

    /** 見出しで開け閉めするキー(Enter / Space)。 */
    fun isToggleKey(key: Key): Boolean = key == Key.Enter || key == Key.NumPadEnter || key == Key.Spacebar

    fun title(c: Component): String = c.title ?: Strings["renderer_disclosure_default_title"]

    /** 読み上げ: 開いている / 閉じている(Android の renderer と同じ文言)。 */
    fun stateText(expanded: Boolean): String = Strings[if (expanded) "renderer_disclosure_expanded" else "renderer_disclosure_collapsed"]
}

/** ウィンドウの間だけ覚える開け閉め(ウィンドウを閉じたら忘れる・端末には残さない)。 */
class DisclosureMemory {
    private val toggled = mutableStateMapOf<String, Boolean>()

    fun isExpanded(key: String, c: Component): Boolean = toggled[key] ?: Disclosure.initiallyExpanded(c)

    fun toggle(key: String, c: Component) {
        toggled[key] = !isExpanded(key, c)
    }
}

val LocalDisclosures = staticCompositionLocalOf { DisclosureMemory() }

/** 見出しの行(押す・Enter / Space で開け閉め)と、開いている時だけ子。 */
@Composable
internal fun DisclosureView(c: Component, screen: String, children: @Composable () -> Unit) {
    val memory = LocalDisclosures.current
    val key = Disclosure.key(screen, c)
    val open = memory.isExpanded(key, c)
    val state = Disclosure.stateText(open)
    Column(Modifier.fillMaxWidth(), verticalArrangement = Arrangement.spacedBy(8.dp)) {
        Row(
            Modifier.fillMaxWidth()
                // clickable も Enter / Space を受けるので、押し下げで切り替えて離すのは捨てる(2 回切り替えない)
                .onPreviewKeyEvent { e ->
                    if (!Disclosure.isToggleKey(e.key)) return@onPreviewKeyEvent false
                    if (e.type == KeyEventType.KeyDown) memory.toggle(key, c)
                    true
                }
                .clickable(role = Role.Button) { memory.toggle(key, c) }
                .semantics { stateDescription = state }
                .padding(vertical = 4.dp),
            verticalAlignment = Alignment.CenterVertically,
        ) {
            Text(Disclosure.title(c), style = MaterialTheme.typography.titleMedium, modifier = Modifier.weight(1f))
            // 矢印は飾り(読み上げは stateDescription)
            Text(if (open) "▾" else "▸", color = MaterialTheme.colorScheme.primary, modifier = Modifier.clearAndSetSemantics { })
        }
        if (open) children()
    }
}
