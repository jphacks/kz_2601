package jp.kz2601.metamon.renderer.components.music

import jp.kz2601.metamon.dsl.Component
import jp.kz2601.metamon.dsl.logic.ExternalUrlPolicy

/** 1 曲(iOS の `MusicSong` の曲名・アーティストだけ)。 */
data class MusicSong(val title: String, val artist: String = "")

/** 検索を開く音楽サービス(#1164)。API キーは使わず、検索のページを開くだけ(アプリが入っていれば OS がアプリで開く)。 */
enum class MusicService { YOUTUBE_MUSIC, SPOTIFY }

/**
 * `musicPlayer` の代わりの表示の決まり(Compose に依らない・JVM で試す)。
 * 値の上限と Spotify の検索の URL は iOS の `MusicRequestRules` / `MusicLinks` と同じ。YouTube Music は Android だけ。
 */
object MusicSearch {
    const val MAX_TITLE = 100
    const val MAX_ARTIST = 100

    /** 一覧に残す件数(新しい順)。 */
    const val MAX_RECENT = 5

    /** 何も選んでいない時のサービス(iOS の `MusicLinks.spotifySearchURL` と同じ) */
    val DEFAULT_SERVICE = MusicService.SPOTIFY

    private const val SPOTIFY_BASE = "https://open.spotify.com/search/"
    private const val YOUTUBE_MUSIC_BASE = "https://music.youtube.com/search?q="
    private const val QUERY_KEEP = "-._~"
    private const val PATH_KEEP = "-._~!$&'()*+,;=:@"
    private const val HEX = "0123456789ABCDEF"

    /** 見出し(iOS と同じく `title` が無ければ既定の文言)。 */
    fun heading(c: Component, default: String): String = c.title ?: default

    /** 前後の空白を落として長さを切る。曲名が空なら null。 */
    fun sanitize(song: MusicSong): MusicSong? {
        val title = clip(song.title, MAX_TITLE)
        if (title.isEmpty()) return null
        return MusicSong(title, clip(song.artist, MAX_ARTIST))
    }

    /** 検索語(曲名 + アーティスト・空は除く)。 */
    fun searchTerm(song: MusicSong): String =
        listOf(song.title, song.artist).map { it.trim() }.filter { it.isNotEmpty() }.joinToString(" ")

    /** 検索のページ(音楽アプリが入っていれば OS がアプリで開く)。開いてよい URL([ExternalUrlPolicy])でなければ null。 */
    fun searchUrl(song: MusicSong, service: MusicService = DEFAULT_SERVICE): String? {
        val term = searchTerm(song)
        if (term.isEmpty()) return null
        val url = when (service) {
            MusicService.SPOTIFY -> SPOTIFY_BASE + encodePathSegment(term)
            MusicService.YOUTUBE_MUSIC -> YOUTUBE_MUSIC_BASE + encodeQueryValue(term)
        }
        return ExternalUrlPolicy.openable(url)
    }

    /** 探した曲を一覧の先頭に入れる(同じ曲は前に出すだけ・[MAX_RECENT] 件まで)。 */
    fun remember(recent: List<MusicSong>, song: MusicSong): List<MusicSong> =
        (listOf(song) + recent.filterNot { same(it, song) }).take(MAX_RECENT)

    /** 同じ曲か(大文字小文字・前後の空白を無視)。 */
    fun same(a: MusicSong, b: MusicSong): Boolean =
        norm(a.title) == norm(b.title) && norm(a.artist) == norm(b.artist)

    /**
     * パスの 1 区間として符号化(iOS の `urlPathAllowed` から `/?#` を除いた文字はそのまま、他は UTF-8 の %XX)。
     * 空白は `%20`(`+` にしない)。
     */
    fun encodePathSegment(s: String): String = percentEncode(s, PATH_KEEP)

    /** クエリの値として符号化(英数字と `-._~` だけそのまま。`&` `=` `+` も %XX・空白は `%20`)。 */
    fun encodeQueryValue(s: String): String = percentEncode(s, QUERY_KEEP)

    private fun percentEncode(s: String, keep: String): String = buildString {
        for (b in s.toByteArray(Charsets.UTF_8)) {
            val code = b.toInt() and 0xFF
            val ch = code.toChar()
            if (code < 0x80 && (ch.isLetterOrDigit() || ch in keep)) {
                append(ch)
            } else {
                append('%').append(HEX[code shr 4]).append(HEX[code and 0xF])
            }
        }
    }

    private fun clip(s: String, max: Int): String = s.trim().take(max)
    private fun norm(s: String): String = s.trim().lowercase()
}
