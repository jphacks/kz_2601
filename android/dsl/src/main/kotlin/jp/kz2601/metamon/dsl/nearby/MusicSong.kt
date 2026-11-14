package jp.kz2601.metamon.dsl.nearby

import jp.kz2601.metamon.dsl.AppDSL
import jp.kz2601.metamon.dsl.Component
import jp.kz2601.metamon.dsl.ComponentType
import jp.kz2601.metamon.dsl.logic.graphemePrefix
import kotlinx.serialization.json.JsonElement
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive
import java.net.URI

/**
 * `musicPlayer`(#412)の 1 曲。近くの端末から届くので、使う前に必ず [MusicRequestRules.sanitize] を通す。
 * iOS の `MusicSong`(JSON のキーも同じ)。ここでは近距離共有で運ぶのに要る分だけ。
 */
data class MusicSong(
    val id: String? = null,
    val title: String,
    val artist: String,
    val artworkURL: String? = null,
) {
    fun toJson(): JsonObject = JsonObject(
        buildMap {
            id?.let { put("id", JsonPrimitive(it)) }
            put("title", JsonPrimitive(title))
            put("artist", JsonPrimitive(artist))
            artworkURL?.let { put("artworkURL", JsonPrimitive(it)) }
        },
    )

    companion object {
        fun from(e: JsonElement?): MusicSong {
            val o = NearbyJson.obj(e, "MusicSong")
            return MusicSong(
                id = NearbyJson.optStr(o, "id"),
                title = NearbyJson.str(o, "title"),
                artist = NearbyJson.str(o, "artist"),
                artworkURL = NearbyJson.optStr(o, "artworkURL"),
            )
        }
    }
}

/** 届いたリクエストの上限と検証(iOS の `MusicRequestRules` と同じ値)。 */
object MusicRequestRules {
    const val MAX_TITLE = 100
    const val MAX_ARTIST = 100
    const val MAX_ID = 40
    const val MAX_ARTWORK_URL = 512

    /** 前後の空白を落として長さを切る。曲名が空なら null。id は英数字とドットだけ、ジャケットは https だけ残す。 */
    fun sanitize(song: MusicSong): MusicSong? {
        val title = clip(song.title, MAX_TITLE)
        if (title.isEmpty()) return null
        val id = song.id?.trim { it == ' ' || it == '\t' }?.takeIf { s ->
            s.isNotEmpty() && s.length <= MAX_ID && s.all { it in 'a'..'z' || it in 'A'..'Z' || it in '0'..'9' || it == '.' }
        }
        val artwork = song.artworkURL?.takeIf { s ->
            s.length <= MAX_ARTWORK_URL && runCatching { URI(s) }.getOrNull()?.let { it.scheme == "https" && it.host != null } == true
        }
        return MusicSong(id = id, title = title, artist = clip(song.artist, MAX_ARTIST), artworkURL = artwork)
    }

    /** 前後の空白を落とし、先頭 `max` 文字(書記素クラスタ・Swift の `prefix`)に切る。 */
    fun clip(s: String, max: Int): String {
        val t = s.trim()
        if (t.length <= max) return t
        return t.graphemePrefix(max)
    }
}

/** 最初の `musicPlayer`(container の子も見る)。iOS の `AppDSL.musicPlayer`。 */
val AppDSL.musicPlayer: Component?
    get() {
        for (s in screens) {
            for (c in s.components) {
                if (c.type == ComponentType.MUSIC_PLAYER) return c
                c.children.firstOrNull { it.type == ComponentType.MUSIC_PLAYER }?.let { return it }
            }
        }
        return null
    }

/** 近くの端末から曲のリクエストを受けるか。 */
val AppDSL.usesMusicRequests: Boolean get() = musicPlayer?.allowRequests == true
