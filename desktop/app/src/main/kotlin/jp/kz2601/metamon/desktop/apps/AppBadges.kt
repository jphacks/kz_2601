package jp.kz2601.metamon.desktop.apps

import androidx.compose.foundation.border
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Modifier
import androidx.compose.ui.semantics.clearAndSetSemantics
import androidx.compose.ui.unit.dp
import jp.kz2601.metamon.desktop.i18n.Strings
import jp.kz2601.metamon.services.http.HttpRequest
import jp.kz2601.metamon.services.http.MetamonHttp
import kotlinx.coroutines.CancellationException
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.JsonArray
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive

/** アプリの一覧の行に付ける小さな印(#2504)。並びはこの順。 */
enum class AppBadge(val key: String) {
    PUBLISHED("desktop_app_badge_published"),
    SHARED("desktop_app_badge_shared"),
    REQUESTED("desktop_app_badge_requested"),
    ;

    val label: String get() = Strings[key]
}

/**
 * 印の元(appId の集まり)。null = 応答に無い(分からない)ので、その印は出さない。
 * - [published]: 「探す」に公開中
 * - [shared]: 共有のリンクを出している
 * - [requested]: リリースを申請中(見送り以外・`GET /v1/release-requests`)
 */
data class AppBadgeSource(
    val published: Set<String>? = null,
    val shared: Set<String>? = null,
    val requested: Set<String>? = null,
)

object AppBadges {
    private const val DECLINED = "declined"

    fun of(appId: String, source: AppBadgeSource): List<AppBadge> = buildList {
        if (source.published?.contains(appId) == true) add(AppBadge.PUBLISHED)
        if (source.shared?.contains(appId) == true) add(AppBadge.SHARED)
        if (source.requested?.contains(appId) == true) add(AppBadge.REQUESTED)
    }

    /**
     * `GET /v1/release-requests` の応答 → 申請中のアプリ。見送り(declined)だけのアプリは入れない。
     * 知らない status は受付の扱い(Android の `ReleaseStatus.of` と同じ)。形が違えば null(印を出さない)。
     */
    fun requestedApps(body: String): Set<String>? {
        val root = runCatching { Json.parseToJsonElement(body) }.getOrNull() as? JsonObject ?: return null
        val items = when (val raw = root["items"]) {
            null -> return emptySet()
            is JsonArray -> raw
            else -> return null
        }
        return items.mapNotNullTo(linkedSetOf()) { item ->
            val o = item as? JsonObject ?: return@mapNotNullTo null
            val appId = (o["appId"] as? JsonPrimitive)?.takeIf { it.isString }?.content?.takeIf { it.isNotEmpty() } ?: return@mapNotNullTo null
            val status = (o["status"] as? JsonPrimitive)?.content
            appId.takeIf { status != DECLINED }
        }
    }

    /** 申請中のアプリを読む。読めなければ null(前の印のまま・印を出さない)。 */
    suspend fun loadRequested(http: MetamonHttp): Set<String>? = try {
        requestedApps(http.send(HttpRequest("GET", listOf("v1", "release-requests"))))
    } catch (e: CancellationException) {
        throw e
    } catch (_: Exception) {
        null
    }

    /** 読み上げ: 名前のあとに印(印が無ければ名前だけ)。 */
    fun a11y(name: String, badges: List<AppBadge>): String =
        if (badges.isEmpty()) name else Strings.format("desktop_app_badges_a11y", name, badges.joinToString(Strings["desktop_app_badges_separator"]) { it.label })
}

/** 行の右に並べる印。読み上げは行の [AppBadges.a11y] に入れるので、ここは読ませない。 */
@Composable
fun AppBadgeChips(badges: List<AppBadge>, modifier: Modifier = Modifier) {
    if (badges.isEmpty()) return
    Row(modifier.clearAndSetSemantics { }, horizontalArrangement = Arrangement.spacedBy(4.dp)) {
        badges.forEach { b ->
            Text(
                b.label,
                style = MaterialTheme.typography.labelSmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
                modifier = Modifier.border(1.dp, MaterialTheme.colorScheme.outlineVariant, RoundedCornerShape(4.dp)).padding(horizontal = 4.dp, vertical = 1.dp),
            )
        }
    }
}
