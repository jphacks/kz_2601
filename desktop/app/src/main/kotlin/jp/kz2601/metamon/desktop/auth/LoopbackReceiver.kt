package jp.kz2601.metamon.desktop.auth

import com.sun.net.httpserver.HttpExchange
import com.sun.net.httpserver.HttpServer
import jp.kz2601.metamon.desktop.i18n.Lang
import jp.kz2601.metamon.desktop.i18n.Strings
import kotlinx.coroutines.CompletableDeferred
import kotlinx.coroutines.withTimeoutOrNull
import java.net.InetAddress
import java.net.InetSocketAddress
import java.net.URLDecoder
import java.nio.charset.StandardCharsets
import java.util.concurrent.ExecutorService
import java.util.concurrent.Executors
import java.util.concurrent.atomic.AtomicBoolean
import kotlin.time.Duration
import kotlin.time.Duration.Companion.minutes

/** 127.0.0.1 の受け口が受けた結果(#2151 の 2)。 */
sealed interface LoopbackResult {
    /** 認可コード。ログに出さないよう [toString] は値を伏せる。 */
    class Code(val code: String) : LoopbackResult {
        override fun toString(): String = "Code(***)"
    }

    /** state が送った物と違う(別のサインインの戻り・作られた戻り)。コードは使わない。 */
    data object StateMismatch : LoopbackResult

    data object Timeout : LoopbackResult

    /** アプリ側で止めた([LoopbackReceiver.cancel])。 */
    data object Cancelled : LoopbackResult

    /** Google が `error=`(`access_denied` など)で戻した・コードが無い。 */
    data class Error(val error: String) : LoopbackResult
}

/**
 * Google のサインインの戻り先。127.0.0.1 の空いた番号で立ち、`/` への最初の 1 回の戻りを受けたら閉じる(#2151 の 2)。
 * 受けた値はログに出さない。
 */
class LoopbackReceiver private constructor(
    private val server: HttpServer,
    private val executor: ExecutorService,
    private val expectedState: String,
) : AutoCloseable {
    private val result = CompletableDeferred<LoopbackResult>()
    private val consumed = AtomicBoolean(false)
    private val closed = AtomicBoolean(false)

    val port: Int = server.address.port

    /** 同意の URL の `redirect_uri` に渡す値。 */
    val redirectUri: String = GooglePkce.redirectUri(port)

    init {
        server.createContext("/") { exchange ->
            try {
                handle(exchange)
            } finally {
                exchange.close()
            }
        }
    }

    /** 結果を待つ。どの結果でも返った時には口は閉じている。 */
    suspend fun await(timeout: Duration = DEFAULT_TIMEOUT): LoopbackResult = try {
        withTimeoutOrNull(timeout) { result.await() } ?: LoopbackResult.Timeout
    } finally {
        close()
    }

    fun cancel() {
        result.complete(LoopbackResult.Cancelled)
        close()
    }

    override fun close() {
        if (!closed.compareAndSet(false, true)) return
        server.stop(0)
        executor.shutdownNow()
    }

    private fun handle(exchange: HttpExchange) {
        if (exchange.requestMethod != "GET" || exchange.requestURI.rawPath != "/" || consumed.get()) {
            respond(exchange, 404, null)
            return
        }
        val params = parseQuery(exchange.requestURI.rawQuery)
        val outcome = when {
            !GooglePkce.statesMatch(expectedState, params["state"]) -> LoopbackResult.StateMismatch
            params["error"] != null -> LoopbackResult.Error(sanitizeError(params.getValue("error")))
            params["code"].isNullOrEmpty() -> LoopbackResult.Error("missing_code")
            else -> LoopbackResult.Code(params.getValue("code"))
        }
        if (!consumed.compareAndSet(false, true)) {
            respond(exchange, 404, null)
            return
        }
        respond(exchange, 200, outcome is LoopbackResult.Code)
        // 応答を送り終えてから知らせる(待つ側が口を閉じる)
        result.complete(outcome)
    }

    private fun respond(exchange: HttpExchange, status: Int, success: Boolean?) {
        val body = (if (success == null) "" else page(success)).toByteArray(StandardCharsets.UTF_8)
        exchange.responseHeaders.apply {
            set("Content-Type", "text/html; charset=utf-8")
            set("Cache-Control", "no-store")
            set("Referrer-Policy", "no-referrer")
            set("X-Content-Type-Options", "nosniff")
            set("Content-Security-Policy", "default-src 'none'")
            set("Connection", "close")
        }
        exchange.sendResponseHeaders(status, if (body.isEmpty()) -1 else body.size.toLong())
        if (body.isNotEmpty()) exchange.responseBody.use { it.write(body) }
    }

    companion object {
        val DEFAULT_TIMEOUT: Duration = 5.minutes
        private const val MAX_ERROR_LENGTH = 64
        private val LOOPBACK_ADDRESS: InetAddress = InetAddress.getByAddress(byteArrayOf(127, 0, 0, 1))

        /** 127.0.0.1 の空いた番号(OS が選ぶ)で立てる。[expectedState] は [GooglePkce.newState] の値。 */
        fun start(expectedState: String): LoopbackReceiver {
            require(expectedState.isNotEmpty()) { "expectedState is empty" }
            val server = HttpServer.create(InetSocketAddress(LOOPBACK_ADDRESS, 0), 1)
            val executor = Executors.newSingleThreadExecutor { r ->
                Thread(r, "loopback-receiver").apply { isDaemon = true }
            }
            server.executor = executor
            val receiver = LoopbackReceiver(server, executor, expectedState)
            server.start()
            return receiver
        }

        /** 同じ名前が 2 回あれば最初の物。 */
        internal fun parseQuery(raw: String?): Map<String, String> {
            if (raw.isNullOrEmpty()) return emptyMap()
            val out = linkedMapOf<String, String>()
            for (part in raw.split('&')) {
                if (part.isEmpty()) continue
                val k = decode(part.substringBefore('=')) ?: continue
                val v = decode(if ('=' in part) part.substringAfter('=') else "") ?: continue
                out.putIfAbsent(k, v)
            }
            return out
        }

        private fun decode(s: String): String? =
            runCatching { URLDecoder.decode(s, StandardCharsets.UTF_8) }.getOrNull()

        /** 画面・ログに出せる形(英数字と `_` `.` `-` だけ・長さを切る)。 */
        private fun sanitizeError(raw: String): String =
            raw.filter { it.isLetterOrDigit() && it.code < 128 || it == '_' || it == '.' || it == '-' }
                .take(MAX_ERROR_LENGTH)
                .ifEmpty { "unknown_error" }

        private fun page(success: Boolean): String {
            val key = if (success) "desktop_signin_loopback_done" else "desktop_signin_loopback_failed"
            val ja = escapeHtml(Strings.table.get(Lang.JA, key))
            val en = escapeHtml(Strings.table.get(Lang.EN, key))
            return "<!doctype html><html><head><meta charset=\"utf-8\"><title>Tailor It</title></head>" +
                "<body><p lang=\"ja\">$ja</p><p lang=\"en\">$en</p></body></html>"
        }

        private fun escapeHtml(s: String): String =
            s.replace("&", "&amp;").replace("<", "&lt;").replace(">", "&gt;").replace("\"", "&quot;")
    }
}
