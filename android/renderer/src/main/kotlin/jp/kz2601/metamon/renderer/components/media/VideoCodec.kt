package jp.kz2601.metamon.renderer.components.media

import android.content.Context
import android.media.MediaCodec
import android.media.MediaExtractor
import android.media.MediaFormat
import android.media.MediaMetadataRetriever
import android.media.MediaMuxer
import android.net.Uri
import jp.kz2601.metamon.dsl.records.LocalVideoStore
import jp.kz2601.metamon.dsl.records.VideoValue
import java.io.File
import java.nio.ByteBuffer
import kotlin.math.max

/**
 * 撮った・選んだ動画を端末内に保存し、video field の値を作る(iOS の `VideoCodec`)。
 * - mp4 に詰め直して(再エンコードはしない)先頭 `maxDuration` 秒で切る。詰め直せない形式は上限内ならそのまま写す
 * - 1 フレーム目を 320px・品質 50 の data URL にしてサムネイルにする
 * 計算だけの部分は android.* に触れない(JVM テスト)。
 */
object VideoCodec {
    const val DEFAULT_MAX_DURATION = 60
    const val THUMBNAIL_MAX_SIDE = 320

    /** 長さの上限(秒)。省略は 60 秒・1 秒未満は 1 秒(iOS と同じ)。 */
    fun limitSeconds(maxDuration: Int?): Int = max(1, maxDuration ?: DEFAULT_MAX_DURATION)

    fun limitMicros(maxDuration: Int?): Long = limitSeconds(maxDuration) * 1_000_000L

    /** 詰め直す時にこの sample を残すか(上限ちょうどまでは残す)。 */
    fun keepsSample(sampleTimeUs: Long, limitUs: Long): Boolean = sampleTimeUs in 0..limitUs

    /** 詰め直せなかった時、そのまま写してよいか(長さが分かって上限内の時だけ)。 */
    fun canCopyAsIs(durationMs: Long?, maxDuration: Int?): Boolean =
        durationMs != null && durationMs > 0 && durationMs * 1_000 <= limitMicros(maxDuration)

    /** 端末内の置き場(バックアップされない領域)。 */
    fun store(context: Context): LocalVideoStore = LocalVideoStore(File(context.noBackupFilesDir, LocalVideoStore.DIRECTORY_NAME))

    /** 作業用の一時ファイル(cache/metamon_videos・FileProvider でカメラにも渡す)。 */
    fun tempFile(context: Context, prefix: String): File =
        File(context.cacheDir, "metamon_videos").apply { mkdirs() }.let { File(it, "$prefix-${System.currentTimeMillis()}.mp4") }

    /** 動画 → 値。重いので IO のスレッドで呼ぶ。失敗したら例外。 */
    fun ingest(context: Context, source: Uri, maxDuration: Int?, store: LocalVideoStore = store(context)): VideoValue {
        val tmp = tempFile(context, "ingest")
        try {
            val remuxed = runCatching { remux(context, source, tmp, limitMicros(maxDuration)) }.isSuccess
            if (!remuxed) {
                tmp.delete()
                check(canCopyAsIs(durationMs(context, source), maxDuration)) { "詰め直せず、上限を超えているか長さが分からない" }
                context.contentResolver.openInputStream(source)?.use { input -> tmp.outputStream().use { input.copyTo(it) } }
                    ?: error("読めない: $source")
            }
            val id = store.importFile(tmp)
            val file = requireNotNull(store.file(id))
            return VideoValue(id, thumbnail(file))
        } finally {
            tmp.delete()
        }
    }

    /** 映像と音声の track を mp4 に詰め直し、[limitUs] より後の sample を捨てる。 */
    private fun remux(context: Context, source: Uri, dest: File, limitUs: Long) {
        val extractor = MediaExtractor()
        var muxer: MediaMuxer? = null
        try {
            extractor.setDataSource(context, source, null)
            val m = MediaMuxer(dest.absolutePath, MediaMuxer.OutputFormat.MUXER_OUTPUT_MPEG_4)
            muxer = m
            val map = HashMap<Int, Int>()
            var bufferSize = 1 shl 20
            for (i in 0 until extractor.trackCount) {
                val format = extractor.getTrackFormat(i)
                val mime = format.getString(MediaFormat.KEY_MIME) ?: continue
                if (!mime.startsWith("video/") && !mime.startsWith("audio/")) continue
                if (mime.startsWith("video/") && format.containsKey(MediaFormat.KEY_ROTATION)) {
                    m.setOrientationHint(format.getInteger(MediaFormat.KEY_ROTATION))
                }
                if (format.containsKey(MediaFormat.KEY_MAX_INPUT_SIZE)) bufferSize = max(bufferSize, format.getInteger(MediaFormat.KEY_MAX_INPUT_SIZE))
                map[i] = m.addTrack(format)
                extractor.selectTrack(i)
            }
            check(map.isNotEmpty()) { "映像も音声も無い" }
            m.start()
            val buffer = ByteBuffer.allocate(bufferSize)
            val info = MediaCodec.BufferInfo()
            while (true) {
                val track = extractor.sampleTrackIndex
                if (track < 0) break
                val time = extractor.sampleTime
                if (!keepsSample(time, limitUs)) {
                    extractor.unselectTrack(track)
                    continue
                }
                info.offset = 0
                info.size = extractor.readSampleData(buffer, 0)
                if (info.size < 0) break
                info.presentationTimeUs = time
                info.flags = if (extractor.sampleFlags and MediaExtractor.SAMPLE_FLAG_SYNC != 0) MediaCodec.BUFFER_FLAG_KEY_FRAME else 0
                m.writeSampleData(map.getValue(track), buffer, info)
                extractor.advance()
            }
            m.stop()
        } finally {
            runCatching { muxer?.release() }
            extractor.release()
        }
    }

    private fun durationMs(context: Context, source: Uri): Long? = runCatching {
        MediaMetadataRetriever().use { r ->
            r.setDataSource(context, source)
            r.extractMetadata(MediaMetadataRetriever.METADATA_KEY_DURATION)?.toLongOrNull()
        }
    }.getOrNull()

    /** 1 フレーム目の縮小 JPEG(data URL)。取れなければ null(サムネイル無しで保存する)。 */
    fun thumbnail(file: File): String? = runCatching {
        MediaMetadataRetriever().use { r ->
            r.setDataSource(file.absolutePath)
            val frame = r.getScaledFrameAtTime(0, MediaMetadataRetriever.OPTION_CLOSEST_SYNC, THUMBNAIL_MAX_SIDE, THUMBNAIL_MAX_SIDE)
                ?: return@use null
            // 回転は retriever が反映済み
            PhotoCodec.dataUrl(frame, maxSide = THUMBNAIL_MAX_SIDE, quality = PhotoCodec.QUALITY)
        }
    }.getOrNull()
}
