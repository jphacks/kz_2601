// 生成物(android/scripts/gen-symbol-icons.py が docs/dsl/icon-map.json から作る)。手で直さない。
package jp.kz2601.metamon.renderer.theme.icons

import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.filled.ArrowBack
import androidx.compose.material.icons.automirrored.filled.ArrowForward
import androidx.compose.material.icons.automirrored.filled.KeyboardArrowLeft
import androidx.compose.material.icons.automirrored.filled.KeyboardArrowRight
import androidx.compose.material.icons.automirrored.filled.Send
import androidx.compose.material.icons.filled.AccountCircle
import androidx.compose.material.icons.filled.Add
import androidx.compose.material.icons.filled.AddCircle
import androidx.compose.material.icons.filled.Build
import androidx.compose.material.icons.filled.Call
import androidx.compose.material.icons.filled.Check
import androidx.compose.material.icons.filled.CheckCircle
import androidx.compose.material.icons.filled.Close
import androidx.compose.material.icons.filled.Delete
import androidx.compose.material.icons.filled.Edit
import androidx.compose.material.icons.filled.Favorite
import androidx.compose.material.icons.filled.FavoriteBorder
import androidx.compose.material.icons.filled.Home
import androidx.compose.material.icons.filled.Info
import androidx.compose.material.icons.filled.KeyboardArrowDown
import androidx.compose.material.icons.filled.KeyboardArrowUp
import androidx.compose.material.icons.filled.LocationOn
import androidx.compose.material.icons.filled.Lock
import androidx.compose.material.icons.filled.Notifications
import androidx.compose.material.icons.filled.Person
import androidx.compose.material.icons.filled.PlayArrow
import androidx.compose.material.icons.filled.Refresh
import androidx.compose.material.icons.filled.Search
import androidx.compose.material.icons.filled.Settings
import androidx.compose.material.icons.filled.ShoppingCart
import androidx.compose.material.icons.filled.Star
import androidx.compose.material.icons.filled.ThumbUp
import androidx.compose.material.icons.filled.Warning
import androidx.compose.ui.graphics.vector.ImageVector

/** SF Symbols の名前 → Material の名前(docs/dsl/icon-map.json の symbols)。 */
internal val symbolMaterialNames: Map<String, String> = mapOf(
    "a.circle" to "abc",
    "airplane" to "flight",
    "arrow.clockwise" to "refresh",
    "arrow.down.right" to "south_east",
    "arrow.left" to "arrow_back",
    "arrow.left.arrow.right" to "swap_horiz",
    "arrow.right" to "arrow_forward",
    "arrow.triangle.2.circlepath" to "sync",
    "arrow.up.right" to "north_east",
    "arrow.up.right.square" to "open_in_new",
    "arrow.uturn.left" to "undo",
    "at" to "alternate_email",
    "barometer" to "speed",
    "bell" to "notifications",
    "book" to "menu_book",
    "book.closed" to "book",
    "bookmark" to "bookmark",
    "bubble.left" to "chat_bubble",
    "bubble.left.and.bubble.right" to "forum",
    "building.columns" to "account_balance",
    "calendar" to "calendar_month",
    "calendar.badge.checkmark" to "event_available",
    "calendar.badge.clock" to "event",
    "calendar.badge.plus" to "edit_calendar",
    "camera" to "photo_camera",
    "car" to "directions_car",
    "cart" to "shopping_cart",
    "chart.bar" to "bar_chart",
    "chart.pie" to "pie_chart",
    "check" to "check",
    "checklist" to "checklist",
    "checkmark" to "check",
    "checkmark.circle" to "check_circle",
    "chevron.down" to "keyboard_arrow_down",
    "chevron.left" to "keyboard_arrow_left",
    "chevron.right" to "keyboard_arrow_right",
    "chevron.up" to "keyboard_arrow_up",
    "clock" to "schedule",
    "cloud" to "cloud",
    "cloud.rain" to "umbrella",
    "cloud.snow" to "ac_unit",
    "cloud.sun" to "wb_cloudy",
    "cup.and.saucer" to "coffee",
    "doc" to "description",
    "doc.text" to "description",
    "doc.text.viewfinder" to "document_scanner",
    "dollarsign" to "attach_money",
    "drop" to "water_drop",
    "dumbbell" to "fitness_center",
    "ellipsis" to "more_horiz",
    "envelope" to "mail",
    "exclamationmark.circle" to "error",
    "exclamationmark.triangle" to "warning",
    "eye" to "visibility",
    "face.smiling" to "mood",
    "figure.run" to "directions_run",
    "figure.strengthtraining.functional" to "sports_gymnastics",
    "figure.strengthtraining.traditional" to "fitness_center",
    "figure.walk" to "directions_walk",
    "flame" to "local_fire_department",
    "fork.knife" to "restaurant",
    "gamecontroller" to "sports_esports",
    "gauge.with.dots.needle.33percent" to "speed",
    "gear" to "settings",
    "gearshape" to "settings",
    "globe" to "public",
    "graduationcap" to "school",
    "gyroscope" to "screen_rotation",
    "hand.raised" to "pan_tool",
    "hand.thumbsup" to "thumb_up",
    "heart" to "favorite_border",
    "heart.fill" to "favorite",
    "heart.text.square" to "monitor_heart",
    "house" to "home",
    "info" to "info",
    "iphone" to "smartphone",
    "level" to "straighten",
    "link" to "link",
    "list.bullet" to "format_list_bulleted",
    "list.bullet.rectangle" to "list_alt",
    "location" to "near_me",
    "lock" to "lock",
    "magnifyingglass" to "search",
    "map" to "map",
    "mappin" to "location_on",
    "mappin.and.ellipse" to "location_on",
    "mic" to "mic",
    "mountain.2" to "terrain",
    "newspaper" to "newspaper",
    "note.text" to "notes",
    "number" to "tag",
    "paperplane" to "send",
    "pencil" to "edit",
    "pencil.line" to "edit_note",
    "person" to "person",
    "person.2" to "group",
    "person.2.wave.2" to "connect_without_contact",
    "person.3" to "groups",
    "person.badge.minus" to "person_remove",
    "person.badge.plus" to "person_add",
    "person.crop.circle" to "account_circle",
    "phone" to "call",
    "photo" to "image",
    "pills" to "medication",
    "pin" to "push_pin",
    "play" to "play_arrow",
    "plus" to "add",
    "plus.circle" to "add_circle",
    "point.bottomleft.forward.to.point.topright.scurvepath" to "route",
    "questionmark" to "help",
    "questionmark.bubble" to "contact_support",
    "rectangle.3.group" to "dashboard",
    "rectangle.bottomhalf.inset.filled" to "call_to_action",
    "rectangle.on.rectangle.angled" to "filter_none",
    "refrigerator" to "kitchen",
    "repeat" to "repeat",
    "safari" to "explore",
    "scalemass" to "scale",
    "slider.horizontal.3" to "tune",
    "sparkles" to "auto_awesome",
    "square.and.arrow.up" to "ios_share",
    "square.and.pencil" to "edit_note",
    "square.grid.2x2" to "grid_view",
    "star" to "star",
    "sun.max" to "wb_sunny",
    "tablecells" to "table_chart",
    "target" to "track_changes",
    "text.book.closed" to "book",
    "text.bubble" to "chat",
    "textformat" to "text_fields",
    "timer" to "timer",
    "trash" to "delete",
    "tray" to "inbox",
    "trophy" to "emoji_events",
    "tropicalstorm" to "warning",
    "video" to "videocam",
    "waveform" to "graphic_eq",
    "wifi.slash" to "wifi_off",
    "wrench" to "build",
    "xmark" to "close",
    "yensign" to "currency_yen",
)

/** Material の名前 → 絵。表に出る名前だけ。 */
internal fun materialVector(name: String): ImageVector? = when (name) {
    "abc" -> Icons.Filled.Abc
    "ac_unit" -> Icons.Filled.AcUnit
    "account_balance" -> Icons.Filled.AccountBalance
    "account_circle" -> Icons.Filled.AccountCircle
    "add" -> Icons.Filled.Add
    "add_circle" -> Icons.Filled.AddCircle
    "alternate_email" -> Icons.Filled.AlternateEmail
    "arrow_back" -> Icons.AutoMirrored.Filled.ArrowBack
    "arrow_forward" -> Icons.AutoMirrored.Filled.ArrowForward
    "attach_money" -> Icons.Filled.AttachMoney
    "auto_awesome" -> Icons.Filled.AutoAwesome
    "bar_chart" -> Icons.Filled.BarChart
    "book" -> Icons.Filled.Book
    "bookmark" -> Icons.Filled.Bookmark
    "build" -> Icons.Filled.Build
    "calendar_month" -> Icons.Filled.CalendarMonth
    "call" -> Icons.Filled.Call
    "call_to_action" -> Icons.Filled.CallToAction
    "chat" -> Icons.AutoMirrored.Filled.Chat
    "chat_bubble" -> Icons.Filled.ChatBubble
    "check" -> Icons.Filled.Check
    "check_circle" -> Icons.Filled.CheckCircle
    "checklist" -> Icons.Filled.Checklist
    "close" -> Icons.Filled.Close
    "cloud" -> Icons.Filled.Cloud
    "coffee" -> Icons.Filled.Coffee
    "connect_without_contact" -> Icons.Filled.ConnectWithoutContact
    "contact_support" -> Icons.AutoMirrored.Filled.ContactSupport
    "currency_yen" -> Icons.Filled.CurrencyYen
    "dashboard" -> Icons.Filled.Dashboard
    "delete" -> Icons.Filled.Delete
    "description" -> Icons.Filled.Description
    "directions_car" -> Icons.Filled.DirectionsCar
    "directions_run" -> Icons.AutoMirrored.Filled.DirectionsRun
    "directions_walk" -> Icons.AutoMirrored.Filled.DirectionsWalk
    "document_scanner" -> Icons.Filled.DocumentScanner
    "edit" -> Icons.Filled.Edit
    "edit_calendar" -> Icons.Filled.EditCalendar
    "edit_note" -> Icons.Filled.EditNote
    "emoji_events" -> Icons.Filled.EmojiEvents
    "error" -> Icons.Filled.Error
    "event" -> Icons.Filled.Event
    "event_available" -> Icons.Filled.EventAvailable
    "explore" -> Icons.Filled.Explore
    "favorite" -> Icons.Filled.Favorite
    "favorite_border" -> Icons.Filled.FavoriteBorder
    "filter_none" -> Icons.Filled.FilterNone
    "fitness_center" -> Icons.Filled.FitnessCenter
    "flight" -> Icons.Filled.Flight
    "format_list_bulleted" -> Icons.AutoMirrored.Filled.FormatListBulleted
    "forum" -> Icons.Filled.Forum
    "graphic_eq" -> Icons.Filled.GraphicEq
    "grid_view" -> Icons.Filled.GridView
    "group" -> Icons.Filled.Group
    "groups" -> Icons.Filled.Groups
    "help" -> Icons.AutoMirrored.Filled.Help
    "home" -> Icons.Filled.Home
    "image" -> Icons.Filled.Image
    "inbox" -> Icons.Filled.Inbox
    "info" -> Icons.Filled.Info
    "ios_share" -> Icons.Filled.IosShare
    "keyboard_arrow_down" -> Icons.Filled.KeyboardArrowDown
    "keyboard_arrow_left" -> Icons.AutoMirrored.Filled.KeyboardArrowLeft
    "keyboard_arrow_right" -> Icons.AutoMirrored.Filled.KeyboardArrowRight
    "keyboard_arrow_up" -> Icons.Filled.KeyboardArrowUp
    "kitchen" -> Icons.Filled.Kitchen
    "link" -> Icons.Filled.Link
    "list_alt" -> Icons.AutoMirrored.Filled.ListAlt
    "local_fire_department" -> Icons.Filled.LocalFireDepartment
    "location_on" -> Icons.Filled.LocationOn
    "lock" -> Icons.Filled.Lock
    "mail" -> Icons.Filled.Mail
    "map" -> Icons.Filled.Map
    "medication" -> Icons.Filled.Medication
    "menu_book" -> Icons.AutoMirrored.Filled.MenuBook
    "mic" -> Icons.Filled.Mic
    "monitor_heart" -> Icons.Filled.MonitorHeart
    "mood" -> Icons.Filled.Mood
    "more_horiz" -> Icons.Filled.MoreHoriz
    "near_me" -> Icons.Filled.NearMe
    "newspaper" -> Icons.Filled.Newspaper
    "north_east" -> Icons.Filled.NorthEast
    "notes" -> Icons.AutoMirrored.Filled.Notes
    "notifications" -> Icons.Filled.Notifications
    "open_in_new" -> Icons.AutoMirrored.Filled.OpenInNew
    "pan_tool" -> Icons.Filled.PanTool
    "person" -> Icons.Filled.Person
    "person_add" -> Icons.Filled.PersonAdd
    "person_remove" -> Icons.Filled.PersonRemove
    "photo_camera" -> Icons.Filled.PhotoCamera
    "pie_chart" -> Icons.Filled.PieChart
    "play_arrow" -> Icons.Filled.PlayArrow
    "public" -> Icons.Filled.Public
    "push_pin" -> Icons.Filled.PushPin
    "refresh" -> Icons.Filled.Refresh
    "repeat" -> Icons.Filled.Repeat
    "restaurant" -> Icons.Filled.Restaurant
    "route" -> Icons.Filled.Route
    "scale" -> Icons.Filled.Scale
    "schedule" -> Icons.Filled.Schedule
    "school" -> Icons.Filled.School
    "screen_rotation" -> Icons.Filled.ScreenRotation
    "search" -> Icons.Filled.Search
    "send" -> Icons.AutoMirrored.Filled.Send
    "settings" -> Icons.Filled.Settings
    "shopping_cart" -> Icons.Filled.ShoppingCart
    "smartphone" -> Icons.Filled.Smartphone
    "south_east" -> Icons.Filled.SouthEast
    "speed" -> Icons.Filled.Speed
    "sports_esports" -> Icons.Filled.SportsEsports
    "sports_gymnastics" -> Icons.Filled.SportsGymnastics
    "star" -> Icons.Filled.Star
    "straighten" -> Icons.Filled.Straighten
    "swap_horiz" -> Icons.Filled.SwapHoriz
    "sync" -> Icons.Filled.Sync
    "table_chart" -> Icons.Filled.TableChart
    "tag" -> Icons.Filled.Tag
    "terrain" -> Icons.Filled.Terrain
    "text_fields" -> Icons.Filled.TextFields
    "thumb_up" -> Icons.Filled.ThumbUp
    "timer" -> Icons.Filled.Timer
    "track_changes" -> Icons.Filled.TrackChanges
    "tune" -> Icons.Filled.Tune
    "umbrella" -> Icons.Filled.Umbrella
    "undo" -> Icons.AutoMirrored.Filled.Undo
    "videocam" -> Icons.Filled.Videocam
    "visibility" -> Icons.Filled.Visibility
    "warning" -> Icons.Filled.Warning
    "water_drop" -> Icons.Filled.WaterDrop
    "wb_cloudy" -> Icons.Filled.WbCloudy
    "wb_sunny" -> Icons.Filled.WbSunny
    "wifi_off" -> Icons.Filled.WifiOff
    else -> null
}

/** [materialVector] が引ける名前(テスト用)。 */
internal val materialVectorNames: Set<String> = setOf(
    "abc",
    "ac_unit",
    "account_balance",
    "account_circle",
    "add",
    "add_circle",
    "alternate_email",
    "arrow_back",
    "arrow_forward",
    "attach_money",
    "auto_awesome",
    "bar_chart",
    "book",
    "bookmark",
    "build",
    "calendar_month",
    "call",
    "call_to_action",
    "chat",
    "chat_bubble",
    "check",
    "check_circle",
    "checklist",
    "close",
    "cloud",
    "coffee",
    "connect_without_contact",
    "contact_support",
    "currency_yen",
    "dashboard",
    "delete",
    "description",
    "directions_car",
    "directions_run",
    "directions_walk",
    "document_scanner",
    "edit",
    "edit_calendar",
    "edit_note",
    "emoji_events",
    "error",
    "event",
    "event_available",
    "explore",
    "favorite",
    "favorite_border",
    "filter_none",
    "fitness_center",
    "flight",
    "format_list_bulleted",
    "forum",
    "graphic_eq",
    "grid_view",
    "group",
    "groups",
    "help",
    "home",
    "image",
    "inbox",
    "info",
    "ios_share",
    "keyboard_arrow_down",
    "keyboard_arrow_left",
    "keyboard_arrow_right",
    "keyboard_arrow_up",
    "kitchen",
    "link",
    "list_alt",
    "local_fire_department",
    "location_on",
    "lock",
    "mail",
    "map",
    "medication",
    "menu_book",
    "mic",
    "monitor_heart",
    "mood",
    "more_horiz",
    "near_me",
    "newspaper",
    "north_east",
    "notes",
    "notifications",
    "open_in_new",
    "pan_tool",
    "person",
    "person_add",
    "person_remove",
    "photo_camera",
    "pie_chart",
    "play_arrow",
    "public",
    "push_pin",
    "refresh",
    "repeat",
    "restaurant",
    "route",
    "scale",
    "schedule",
    "school",
    "screen_rotation",
    "search",
    "send",
    "settings",
    "shopping_cart",
    "smartphone",
    "south_east",
    "speed",
    "sports_esports",
    "sports_gymnastics",
    "star",
    "straighten",
    "swap_horiz",
    "sync",
    "table_chart",
    "tag",
    "terrain",
    "text_fields",
    "thumb_up",
    "timer",
    "track_changes",
    "tune",
    "umbrella",
    "undo",
    "videocam",
    "visibility",
    "warning",
    "water_drop",
    "wb_cloudy",
    "wb_sunny",
    "wifi_off",
)
