package jp.kz2601.metamon.account

import java.io.File

/**
 * 撮影・録音の一時ファイル(cache の下)。値にしたら消すが、途中でプロセスが落ちると残るので後片付け(#841)でまとめて消す。
 * 置き場は PhotoFieldLeaf(metamon_photos)・VideoCodec(metamon_videos)・LocalAudioStore(metamon-rec-*)と同じ。
 */
object MediaTempFiles {
    private val directories = listOf("metamon_photos", "metamon_videos")
    private const val RECORDING_PREFIX = "metamon-rec-"

    fun removeAll(cacheDir: File) {
        directories.forEach { File(cacheDir, it).deleteRecursively() }
        cacheDir.listFiles { f -> f.isFile && f.name.startsWith(RECORDING_PREFIX) }?.forEach { it.delete() }
    }
}
