package jp.kz2601.metamon.renderer

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.material3.Button
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedCard
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.tooling.preview.Preview
import androidx.compose.ui.unit.dp
import jp.kz2601.metamon.dsl.AppDSL
import jp.kz2601.metamon.dsl.Component
import jp.kz2601.metamon.dsl.ComponentType
import jp.kz2601.metamon.dsl.Screen

// 骨格(#356): 最初の画面を縦に並べるだけ。グリッド配置(area)・ナビ・部品のレジストリは #357 で入れる。

/** DSL の最初の画面を描く。 */
@Composable
fun DSLView(dsl: AppDSL, runtime: DSLRuntime, modifier: Modifier = Modifier) {
    val screen = dsl.screen(dsl.initialScreen) ?: dsl.screens.firstOrNull()
    if (screen == null) {
        UnsupportedComponent("画面がありません", modifier)
        return
    }
    ScreenView(screen, runtime, modifier)
}

@Composable
fun ScreenView(screen: Screen, runtime: DSLRuntime, modifier: Modifier = Modifier) {
    Column(
        modifier = modifier.fillMaxSize().padding(16.dp),
        verticalArrangement = Arrangement.spacedBy(12.dp),
    ) {
        if (screen.title.isNotEmpty()) {
            Text(screen.title, style = MaterialTheme.typography.headlineSmall)
        }
        screen.components.forEach { ComponentView(it, runtime) }
    }
}

@Composable
fun ComponentView(component: Component, runtime: DSLRuntime, modifier: Modifier = Modifier) {
    when {
        component.type == ComponentType.VOICE_INPUT -> VoiceInputView(component, runtime, modifier)
        component.type == ComponentType.TEXT -> Text(component.text.orEmpty(), modifier)
        component.type == ComponentType.DIVIDER -> HorizontalDivider(modifier)
        component.type == ComponentType.SPACER -> Spacer(modifier.height(16.dp))
        component.type.isContainer -> Column(modifier, verticalArrangement = Arrangement.spacedBy(8.dp)) {
            component.title?.let { Text(it, style = MaterialTheme.typography.titleMedium) }
            component.children.forEach { ComponentView(it, runtime) }
        }
        component.type == ComponentType.UNKNOWN -> UnsupportedComponent("アプリを更新すると表示されます", modifier)
        else -> UnsupportedComponent("「${component.type.wire}」は Android ではまだ使えません", modifier)
    }
}

/** 音声の入口(仮)。認識は #359。 */
@Composable
fun VoiceInputView(component: Component, runtime: DSLRuntime, modifier: Modifier = Modifier) {
    Column(
        modifier = modifier.fillMaxWidth().padding(vertical = 48.dp),
        horizontalAlignment = Alignment.CenterHorizontally,
        verticalArrangement = Arrangement.spacedBy(24.dp),
    ) {
        Text(
            component.prompt ?: "話しかけてください",
            style = MaterialTheme.typography.headlineSmall,
            textAlign = TextAlign.Center,
        )
        val buttonSize = if (component.size == "large") 120.dp else 72.dp
        Button(
            onClick = { runtime.onVoiceInput(component.id) },
            shape = CircleShape,
            modifier = Modifier.size(buttonSize),
        ) {
            Text("話す", style = MaterialTheme.typography.titleMedium)
        }
    }
}

@Composable
private fun UnsupportedComponent(message: String, modifier: Modifier = Modifier) {
    OutlinedCard(modifier.fillMaxWidth()) {
        Box(Modifier.padding(12.dp)) {
            Text(message, style = MaterialTheme.typography.bodySmall)
        }
    }
}

@Preview(showBackground = true)
@Composable
private fun DSLViewPreview() {
    MaterialTheme {
        DSLView(AppDSL.fallback, NoopDSLRuntime)
    }
}
