package jp.kz2601.metamon.services.feedback

import jp.kz2601.metamon.services.http.ApiJson
import jp.kz2601.metamon.services.http.HttpRequest
import jp.kz2601.metamon.services.http.MetamonHttp
import kotlinx.serialization.Serializable
import java.util.Base64

/**
 * アプリ内のフィードバック(#319・iOS の `FeedbackSubmission`)。添付はどれも本人が選んだものだけ(既定はすべて無し)。記録の中身は送らない。
 * 契約: docs/backend/openapi-appconfig.yaml `POST /feedback`
 */
data class FeedbackSubmission(
    val kind: Kind = Kind.OTHER,
    val message: String,
    /** 送信前に見せた同意文の版([FeedbackConsent.VERSION]) */
    val consentVersion: String = FeedbackConsent.VERSION,
    /** JPEG([FeedbackConsent.MAX_SCREENSHOT_BYTES] 以下) */
    val screenshotJpeg: ByteArray? = null,
    val app: AppRef? = null,
    val device: Device? = null,
) {
    enum class Kind(val wire: String) { BUG("bug"), IDEA("idea"), OTHER("other") }

    /** 今のアプリの DSL(構造だけ)。サーバーが本人の木の `versions/{dslVersion}` を写すので、送るのは参照だけ。 */
    @Serializable
    data class AppRef(val appId: String, val dslVersion: Int)

    /** アプリの版・OS・機種・言語(各 64 文字まで)。 */
    @Serializable
    data class Device(val appVersion: String, val build: String, val os: String, val model: String, val locale: String) {
        /** サーバーの上限(各 64 文字)に収める。 */
        fun clipped(): Device = Device(appVersion.take(MAX), build.take(MAX), os.take(MAX), model.take(MAX), locale.take(MAX))

        private companion object {
            const val MAX = 64
        }
    }

    override fun equals(other: Any?): Boolean = other is FeedbackSubmission && kind == other.kind && message == other.message &&
        consentVersion == other.consentVersion && app == other.app && device == other.device &&
        (screenshotJpeg?.contentEquals(other.screenshotJpeg) ?: (other.screenshotJpeg == null))

    override fun hashCode(): Int = listOf(kind, message, consentVersion, app, device, screenshotJpeg?.contentHashCode()).hashCode()
}

/** 同意文の版と上限(iOS の `FeedbackConsent` と同じ値)。 */
object FeedbackConsent {
    /** 文言(strings_feedback_composer.xml の同意の段落)を変えたら上げる。サーバーは `consentVersion` として保存する */
    const val VERSION = "feedback-2026-11"

    /** サーバーの保存期間(日)。同意文にも出す。backend `store.FeedbackRetention` と揃える */
    const val RETENTION_DAYS = 180

    /** スクショの上限(デコード後のバイト)。backend `maxFeedbackScreenshot` と揃える */
    const val MAX_SCREENSHOT_BYTES = 512 shl 10

    const val MAX_MESSAGE_LENGTH = 2000
}

/** サーバーが実際に保存した添付。 */
@Serializable
data class FeedbackReceipt(val id: String, val status: String, val attachments: Attachments) {
    @Serializable
    data class Attachments(val screenshot: Boolean = false, val dsl: Boolean = false, val device: Boolean = false)
}

interface FeedbackClient {
    suspend fun send(feedback: FeedbackSubmission): FeedbackReceipt
}

/** appconfig の `POST /v1/feedback`(iOS の `HTTPFeedbackClient`)。本文・添付はログに出さない。 */
class HttpFeedbackClient(private val http: MetamonHttp) : FeedbackClient {
    override suspend fun send(feedback: FeedbackSubmission): FeedbackReceipt {
        val body = http.send(HttpRequest("POST", listOf("v1", "feedback"), body = bodyJson(feedback), timeoutMillis = TIMEOUT_MILLIS))
        return ApiJson.decodeFromString(FeedbackReceipt.serializer(), body)
    }

    @Serializable
    internal data class Body(val kind: String, val message: String, val consentVersion: String, val attachments: Attachments? = null) {
        @Serializable
        data class Attachments(
            val screenshot: String? = null,
            val app: FeedbackSubmission.AppRef? = null,
            val device: FeedbackSubmission.Device? = null,
        )
    }

    companion object {
        private const val TIMEOUT_MILLIS = 30_000L

        /** 送る JSON。添付が 1 つも無ければ `attachments` ごと付けない。 */
        fun bodyJson(f: FeedbackSubmission): String {
            val a = Body.Attachments(
                screenshot = f.screenshotJpeg?.let { Base64.getEncoder().encodeToString(it) },
                app = f.app,
                device = f.device?.clipped(),
            )
            val attachments = a.takeIf { it.screenshot != null || it.app != null || it.device != null }
            return ApiJson.encodeToString(Body.serializer(), Body(f.kind.wire, f.message.trim(), f.consentVersion, attachments))
        }
    }
}

/** モック(サーバー無しの起動・テスト)。送った内容を覚えるだけ。 */
class MockFeedbackClient : FeedbackClient {
    private val _sent = mutableListOf<FeedbackSubmission>()
    val sent: List<FeedbackSubmission> get() = synchronized(_sent) { _sent.toList() }

    override suspend fun send(feedback: FeedbackSubmission): FeedbackReceipt {
        val n = synchronized(_sent) { _sent += feedback; _sent.size }
        return FeedbackReceipt(
            id = "fb_mock$n",
            status = "received",
            attachments = FeedbackReceipt.Attachments(feedback.screenshotJpeg != null, feedback.app != null, feedback.device != null),
        )
    }
}
