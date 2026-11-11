package jp.kz2601.metamon.renderer.components.media

import android.content.Context
import android.media.MediaCodec
import android.media.MediaExtractor
import android.media.MediaFormat
import android.media.MediaMetadataRetriever
import android.media.MediaMuxer
import android.net.Uri
import android.os.Handler
import android.os.Looper
import androidx.media3.common.MediaItem
import androidx.media3.common.MimeTypes
import androidx.media3.common.util.UnstableApi
import androidx.media3.effect.Presentation
import androidx.media3.transformer.Composition
import androidx.media3.transformer.DefaultEncoderFactory
import androidx.media3.transformer.EditedMediaItem
import androidx.media3.transformer.Effects
import androidx.media3.transformer.ExportException
import androidx.media3.transformer.ExportResult
import androidx.media3.transformer.Transformer
import androidx.media3.transformer.VideoEncoderSettings
import jp.kz2601.metamon.dsl.records.LocalVideoStore
import jp.kz2601.metamon.dsl.records.VideoValue
import java.io.File
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.suspendCancellableCoroutine
import java.nio.ByteBuffer
import kotlin.coroutines.resume
import kotlin.coroutines.resumeWithException
import kotlin.math.max
import kotlin.math.min

/**
 * 撮った・選んだ動画を端末内に保存し、video field の値を作る(iOS の `VideoCodec`)。
 * - 中くらいの画質(iOS の `AVAssetExportPresetMediumQuality` に合わせ、短辺 480・映像 1 Mbps の H.264)の mp4 に
 *   書き直し、先頭 `maxDuration` 秒で切る(#1202)。もともと小さい動画は書き直さない
 * - 書き直せなければ mp4 に詰め直す(再エンコードしない)。詰め直せない形式は上限内ならそのまま写す
 * - 1 フレーム目を 320px・品質 50 の data URL にしてサムネイルにする
 * 計算だけの部分は android.* に触れない(JVM テスト)。
 */
object VideoCodec {
    const val DEFAULT_MAX_DURATION = 60
    const val THUMBNAIL_MAX_SIDE = 320

    /** 書き直しの短辺(px)と映像のビットレート(bps)。iOS の MediumQuality と同じくらいの大きさになる値。 */
    const val TARGET_SHORT_SIDE = 480
    const val TARGET_VIDEO_BITRATE = 1_000_000

    /** 書き直し方。[shortSide] が null なら縮めない。 */
    data class EncodePlan(val shortSide: Int?, val videoBitrate: Int)

    /**
     * 元の大きさ・ビットレートから書き直し方を決める。null なら書き直さない(短辺も
     * ビットレートも目標以下と分かっている時)。大きさが分からなければ縮めずにビットレートだけ下げる。
     */
    fun encodePlan(width: Int?, height: Int?, sourceBitrate: Int?): EncodePlan? {
        val shortSide = if (width != null && height != null && width > 0 && height > 0) min(width, height) else null
        val bitrate = sourceBitrate?.takeIf { it > 0 }
        val small = shortSide != null && shortSide <= TARGET_SHORT_SIDE
        if (small && bitrate != null && bitrate <= TARGET_VIDEO_BITRATE) return null
        return EncodePlan(
            shortSide = if (shortSide != null && !small) TARGET_SHORT_SIDE else null,
            videoBitrate = bitrate?.let { min(it, TARGET_VIDEO_BITRATE) } ?: TARGET_VIDEO_BITRATE,
        )
    }

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

    /** 動画 → 値。重いので IO のスレッドで呼ぶ。失敗したら例外。取り消すと書き直しも止める。 */
    suspend fun ingest(context: Context, source: Uri, maxDuration: Int?, store: LocalVideoStore = store(context)): VideoValue {
        val tmp = tempFile(context, "ingest")
        try {
            val limitUs = limitMicros(maxDuration)
            val info = sourceInfo(context, source)
            val plan = encodePlan(info?.first, info?.second, info?.third)
            val encoded = plan != null && try {
                transcode(context, source, tmp, plan, limitUs / 1_000)
                true
            } catch (e: CancellationException) {
                throw e
            } catch (_: Exception) {
                tmp.delete()
                false
            }
            val remuxed = encoded || runCatching { remux(context, source, tmp, limitUs) }.isSuccess
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

    /** Media3 Transformer で H.264 / [plan] の大きさ・ビットレートに書き直す。Transformer はメインの Looper で動かす。 */
    @androidx.annotation.OptIn(UnstableApi::class)
    private suspend fun transcode(context: Context, source: Uri, dest: File, plan: EncodePlan, limitMs: Long) {
        val main = Handler(Looper.getMainLooper())
        suspendCancellableCoroutine { cont ->
            var transformer: Transformer? = null
            main.post {
                if (!cont.isActive) return@post
                val encoder = DefaultEncoderFactory.Builder(context)
                    .setRequestedVideoEncoderSettings(VideoEncoderSettings.Builder().setBitrate(plan.videoBitrate).build())
                    .setEnableFallback(true)
                    .build()
                val t = Transformer.Builder(context)
                    .setLooper(Looper.getMainLooper())
                    .setVideoMimeType(MimeTypes.VIDEO_H264)
                    .setEncoderFactory(encoder)
                    .addListener(object : Transformer.Listener {
                        override fun onCompleted(composition: Composition, exportResult: ExportResult) {
                            if (cont.isActive) cont.resume(Unit)
                        }

                        override fun onError(composition: Composition, exportResult: ExportResult, exportException: ExportException) {
                            if (cont.isActive) cont.resumeWithException(exportException)
                        }
                    })
                    .build()
                transformer = t
                val item = MediaItem.Builder()
                    .setUri(source)
                    .setClippingConfiguration(MediaItem.ClippingConfiguration.Builder().setEndPositionMs(limitMs).build())
                    .build()
                val effects = plan.shortSide?.let { Effects(emptyList(), listOf(Presentation.createForShortSide(it))) } ?: Effects.EMPTY
                t.start(EditedMediaItem.Builder(item).setEffects(effects).build(), dest.absolutePath)
            }
            cont.invokeOnCancellation { main.post { transformer?.cancel() } }
        }
    }

    /** 元の動画の幅・高さ・ビットレート。読めなければ null。 */
    private fun sourceInfo(context: Context, source: Uri): Triple<Int?, Int?, Int?>? = runCatching {
        MediaMetadataRetriever().use { r ->
            r.setDataSource(context, source)
            Triple(
                r.extractMetadata(MediaMetadataRetriever.METADATA_KEY_VIDEO_WIDTH)?.toIntOrNull(),
                r.extractMetadata(MediaMetadataRetriever.METADATA_KEY_VIDEO_HEIGHT)?.toIntOrNull(),
                r.extractMetadata(MediaMetadataRetriever.METADATA_KEY_BITRATE)?.toIntOrNull(),
            )
        }
    }.getOrNull()

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
