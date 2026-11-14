package jp.kz2601.metamon.services.update

import jp.kz2601.metamon.services.consent.KeyValueStore
import kotlinx.serialization.json.JsonElement
import kotlinx.serialization.json.JsonObject
import java.net.URI

/**
 * アプリ内のお知らせ(#333・iOS の `ServiceNotice`)。`GET /v1/client-config` の `notice`(環境変数 `NOTICE_JSON`)。
 * 障害・メンテナンスの 1 行告知。閉じた `id` は [NoticeDismissals] が覚える。
 */
data class ServiceNotice(
    val id: String,
    val severity: Severity = Severity.INFO,
    val ja: String,
    val en: String? = null,
    val linkUrl: String? = null,
    /** iOS の build の範囲(Android の版とは比べられない) */
    val minIOSBuild: Int? = null,
    val maxIOSBuild: Int? = null,
) {
    enum class Severity { INFO, WARNING }

    /** 表示する言語の文面。英語で `en` が無ければ日本語。 */
    fun text(languageCode: String): String {
        val english = en?.trim()
        if (languageCode.lowercase().startsWith("en") && !english.isNullOrEmpty()) return english
        return ja.trim()
    }

    /** https のリンクだけ使う(サーバーでも弾くが、念のため)。 */
    val link: String?
        get() {
            val raw = linkUrl ?: return null
            val uri = runCatching { URI(raw) }.getOrNull() ?: return null
            return raw.takeIf { uri.scheme == "https" && !uri.host.isNullOrEmpty() }
        }

    /** 出してよい形か(id と日本語の文面がある)。 */
    val isDisplayable: Boolean get() = id.isNotBlank() && ja.isNotBlank()

    /**
     * Android に出すか。iOS の build の範囲(`minIOSBuild` / `maxIOSBuild`)が付いたお知らせは
     * iOS の特定の版に向けたものなので Android には出さない。範囲の無いお知らせ(障害・メンテナンス)は出す。
     */
    val appliesToAndroid: Boolean get() = minIOSBuild == null && maxIOSBuild == null

    companion object {
        /** 形が合わなければ null(お知らせだけ捨てて版の判定は続ける)。severity の知らない値は info。 */
        fun decode(element: JsonElement): ServiceNotice? {
            val obj = element.asObject() ?: return null
            val id = obj.string("id") ?: return null
            val message = obj["message"] as? JsonObject ?: return null
            val ja = message.string("ja") ?: return null
            return ServiceNotice(
                id = id,
                severity = if (obj.string("severity") == "warning") Severity.WARNING else Severity.INFO,
                ja = ja,
                en = message.string("en"),
                linkUrl = obj.string("linkUrl"),
                minIOSBuild = obj.positiveInt("minIOSBuild"),
                maxIOSBuild = obj.positiveInt("maxIOSBuild"),
            )
        }
    }
}

/** 閉じたお知らせの id を覚える(#333・iOS と同じ鍵)。同じ id は二度と出さない。新しい文面は新しい id で出す。 */
class NoticeDismissals(private val store: KeyValueStore) {
    fun isDismissed(id: String): Boolean = id in ids

    fun dismiss(id: String) {
        val list = (ids.filter { it != id } + id).takeLast(LIMIT)
        store.putString(KEY, list.joinToString(SEPARATOR))
    }

    /** 後片付け(#841)。iOS は UserDefaults ごと消す。 */
    fun clear() = store.remove(KEY)

    val ids: List<String> get() = store.getString(KEY)?.split(SEPARATOR)?.filter { it.isNotEmpty() }.orEmpty()

    companion object {
        const val KEY = "serviceNotice.dismissedIDs"

        /** 古いものから捨てる(設定を太らせない) */
        const val LIMIT = 20

        /** id は `[A-Za-z0-9._-]` だけなので改行で区切れる */
        private const val SEPARATOR = "\n"
    }
}
