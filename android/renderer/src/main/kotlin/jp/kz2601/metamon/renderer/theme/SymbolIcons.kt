package jp.kz2601.metamon.renderer.theme

import androidx.compose.foundation.border
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.material3.Icon
import androidx.compose.runtime.Composable
import androidx.compose.runtime.staticCompositionLocalOf
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.vector.ImageVector
import androidx.compose.ui.unit.Dp
import jp.kz2601.metamon.renderer.theme.icons.materialVector
import jp.kz2601.metamon.renderer.theme.icons.symbolMaterialNames

/**
 * DSL のアイコン名(SF Symbols の語彙)→ Material アイコン。表の正典は docs/dsl/icon-map.json(iOS と共有)、
 * Kotlin の表は android/scripts/gen-symbol-icons.py がそこから作る(icons/SymbolIconTable.kt)。
 */
object SymbolIcons {
    private val suffixes = listOf(".fill", ".circle", ".square")

    /** 名前 → Material の名前。末尾の修飾(.fill / .circle.fill など)を外して引き直す。無い名前は null。 */
    fun materialName(name: String?): String? {
        var n = name?.trim()?.lowercase()?.takeIf { it.isNotEmpty() } ?: return null
        symbolMaterialNames[n]?.let { return it }
        for (suffix in suffixes) {
            n = n.removeSuffix(suffix)
            symbolMaterialNames[n]?.let { return it }
        }
        return null
    }

    /** 名前 → 絵。無い名前は null(呼び側が既定の印を出す)。 */
    fun vector(name: String?): ImageVector? = materialName(name)?.let(::materialVector)
}

/** アイコン名 → 絵。プレビューやテストで差し替えられる。 */
val LocalSymbolIcons = staticCompositionLocalOf<(String?) -> ImageVector?> { SymbolIcons::vector }

/**
 * アイコン 1 個。表に無い名前は細い円(iOS の既定 `circle` に近い控えめな印)。
 * 読み上げはしない(意味は隣の文字が持つ)。
 */
@Composable
fun SymbolIcon(name: String?, size: Dp, tint: Color, modifier: Modifier = Modifier) {
    val vector = LocalSymbolIcons.current(name)
    if (vector != null) {
        Icon(vector, contentDescription = null, modifier = modifier.size(size), tint = tint)
    } else {
        Box(modifier.size(size).padding(size / 8).border(size / 12, tint, CircleShape))
    }
}
