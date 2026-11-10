package jp.kz2601.metamon.renderer.components

import android.content.Context
import coil3.ImageLoader
import coil3.SingletonImageLoader
import coil3.disk.DiskCache
import coil3.memory.MemoryCache
import coil3.network.okhttp.OkHttpNetworkFetcherFactory
import coil3.request.crossfade
import okhttp3.Interceptor
import okhttp3.OkHttpClient
import okhttp3.Response
import okhttp3.ResponseBody.Companion.asResponseBody
import okio.Buffer
import okio.ForwardingSource
import okio.Path.Companion.toOkioPath
import okio.Source
import okio.buffer
import java.io.File
import java.io.IOException
import java.net.URI
import java.time.Duration

/**
 * https の画像の読み込み(iOS の `AsyncImage` + URLCache)。image / avatar / photoField の写真で使う。
 * 開いてよいのは https だけ・1 枚 [MAX_BYTES] まで。キャッシュはメモリ [MEMORY_PERCENT]・ディスク [DISK_MAX_BYTES]。
 */
object RemoteImages {
    const val DISK_DIR = "metamon-images"
    const val DISK_MAX_BYTES = 64L * 1024 * 1024
    const val MEMORY_PERCENT = 0.15
    const val MAX_BYTES = 10L * 1024 * 1024
    const val MAX_URL_LENGTH = 2048

    /** 読み込んでよい URL(https・host あり・ID / パスワード無し)。それ以外は null。 */
    fun loadableUrl(value: String?): String? {
        val s = value?.trim()?.takeIf { it.isNotEmpty() && it.length <= MAX_URL_LENGTH } ?: return null
        val uri = runCatching { URI(s) }.getOrNull() ?: return null
        if (!uri.scheme.equals("https", ignoreCase = true)) return null
        if (uri.host.isNullOrEmpty() || uri.rawUserInfo != null) return null
        return s
    }

    /** 宣言された大きさ(不明は -1)が上限を超えるか。 */
    fun exceeds(contentLength: Long, maxBytes: Long = MAX_BYTES): Boolean = contentLength > maxBytes

    fun diskDir(context: Context): File = File(context.cacheDir, DISK_DIR)

    @Volatile
    private var installed: ImageLoader? = null

    /**
     * アプリ全体の画像の読み込みを決める(起動の組み立てで 1 回)。[client] は API と同じ OkHttp(接続を共有)。
     * 認証のヘッダは付けない([client] にトークンの interceptor が無いこと)。
     */
    @Synchronized
    fun install(context: Context, client: OkHttpClient) {
        if (installed != null) return
        val app = context.applicationContext
        val images = client.newBuilder()
            .readTimeout(Duration.ofSeconds(20))
            .addNetworkInterceptor(ImageGuard())
            .build()
        val loader = ImageLoader.Builder(app)
            .components { add(OkHttpNetworkFetcherFactory(callFactory = { images })) }
            .memoryCache { MemoryCache.Builder().maxSizePercent(app, MEMORY_PERCENT).build() }
            .diskCache { DiskCache.Builder().directory(diskDir(app).toOkioPath()).maxSizeBytes(DISK_MAX_BYTES).build() }
            .crossfade(true)
            .build()
        installed = loader
        SingletonImageLoader.setSafe { loader }
    }

    /** サインアウト・アカウント削除の後片付け(#841)。メモリとディスクのキャッシュを消す。 */
    fun wipe(context: Context) = wipe(diskDir(context))

    /** [dir] は [install] と同じディスクのキャッシュの置き場。 */
    fun wipe(dir: File) {
        val loader = installed
        loader?.memoryCache?.clear()
        loader?.diskCache?.clear()
        // 開いていない(この起動でまだ読んでいない)時の前の起動の分も残さない
        if (loader == null) dir.deleteRecursively()
    }
}

/** 画像の通信の決まり。https 以外(リダイレクト先も)と、[maxBytes] を超える応答を止める。 */
class ImageGuard(
    private val maxBytes: Long = RemoteImages.MAX_BYTES,
    private val requireHttps: Boolean = true,
) : Interceptor {
    override fun intercept(chain: Interceptor.Chain): Response {
        val request = chain.request()
        if (requireHttps && !request.url.isHttps) throw IOException("https 以外の画像は読まない")
        val response = chain.proceed(request)
        val body = response.body
        val declared = body.contentLength()
        if (RemoteImages.exceeds(declared, maxBytes)) {
            response.close()
            throw IOException("画像が大きすぎる($declared bytes)")
        }
        if (declared >= 0) return response
        // 大きさが分からない(chunked)時は読みながら数える
        val limited = LimitedSource(body.source(), maxBytes).buffer()
        return response.newBuilder().body(limited.asResponseBody(body.contentType(), -1)).build()
    }

    private class LimitedSource(source: Source, private val maxBytes: Long) : ForwardingSource(source) {
        private var total = 0L
        override fun read(sink: Buffer, byteCount: Long): Long {
            val n = super.read(sink, byteCount)
            if (n > 0) {
                total += n
                if (total > maxBytes) throw IOException("画像が大きすぎる")
            }
            return n
        }
    }
}
