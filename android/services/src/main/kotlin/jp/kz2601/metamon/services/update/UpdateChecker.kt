package jp.kz2601.metamon.services.update

import jp.kz2601.metamon.services.http.HttpRequest
import jp.kz2601.metamon.services.http.MetamonHttp
import kotlinx.coroutines.CancellationException
import java.net.URI

fun interface ClientConfigClient {
    suspend fun fetch(): ClientConfig
}

/**
 * `GET {appconfig}/v1/client-config`。トークンも App Check も無しで呼ぶ(サインイン前にも呼ぶ・backend の対象外)。
 * [MetamonHttp] は Bearer の無い [jp.kz2601.metamon.services.http.TokenProvider] で作る。
 */
class HttpClientConfigClient(private val http: MetamonHttp, private val timeoutMillis: Long = 8_000) : ClientConfigClient {
    override suspend fun fetch(): ClientConfig =
        ClientConfig.decode(http.send(HttpRequest("GET", listOf("v1", "client-config"), timeoutMillis = timeoutMillis)))
}

/** 起動時・前面に戻った時の確認(iOS の `UpdateChecker`)。失敗はすべて [UpdateRequirement.None](fail open)。 */
class UpdateChecker(private val client: ClientConfigClient?, private val build: String) {
    data class Result(
        val requirement: UpdateRequirement,
        /** サーバーが指定したストアの URL(無ければ null = アプリの既定) */
        val storeUrl: String? = null,
        /** この端末に出すお知らせ。閉じたかどうかは見ない([NoticeDismissals]) */
        val notice: ServiceNotice? = null,
    )

    suspend fun check(): Result {
        val config = try {
            client?.fetch()
        } catch (e: CancellationException) {
            throw e
        } catch (_: Exception) {
            null
        } ?: return Result(UpdateRequirement.None)
        return Result(
            requirement = UpdateRequirement.evaluate(config, build),
            storeUrl = config.androidStoreUrl?.takeIf(::isStoreUrl),
            notice = config.notice?.takeIf { it.isDisplayable && it.appliesToAndroid },
        )
    }

    companion object {
        /** https か market: のストアの URL だけ受ける。 */
        fun isStoreUrl(raw: String): Boolean {
            val uri = runCatching { URI(raw) }.getOrNull() ?: return false
            return when (uri.scheme) {
                "https" -> !uri.host.isNullOrEmpty()
                "market" -> true
                else -> false
            }
        }
    }
}
