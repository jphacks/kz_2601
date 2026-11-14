package jp.kz2601.metamon.dsl.records

import java.io.File
import java.util.UUID

/**
 * video field の値(iOS の `VideoValue`・#206)。動画本体は撮った端末のアプリ領域にだけ置き、記録には
 * `"video:<id>|data:image/jpeg;base64,<1 フレーム目のサムネイル>"` だけを入れる。`https://` は将来の保存先用(今は再生だけ)。
 */
data class VideoValue(
    /** 端末内のファイル id([LocalVideoStore] のファイル名)。https の時は null */
    val id: String?,
    /** サムネイル(data URL) */
    val thumbnail: String? = null,
    /** https の動画 URL */
    val remoteUrl: String? = null,
) {
    /** 記録に保存する文字列。 */
    val encoded: String
        get() = remoteUrl ?: (PREFIX + id.orEmpty() + (thumbnail?.let { "|$it" } ?: ""))

    companion object {
        const val PREFIX = "video:"

        fun of(value: JSONValue?): VideoValue? = value?.stringValue?.let(::parse)

        fun parse(s: String): VideoValue? {
            if (s.startsWith("https://")) return VideoValue(id = null, remoteUrl = s)
            if (!s.startsWith(PREFIX)) return null
            val parts = s.removePrefix(PREFIX).split("|", limit = 2)
            val id = parts[0]
            if (!LocalVideoStore.isValidId(id)) return null
            val thumb = parts.getOrNull(1)?.takeIf { it.startsWith("data:image/") }
            return VideoValue(id, thumb)
        }

        /** 記録の値の中にある端末内動画の id(記録を消す時に一緒にファイルを消す)。 */
        fun localIds(data: Map<String, JSONValue>): Set<String> = data.values.mapNotNull { of(it)?.id }.toSet()
    }
}

/**
 * 端末内の動画の置き場(iOS の `LocalVideoStore`)。`<directory>/<id>.mp4`。
 * :app ではバックアップされない領域(noBackupFilesDir)に置く。アプリを消すと一緒に消える。
 */
class LocalVideoStore(val directory: File) {
    fun file(id: String): File? = if (isValidId(id)) File(directory, "$id.$FILE_EXTENSION") else null

    fun exists(id: String): Boolean = file(id)?.isFile == true

    /** [source] を新しい id で取り込む(rename。できなければ copy して元を消す)。 */
    fun importFile(source: File, id: String = UUID.randomUUID().toString()): String {
        val dest = requireNotNull(file(id)) { "不正な id: $id" }
        directory.mkdirs()
        dest.delete()
        if (!source.renameTo(dest)) {
            source.copyTo(dest, overwrite = true)
            source.delete()
        }
        return id
    }

    fun remove(ids: Iterable<String>) {
        ids.forEach { id -> file(id)?.delete() }
    }

    fun removeAll() {
        directory.deleteRecursively()
    }

    companion object {
        const val FILE_EXTENSION = "mp4"
        const val DIRECTORY_NAME = "metamon-videos"

        /** id は英数・-・_ だけ(パスを組み立てるので `..` や `/` を通さない)・80 文字まで。 */
        fun isValidId(id: String): Boolean =
            id.isNotEmpty() && id.length <= 80 && id.all { (it.code < 0x80 && it.isLetterOrDigit()) || it == '-' || it == '_' }
    }
}
