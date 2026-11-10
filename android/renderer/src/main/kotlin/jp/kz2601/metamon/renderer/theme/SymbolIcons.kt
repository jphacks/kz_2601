package jp.kz2601.metamon.renderer.theme

import androidx.compose.foundation.border
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.filled.ArrowBack
import androidx.compose.material.icons.automirrored.filled.ArrowForward
import androidx.compose.material.icons.automirrored.filled.KeyboardArrowLeft
import androidx.compose.material.icons.automirrored.filled.KeyboardArrowRight
import androidx.compose.material.icons.automirrored.filled.List
import androidx.compose.material.icons.automirrored.filled.Send
import androidx.compose.material.icons.filled.AccountCircle
import androidx.compose.material.icons.filled.Add
import androidx.compose.material.icons.filled.AddCircle
import androidx.compose.material.icons.filled.Build
import androidx.compose.material.icons.filled.Call
import androidx.compose.material.icons.filled.Check
import androidx.compose.material.icons.filled.CheckCircle
import androidx.compose.material.icons.filled.Close
import androidx.compose.material.icons.filled.DateRange
import androidx.compose.material.icons.filled.Delete
import androidx.compose.material.icons.filled.Edit
import androidx.compose.material.icons.filled.Email
import androidx.compose.material.icons.filled.Face
import androidx.compose.material.icons.filled.Favorite
import androidx.compose.material.icons.filled.FavoriteBorder
import androidx.compose.material.icons.filled.Home
import androidx.compose.material.icons.filled.Info
import androidx.compose.material.icons.filled.KeyboardArrowDown
import androidx.compose.material.icons.filled.KeyboardArrowUp
import androidx.compose.material.icons.filled.LocationOn
import androidx.compose.material.icons.filled.Lock
import androidx.compose.material.icons.filled.MoreVert
import androidx.compose.material.icons.filled.Notifications
import androidx.compose.material.icons.filled.Person
import androidx.compose.material.icons.filled.Phone
import androidx.compose.material.icons.filled.Place
import androidx.compose.material.icons.filled.PlayArrow
import androidx.compose.material.icons.filled.Refresh
import androidx.compose.material.icons.filled.Search
import androidx.compose.material.icons.filled.Settings
import androidx.compose.material.icons.filled.Share
import androidx.compose.material.icons.filled.ShoppingCart
import androidx.compose.material.icons.filled.Star
import androidx.compose.material.icons.filled.ThumbUp
import androidx.compose.material.icons.filled.Warning
import androidx.compose.material3.Icon
import androidx.compose.runtime.Composable
import androidx.compose.runtime.staticCompositionLocalOf
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.vector.ImageVector
import androidx.compose.ui.unit.Dp

/**
 * DSL のアイコン名(SF Symbols の語彙)→ Material アイコン。最小の表(material-icons-core に有る分だけ)。
 * 表の正典・Material Symbols 全体への対応は #842 で作り、[LocalSymbolIcons] を差し替える。
 */
object SymbolIcons {
    /** 末尾の修飾(.fill / .circle.fill など)を外した名前で引く。無い名前は null(呼び側が既定の印を出す)。 */
    fun vector(name: String?): ImageVector? {
        var n = name?.trim()?.lowercase()?.takeIf { it.isNotEmpty() } ?: return null
        table[n]?.let { return it }
        for (suffix in listOf(".fill", ".circle", ".square")) {
            n = n.removeSuffix(suffix)
            table[n]?.let { return it }
        }
        return null
    }

    private val table: Map<String, ImageVector> = mapOf(
        "arrow.clockwise" to Icons.Filled.Refresh,
        "arrow.left" to Icons.AutoMirrored.Filled.ArrowBack,
        "arrow.up.right" to Icons.AutoMirrored.Filled.ArrowForward,
        "arrow.right" to Icons.AutoMirrored.Filled.ArrowForward,
        "bell" to Icons.Filled.Notifications,
        "calendar" to Icons.Filled.DateRange,
        "cart" to Icons.Filled.ShoppingCart,
        "checklist" to Icons.AutoMirrored.Filled.List,
        "checkmark" to Icons.Filled.Check,
        "checkmark.circle" to Icons.Filled.CheckCircle,
        "chevron.down" to Icons.Filled.KeyboardArrowDown,
        "chevron.left" to Icons.AutoMirrored.Filled.KeyboardArrowLeft,
        "chevron.right" to Icons.AutoMirrored.Filled.KeyboardArrowRight,
        "chevron.up" to Icons.Filled.KeyboardArrowUp,
        "ellipsis" to Icons.Filled.MoreVert,
        "envelope" to Icons.Filled.Email,
        "exclamationmark.triangle" to Icons.Filled.Warning,
        "face.smiling" to Icons.Filled.Face,
        "gear" to Icons.Filled.Settings,
        "gearshape" to Icons.Filled.Settings,
        "hand.thumbsup" to Icons.Filled.ThumbUp,
        "heart" to Icons.Filled.FavoriteBorder,
        "heart.fill" to Icons.Filled.Favorite,
        "house" to Icons.Filled.Home,
        "info" to Icons.Filled.Info,
        "list.bullet" to Icons.AutoMirrored.Filled.List,
        "location" to Icons.Filled.LocationOn,
        "lock" to Icons.Filled.Lock,
        "magnifyingglass" to Icons.Filled.Search,
        "map" to Icons.Filled.Place,
        "mappin" to Icons.Filled.LocationOn,
        "mappin.and.ellipse" to Icons.Filled.LocationOn,
        "paperplane" to Icons.AutoMirrored.Filled.Send,
        "pencil" to Icons.Filled.Edit,
        "person" to Icons.Filled.Person,
        "person.2" to Icons.Filled.Person,
        "person.3" to Icons.Filled.Person,
        "person.crop.circle" to Icons.Filled.AccountCircle,
        "phone" to Icons.Filled.Call,
        "iphone" to Icons.Filled.Phone,
        "play" to Icons.Filled.PlayArrow,
        "plus" to Icons.Filled.Add,
        "plus.circle" to Icons.Filled.AddCircle,
        "square.and.arrow.up" to Icons.Filled.Share,
        "square.and.pencil" to Icons.Filled.Edit,
        "star" to Icons.Filled.Star,
        "trash" to Icons.Filled.Delete,
        "wrench" to Icons.Filled.Build,
        "xmark" to Icons.Filled.Close,
    )
}

/** アイコン名 → 絵。#842 の対応表で差し替える。 */
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
