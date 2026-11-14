package jp.kz2601.metamon.desktop

import androidx.compose.foundation.border
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.material3.Icon
import androidx.compose.material3.LocalContentColor
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.vector.ImageVector
import androidx.compose.ui.unit.Dp
import androidx.compose.ui.unit.dp
import jp.kz2601.metamon.dsl.Component
import jp.kz2601.metamon.dsl.Size as DslSize
import jp.kz2601.metamon.renderer.theme.icons.materialVector
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive
import kotlinx.serialization.json.contentOrNull

/**
 * DSL のアイコン名(SF Symbols の語彙)→ Material の絵(#1742)。
 * 名前の表は docs/dsl/icon-map.json をビルド時に同梱して読む(build.gradle.kts の syncIconMap)。
 * 絵は Android が icon-map.json から作った表(renderer/theme/icons)をビルド時に取り込む。
 */
object DesktopIcons {
    const val RESOURCE = "icons/icon-map.json"
    private val suffixes = listOf(".fill", ".circle", ".square")

    /** SF Symbols の名前 → Material の名前。 */
    val symbols: Map<String, String> by lazy {
        val text = DesktopIcons::class.java.classLoader.getResourceAsStream(RESOURCE)?.use { it.readBytes().decodeToString() } ?: return@lazy emptyMap()
        val symbols = (Json.parseToJsonElement(text) as JsonObject)["symbols"] as? JsonObject ?: return@lazy emptyMap()
        symbols.mapNotNull { (k, v) -> (v as? JsonPrimitive)?.contentOrNull?.let { k to it } }.toMap()
    }

    /** 名前 → Material の名前。末尾の修飾(.fill / .circle.fill など)を外して引き直す(Android の SymbolIcons と同じ)。無い名前は null。 */
    fun materialName(name: String?): String? {
        var n = name?.trim()?.lowercase()?.takeIf { it.isNotEmpty() } ?: return null
        symbols[n]?.let { return it }
        for (suffix in suffixes) {
            n = n.removeSuffix(suffix)
            symbols[n]?.let { return it }
        }
        return null
    }

    /** 名前 → 絵。無い名前は null(呼び側が既定の印を出す)。 */
    fun vector(name: String?): ImageVector? = materialName(name)?.let(::materialVector)

    fun size(c: Component): Dp = when (c.size) {
        DslSize.SMALL, DslSize.COMPACT -> 16.dp
        DslSize.LARGE -> 44.dp
        else -> 28.dp
    }
}

/** `icon`。表に無い名前は細い円(iOS の既定 `circle` に近い控えめな印)。label が無ければ飾り(読み上げは A11y.kt)。 */
@Composable
fun IconLeaf(c: Component) {
    val size = DesktopIcons.size(c)
    val tint = dslColor(c.color) ?: LocalContentColor.current
    Box(Modifier.fillMaxWidth(), contentAlignment = Alignment.Center) {
        val vector = DesktopIcons.vector(c.name)
        if (vector != null) {
            Icon(vector, contentDescription = null, modifier = Modifier.size(size), tint = tint)
        } else {
            Box(Modifier.size(size).padding(size / 8).border(size / 12, tint, CircleShape))
        }
    }
}
