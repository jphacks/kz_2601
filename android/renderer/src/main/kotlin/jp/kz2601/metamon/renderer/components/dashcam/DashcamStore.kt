package jp.kz2601.metamon.renderer.components.dashcam

import java.io.File

/**
 * `dashcam` の端末内の置き場: `<base>/<アプリ名のハッシュ>/{loop,saved}/`(iOS の `DashcamStore`)。
 * 映像はレコードに入れない(同期しない)。base はアプリの内部領域(filesDir/metamon-dashcam・バックアップしない)。
 */
class DashcamStore(base: File, appName: String) {
    val root = File(base, DashcamStoreKey.make(appName))
    val loopDir = File(root, "loop")
    val savedDir = File(root, "saved")

    fun prepare() {
        loopDir.mkdirs()
        savedDir.mkdirs()
        // 落ちた等で閉じられなかった録画中のファイル(再生できない)を消す
        loopDir.listFiles()?.filter { it.name.startsWith("rec-") }?.forEach { it.delete() }
    }

    fun file(s: DashcamSegment): File = File(if (s.saved) savedDir else loopDir, s.fileName)

    fun recordingFile(start: Long): File = File(loopDir, DashcamSegment.recordingName(start))

    /** 両方の置き場の区間(古い順)。 */
    fun segments(): List<DashcamSegment> =
        listOf(loopDir to false, savedDir to true).flatMap { (dir, saved) ->
            dir.listFiles().orEmpty().mapNotNull { DashcamSegment.parse(it.name, it.length(), saved) }
        }.sortedBy { it.start }

    /** 録画中のファイルを区間として確定する(長さをファイル名に入れる)。空のファイルは捨てる。 */
    fun finalize(recording: File, start: Long, duration: Long, saved: Boolean): DashcamSegment? {
        if (!recording.isFile) return null
        if (recording.length() == 0L) {
            recording.delete()
            return null
        }
        val s = DashcamSegment(start, maxOf(0L, duration), recording.length(), saved)
        return if (recording.renameTo(file(s))) s else null.also { recording.delete() }
    }

    /** ループの区間を保存済みに移す。 */
    fun save(s: DashcamSegment): DashcamSegment? {
        if (s.saved) return s
        val moved = s.copy(saved = true)
        return if (file(s).renameTo(file(moved))) moved else null
    }

    fun delete(s: DashcamSegment) {
        file(s).delete()
    }

    /** 端末の空き容量(取れなければ null)。 */
    fun deviceFreeBytes(): Long? = root.usableSpace.takeIf { it > 0 }

    companion object {
        /** filesDir の下の置き場(FileProvider の metamon_dashcam と揃える) */
        const val DIRECTORY = "metamon-dashcam"

        /** 端末の空きがこれを切ったら録画を止める */
        const val MINIMUM_FREE_BYTES = 500_000_000L

        /** 録れない理由(null = 録れる)。true = 端末の空きが少ない / false = 保存済みで上限。 */
        fun storageProblem(segments: List<DashcamSegment>, freeBytes: Long?, maxBytes: Long): Boolean? {
            if (freeBytes != null && freeBytes < MINIMUM_FREE_BYTES) return true
            return if (DashcamLoop.isFull(segments, maxBytes, DashcamLoop.reserve(segments))) false else null
        }
    }
}
