package jp.kz2601.metamon.desktop.auth

import jp.kz2601.metamon.services.http.FailureKind
import jp.kz2601.metamon.services.http.MetamonHttp
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.suspendCancellableCoroutine
import kotlinx.coroutines.withContext
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive
import kotlinx.serialization.json.contentOrNull
import kotlinx.serialization.json.jsonObject
import okhttp3.Call
import okhttp3.Callback
import okhttp3.FormBody
import okhttp3.HttpUrl.Companion.toHttpUrl
import okhttp3.MediaType.Companion.toMediaType
import okhttp3.OkHttpClient
import okhttp3.Request
import okhttp3.RequestBody
import okhttp3.RequestBody.Companion.toRequestBody
import okhttp3.Response
import java.io.IOException
import java.time.Instant
import kotlin.coroutines.resume
import kotlin.coroutines.resumeWithException

/** サインイン中の人。ID トークンはメモリだけに持ち、端末に残すのは [refreshToken] だけ([TokenStore])。 */
data class AuthSession(
    val uid: String,
    val idToken: String,
    val refreshToken: String,
    val expiresAt: Instant,
) {
    /** 期限の [skewSeconds] 秒前から切れた扱い(送っている間に切れないように) */
    fun isExpired(now: Instant = Instant.now(), skewSeconds: Long = 60): Boolean =
        !now.plusSeconds(skewSeconds).isBefore(expiresAt)

    /** ログに出してよい形(トークンは伏せる)。 */
    override fun toString(): String = "AuthSession(uid=$uid, expiresAt=$expiresAt)"
}

/** 認証の結果。失敗は API と同じ [FailureKind] で分ける(画面の文言・主ボタンを揃える)。 */
sealed interface AuthResult<out T> {
    data class Ok<T>(val value: T) : AuthResult<T>
    data class Failed(val kind: FailureKind) : AuthResult<Nothing>
}

/**
 * Firebase Auth の REST(Identity Toolkit・Secure Token)の最小クライアント(#2150)。
 * Firebase の Android SDK は JVM で動かないので、デスクトップはこれで匿名で始め、ID トークンを更新する。
 * 画面・`DesktopApi` にはまだつながない(#2154 でつなぐ)。
 */
class FirebaseAuthRestClient(
    private val config: FirebaseAuthConfig,
    private val client: OkHttpClient = MetamonHttp.defaultClient(),
    private val clock: () -> Instant = Instant::now,
) {
    /** 匿名で始める(`accounts:signUp`)。 */
    suspend fun signInAnonymously(): AuthResult<AuthSession> {
        val key = config.apiKey ?: return notConfigured()
        val url = "${config.identityToolkitUrl.trimEnd('/')}/accounts:signUp".toHttpUrl().newBuilder()
            .addQueryParameter("key", key).build()
        val body = """{"returnSecureToken":true}""".toRequestBody(JSON)
        return call(Request.Builder().url(url).post(body).build()) { json ->
            AuthSession(
                uid = json.string("localId") ?: return@call null,
                idToken = json.string("idToken") ?: return@call null,
                refreshToken = json.string("refreshToken") ?: return@call null,
                expiresAt = clock().plusSeconds(json.string("expiresIn")?.toLongOrNull() ?: return@call null),
            )
        }
    }

    /** ID トークンの更新(`token` の `grant_type=refresh_token`)。リフレッシュトークンも新しい物に替わることがある。 */
    suspend fun refresh(refreshToken: String): AuthResult<AuthSession> {
        val key = config.apiKey ?: return notConfigured()
        val url = "${config.secureTokenUrl.trimEnd('/')}/token".toHttpUrl().newBuilder()
            .addQueryParameter("key", key).build()
        val body: RequestBody = FormBody.Builder()
            .add("grant_type", "refresh_token")
            .add("refresh_token", refreshToken)
            .build()
        return call(Request.Builder().url(url).post(body).build()) { json ->
            AuthSession(
                uid = json.string("user_id") ?: return@call null,
                idToken = json.string("id_token") ?: return@call null,
                refreshToken = json.string("refresh_token") ?: refreshToken,
                expiresAt = clock().plusSeconds(json.string("expires_in")?.toLongOrNull() ?: return@call null),
            )
        }
    }

    private fun notConfigured(): AuthResult<Nothing> =
        AuthResult.Failed(FailureKind.Rejected("TAILORIT_FIREBASE_API_KEY is not set"))

    /** 本文を読むのも I/O なので、呼び出し元のスレッド(画面など)ではなく IO で回す。 */
    private suspend fun <T : Any> call(request: Request, parse: (JsonObject) -> T?): AuthResult<T> =
        withContext(Dispatchers.IO) {
            val (status, body, retryAfter) = try {
                client.newCall(request).await().use { r ->
                    Triple(r.code, r.body.string(), r.header("Retry-After")?.toDoubleOrNull())
                }
            } catch (e: IOException) {
                return@withContext AuthResult.Failed(FailureKind.classify(e))
            }
            val json = runCatching { Json.parseToJsonElement(body).jsonObject }.getOrNull()
            if (status in 200..299) {
                val value = json?.let(parse) ?: return@withContext AuthResult.Failed(FailureKind.Other("unreadable auth response"))
                return@withContext AuthResult.Ok(value)
            }
            return@withContext AuthResult.Failed(classify(status, json?.let(::errorMessage), retryAfter))
        }

    companion object {
        private val JSON = "application/json".toMediaType()

        /** 入れ直してもらうしかない(サインインし直す)エラー。Firebase の `error.message` の値 */
        private val SESSION_ERRORS = setOf(
            "TOKEN_EXPIRED",
            "INVALID_REFRESH_TOKEN",
            "INVALID_ID_TOKEN",
            "USER_DISABLED",
            "USER_NOT_FOUND",
            "CREDENTIAL_TOO_OLD_LOGIN_AGAIN",
        )

        /**
         * Firebase の失敗を [FailureKind] に分ける。Firebase は期限切れ・無効なトークンも 400 で返すので
         * `error.message` で [FailureKind.SessionExpired] に寄せる。
         */
        fun classify(status: Int, message: String?, retryAfterSeconds: Double? = null): FailureKind {
            // "WEAK_PASSWORD : Password should be ..." の形もあるので、頭のコードだけ見る
            val code = message?.substringBefore(" : ")?.trim()
            return when {
                code in SESSION_ERRORS -> FailureKind.SessionExpired
                status == 401 -> FailureKind.SessionExpired
                code == "TOO_MANY_ATTEMPTS_TRY_LATER" || status == 429 -> FailureKind.Busy(retryAfterSeconds)
                status == 504 -> FailureKind.Timeout
                status in 500..599 -> FailureKind.Busy(retryAfterSeconds)
                status in 400..499 -> FailureKind.Rejected(message ?: "HTTP $status")
                else -> FailureKind.Other(message ?: "HTTP $status")
            }
        }

        /** `{"error":{"code":400,"message":"TOKEN_EXPIRED"}}`(Identity Toolkit・Secure Token とも) */
        private fun errorMessage(json: JsonObject): String? =
            (json["error"] as? JsonObject)?.string("message")

        private fun JsonObject.string(key: String): String? = (this[key] as? JsonPrimitive)?.contentOrNull
    }
}

private suspend fun Call.await(): Response = suspendCancellableCoroutine { cont ->
    cont.invokeOnCancellation { cancel() }
    enqueue(object : Callback {
        override fun onResponse(call: Call, response: Response) = cont.resume(response)
        override fun onFailure(call: Call, e: IOException) = cont.resumeWithException(e)
    })
}
