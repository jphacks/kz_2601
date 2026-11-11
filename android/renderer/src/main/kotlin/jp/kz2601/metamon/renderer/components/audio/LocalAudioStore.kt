package jp.kz2601.metamon.renderer.components.audio

import android.content.Context
import java.io.File

/**
 * 端末内の録音ファイル置き場(iOS の `LocalAudioStore`: Application Support/metamon-audio/<id>.m4a)。
 * Android は noBackupFilesDir/metamon-audio(バックアップの対象外)。サーバーへは上げない。
 * レコード・アプリ・アカウントを消した時の後片付けは子 Issue([removeAll] / [remove] を呼ぶ口だけ置く)。
 */
class LocalAudioStore(val directory: File) {
    fun file(id: String): File? = if (AudioValue.isValidId(id)) File(directory, "$id.$EXTENSION") else null

    fun exists(id: String): Boolean = file(id)?.isFile == true

    /** 録音中の一時ファイル(止めて [importFile] するまでの置き場)。 */
    fun temporaryRecording(cacheDir: File, id: String = AudioValue.newId()): File = File(cacheDir, "metamon-rec-$id.$EXTENSION")

    /** [source] を新しい id で取り込む(rename、だめなら copy)。失敗したら null。 */
    fun importFile(source: File, id: String = AudioValue.newId()): String? {
        val dest = file(id) ?: return null
        return runCatching {
            directory.mkdirs()
            dest.delete()
            if (!source.renameTo(dest)) {
                source.copyTo(dest, overwrite = true)
                source.delete()
            }
            id
        }.getOrNull()
    }

    fun remove(ids: Iterable<String>) {
        ids.forEach { id -> file(id)?.delete() }
    }

    fun removeAll() {
        directory.deleteRecursively()
    }

    companion object {
        const val EXTENSION = "m4a"

        fun of(context: Context): LocalAudioStore = LocalAudioStore(File(context.noBackupFilesDir, "metamon-audio"))
    }
}
