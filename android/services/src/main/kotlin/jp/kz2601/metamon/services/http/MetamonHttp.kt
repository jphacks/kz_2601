package jp.kz2601.metamon.services.http

import jp.kz2601.metamon.services.RemainingToday
import kotlinx.coroutines.delay
import kotlinx.coroutines.suspendCancellableCoroutine
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive
import kotlinx.serialization.json.contentOrNull
import kotlinx.serialization.json.doubleOrNull
import kotlinx.serialization.json.jsonObject
import okhttp3.Call
import okhttp3.Callback
import okhttp3.Headers
import okhttp3.HttpUrl
import okhttp3.HttpUrl.Companion.toHttpUrl
import okhttp3.MediaType.Companion.toMediaType
import okhttp3.OkHttpClient
import okhttp3.Request
import okhttp3.RequestBody.Companion.toRequestBody
import okhttp3.Response
import java.io.IOException
import java.time.Duration
import java.time.Instant
import java.time.ZonedDateTime
import java.time.format.DateTimeFormatter
import kotlin.coroutines.resume
import kotlin.coroutines.resumeWithException

/** API の JSON。知らないキーは無視し、null は送らない。 */
val ApiJson: Json = Json {
    ignoreUnknownKeys = true
    explicitNulls = false
    encodeDefaults = true
    coerceInputValues = true
}

/** 1 回のリクエスト。パスは区切りごとに渡す(appId などの値は OkHttp がエスケープする)。 */
data class HttpRequest(
    val method: String,
    val path: List<String>,
    val query: List<Pair<String, String>> = emptyList(),
    /** JSON 本文。null は本文なし */
    val body: String? = null,
    val contentType: String = JSON_TYPE,
    val headers: Map<String, String> = emptyMap(),
    /** 次のバイトが届くまでの待ち(全体の上限ではない)。null は既定 */
    val timeoutMillis: Long? = null,
) {
    val isIdempotent: Boolean get() = method == "GET" || method == "HEAD"

    companion object {
        const val JSON_TYPE = "application/json"
    }
}

/** ステータスを問わない応答(304 などを呼び出し側で扱う)。 */
class HttpResponse(val status: Int, val body: String, val headers: Headers) {
    fun header(name: String): String? = headers[name]
    val isSuccessful: Boolean get() = status in 200..299
}

/**
 * 自動のやり直しの決まり。
 * - 401 / 401 `app_check_failed` / 429 は iOS と同じく 1 回だけ再送(全メソッド。サーバーが処理する前に弾く応答なので二重実行にならない)
 * - GET / HEAD だけ、圏外・時間切れ・502 / 503 / 504 を指数の間隔で [idempotentAttempts] 回まで試す(iOS には無い。POST は二重実行を避けて再送しない)
 */
data class RetryPolicy(
    /** これを超える `Retry-After` は待たずに失敗にして画面に出す */
    val maxRetryAfterSeconds: Double = 15.0,
    val defaultRetryAfterSeconds: Double = 2.0,
    val idempotentAttempts: Int = 3,
    val initialBackoffMillis: Long = 500,
    val maxBackoffMillis: Long = 4_000,
) {
    fun backoffMillis(retry: Int): Long =
        (initialBackoffMillis shl (retry - 1).coerceIn(0, 20)).coerceAtMost(maxBackoffMillis)
}

/**
 * JSON over HTTP の最小クライアント(iOS の `HTTP`)。ai / appconfig で 1 つずつ作る。
 * 毎回のヘッダ: `Authorization: Bearer`・`X-Firebase-AppCheck`・`X-Client-*`・`X-Time-Zone`・`Accept-Language`。
 * HTTP のキャッシュは使わない(DSL の鮮度は `If-None-Match` で自前に見る)。
 */
class MetamonHttp(
    baseUrl: String,
    private val tokens: TokenProvider,
    private val clientInfo: ClientInfo,
    private val appCheck: AppCheckTokenProvider? = null,
    private val client: OkHttpClient = defaultClient(),
    private val retry: RetryPolicy = RetryPolicy(),
    private val device: DeviceContext = DeviceContext(),
    private val sleep: suspend (Long) -> Unit = { delay(it) },
    private val clock: () -> Instant = Instant::now,
    /** 応答の `X-RateLimit-*` から読んだ「今日の残り」の行き先(iOS の `HTTP.noteRemaining`・#1203)。ai だけに付ける */
    private val remainingSink: ((RemainingToday) -> Unit)? = null,
    /** ID トークンを取り直しても 401 だった(iOS の `sessionExpired`・#1102)。サインイン画面に戻す口 */
    private val onSessionExpired: (() -> Unit)? = null,
) {
    val baseUrl: HttpUrl = baseUrl.toHttpUrl()

    /** いまの UI の言語("ja" | "en")。`Accept-Language` と生成・相談の `locale` に使う(#843)。 */
    fun language(): String = device.language()

    fun url(req: HttpRequest): HttpUrl = baseUrl.newBuilder().apply {
        req.path.forEach { addPathSegment(it) }
        req.query.forEach { (k, v) -> addQueryParameter(k, v) }
    }.build()

    /** 2xx の本文だけ返す(それ以外は [ApiError])。 */
    suspend fun send(req: HttpRequest): String = check(exchange(req))

    /** ステータスを問わず返す。転送のエラーだけ throw。401 / 429 の 1 回の再送と GET のやり直しはここで行う。 */
    suspend fun exchange(req: HttpRequest): HttpResponse {
        var bearer = tokens.idToken(false)
        var check = appCheck?.token(false)
        var r = performWithRetry(req, bearer, check)

        val provider = appCheck
        if (r.status == 401 && isAppCheckRejection(r) && provider != null) {
            // App Check のトークンが弾かれた。取り直して値が変わった時だけ 1 回再送(ID トークンは取り直さない)
            val fresh = provider.token(true)
            if (fresh != null && fresh != check) {
                check = fresh
                r = perform(req, bearer, check)
            }
        }
        if (r.status == 401 && !isAppCheckRejection(r) && bearer != null) {
            // ID トークンの失効(1 時間)。取り直して値が変わった時だけ 1 回再送
            val fresh = tokens.idToken(true)
            if (fresh != null && fresh != bearer) {
                bearer = fresh
                r = perform(req, bearer, check)
            }
            // 取り直しても(取り直せなくても)401 = セッション切れ。トークンを付けていない(未サインイン)時は呼ばない
            if (r.status == 401 && !isAppCheckRejection(r)) onSessionExpired?.invoke()
        }
        if (r.status == 429) {
            val wait = retryAfterSeconds(r) ?: retry.defaultRetryAfterSeconds
            if (wait <= retry.maxRetryAfterSeconds) {
                sleep((wait * 1000).toLong())
                r = perform(req, bearer, check)
            }
        }
        noteRemaining(r.status, r.body, r.headers)
        return r
    }

    /** ヘッダーの無い応答(appconfig など)では何もしない */
    private fun noteRemaining(status: Int, body: String, headers: Headers) {
        val sink = remainingSink ?: return
        RemainingToday.parse({ headers[it] }, status, body, clock())?.let(sink)
    }

    /**
     * 本文を少しずつ読むリクエスト(SSE・#636)。ステータスを問わず返し、再送はしない(呼び出し側が [exchange] に戻す)。
     * 返した [Response] は呼び出し側が閉じる。
     * [forceRefresh] は 401 の後の張り直し(ID トークンと App Check を取り直す・ホットリロードの SSE・#883)。
     */
    suspend fun openStream(req: HttpRequest, forceRefresh: Boolean = false): Response {
        val bearer = tokens.idToken(forceRefresh)
        val check = appCheck?.token(forceRefresh)
        // 本文はまだ無いので、ヘッダーだけで読む(429 は本文の scope が要るので数えない)
        return clientFor(req).newCall(build(req, bearer, check)).await().also { noteRemaining(it.code, "", it.headers) }
    }

    fun check(r: HttpResponse): String {
        if (!r.isSuccessful) throw errorFrom(r)
        return r.body
    }

    /** 共通のエラー封筒 → [ApiError.Server]。封筒で無ければ [ApiError.Http]。 */
    fun errorFrom(r: HttpResponse): ApiError = errorFrom(r.status, r.body, retryAfterSeconds(r))

    /** `Retry-After` を秒に(秒 / HTTP-date)。無ければ null。 */
    fun retryAfterSeconds(r: HttpResponse): Double? = parseRetryAfter(r.header("Retry-After"), clock())

    private suspend fun performWithRetry(req: HttpRequest, bearer: String?, check: String?): HttpResponse {
        val attempts = if (req.isIdempotent) retry.idempotentAttempts.coerceAtLeast(1) else 1
        var n = 1
        while (true) {
            val r = try {
                perform(req, bearer, check)
            } catch (e: IOException) {
                if (n >= attempts) throw e
                null
            }
            if (r != null && (n >= attempts || !isTransient(r))) return r
            sleep(retry.backoffMillis(n))
            n++
        }
    }

    /** やり直して直りうる 5xx(運営の停止・無料プランの停止は待っても直らないので除く)。 */
    private fun isTransient(r: HttpResponse): Boolean {
        if (r.status !in setOf(502, 503, 504)) return false
        return errorCode(r.body) !in setOf(ErrorCodes.AI_PAUSED, ErrorCodes.FREE_PLAN_PAUSED, ErrorCodes.APP_STOPPED)
    }

    private suspend fun perform(req: HttpRequest, bearer: String?, check: String?): HttpResponse {
        val call = clientFor(req).newCall(build(req, bearer, check))
        return suspendCancellableCoroutine { cont ->
            cont.invokeOnCancellation { call.cancel() }
            call.enqueue(object : Callback {
                override fun onFailure(call: Call, e: IOException) {
                    cont.resumeWithException(e)
                }

                override fun onResponse(call: Call, response: Response) {
                    val result = runCatching {
                        response.use { HttpResponse(it.code, it.body.string(), it.headers) }
                    }
                    result.fold(cont::resume, cont::resumeWithException)
                }
            })
        }
    }

    private fun clientFor(req: HttpRequest): OkHttpClient {
        val t = req.timeoutMillis ?: return client
        return client.newBuilder().readTimeout(Duration.ofMillis(t)).build()
    }

    private fun build(req: HttpRequest, bearer: String?, check: String?): Request {
        val body = when {
            req.body != null -> req.body.toRequestBody(req.contentType.toMediaType())
            req.method in setOf("POST", "PUT", "PATCH") -> ByteArray(0).toRequestBody(null)
            else -> null
        }
        return Request.Builder().url(url(req)).method(req.method, body).apply {
            clientInfo.headers.forEach { (k, v) -> header(k, v) }
            header("X-Time-Zone", device.timeZone())
            header("Accept-Language", device.language())
            req.headers.forEach { (k, v) -> header(k, v) }
            if (bearer != null) header("Authorization", "Bearer $bearer")
            if (check != null) header(AppCheckHeader.NAME, check)
        }.build()
    }

    private fun isAppCheckRejection(r: HttpResponse): Boolean = errorCode(r.body) == AppCheckHeader.FAILED_CODE

    companion object {
        fun defaultClient(): OkHttpClient = OkHttpClient.Builder()
            .connectTimeout(Duration.ofSeconds(15))
            // ai の生成(Claude 呼び出し)を待つので長め(iOS の 45 秒)
            .readTimeout(Duration.ofSeconds(45))
            .writeTimeout(Duration.ofSeconds(45))
            .build()

        fun errorFrom(status: Int, body: String, retryAfter: Double?): ApiError {
            val inner = envelope(body)
            if (inner != null) {
                val code = (inner["code"] as? JsonPrimitive)?.contentOrNull
                if (code != null) {
                    return ApiError.Server(
                        status = status,
                        code = code,
                        serverMessage = (inner["message"] as? JsonPrimitive)?.contentOrNull ?: "",
                        details = inner["details"] as? JsonObject ?: JsonObject(emptyMap()),
                        retryAfterSeconds = retryAfter,
                    )
                }
            }
            return ApiError.Http(status, body, retryAfter)
        }

        fun errorCode(body: String): String? = (envelope(body)?.get("code") as? JsonPrimitive)?.contentOrNull

        /** `{error:{...}}` の中身。SSE の `event: error` の `status` もここに入る */
        internal fun envelope(body: String): JsonObject? = runCatching {
            ApiJson.parseToJsonElement(body).jsonObject["error"] as? JsonObject
        }.getOrNull()

        fun parseRetryAfter(raw: String?, now: Instant): Double? {
            val v = raw?.trim().orEmpty()
            if (v.isEmpty()) return null
            v.toDoubleOrNull()?.let { return it.coerceAtLeast(0.0) }
            val date = runCatching { ZonedDateTime.parse(v, DateTimeFormatter.RFC_1123_DATE_TIME).toInstant() }.getOrNull()
                ?: return null
            return (Duration.between(now, date).toMillis() / 1000.0).coerceAtLeast(0.0)
        }

        internal fun statusOf(inner: JsonObject): Int? = (inner["status"] as? JsonPrimitive)?.doubleOrNull?.toInt()
    }
}

private suspend fun Call.await(): Response = suspendCancellableCoroutine { cont ->
    cont.invokeOnCancellation { cancel() }
    enqueue(object : Callback {
        override fun onFailure(call: Call, e: IOException) {
            cont.resumeWithException(e)
        }

        override fun onResponse(call: Call, response: Response) {
            cont.resume(response) { _, value, _ -> value.close() }
        }
    })
}
