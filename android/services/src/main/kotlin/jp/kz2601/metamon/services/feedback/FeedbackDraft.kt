package jp.kz2601.metamon.services.feedback

import jp.kz2601.metamon.dsl.AppDocument
import jp.kz2601.metamon.dsl.isVoiceOnly
import jp.kz2601.metamon.services.http.ApiJson
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.JsonArray
import kotlinx.serialization.json.JsonElement
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive
import kotlinx.serialization.json.intOrNull
import kotlinx.serialization.json.jsonObject

/**
 * フィードバックの画面を開いた時点の「送れるもの」(iOS の `FeedbackContext`)。どれも本人が選んだ時だけ送る。
 * スクショは端末の画像の型なので画面の側で持つ([hasScreenshot] はその有無だけ)。
 */
data class FeedbackContext(
    val app: AppSummary?,
    val device: FeedbackSubmission.Device,
    val hasScreenshot: Boolean = false,
) {
    /** 今のアプリの DSL の見せ方。送るのは [ref](サーバーが本人の木から構造だけを写す)。 */
    data class AppSummary(
        val ref: FeedbackSubmission.AppRef,
        val name: String,
        val screens: List<String>,
        val entities: List<String>,
        /** 端末にある同じ版の DSL(整形済み)。無ければ要約だけ見せる */
        val json: String?,
    )

    companion object {
        /** 初期状態(音声の画面だけ)は送る意味が無いので null。[cachedEnvelope] は DocumentCache の `{appId, version, dsl}`。 */
        fun appSummary(doc: AppDocument, cachedEnvelope: String?): AppSummary? {
            if (doc.version < 1 || doc.dsl.isVoiceOnly) return null
            return AppSummary(
                ref = FeedbackSubmission.AppRef(doc.appId, doc.version),
                name = doc.dsl.app.name,
                screens = doc.dsl.screens.map { it.title.ifEmpty { it.id } },
                entities = doc.dsl.entities.map { e -> "${e.name.ifEmpty { e.id }}(${e.fields.joinToString(", ") { it.name }})" },
                json = cachedEnvelope?.let { dslJson(it, doc.version) },
            )
        }

        /** 封筒から同じ版の `dsl` だけを、キーを名前順にして整形する。記録は入っていない。 */
        fun dslJson(envelope: String, version: Int): String? {
            val env = runCatching { ApiJson.parseToJsonElement(envelope).jsonObject }.getOrNull() ?: return null
            if ((env["version"] as? JsonPrimitive)?.intOrNull != version) return null
            val dsl = env["dsl"] as? JsonObject ?: return null
            return Pretty.encodeToString(JsonElement.serializer(), sorted(dsl))
        }

        private val Pretty = Json { prettyPrint = true }

        private fun sorted(e: JsonElement): JsonElement = when (e) {
            is JsonObject -> JsonObject(e.entries.sortedBy { it.key }.associate { it.key to sorted(it.value) })
            is JsonArray -> JsonArray(e.map(::sorted))
            else -> e
        }
    }
}

/**
 * 書きかけのフィードバック(iOS の FeedbackComposerView の @State)。添付はどれも既定でオフ、同意も既定でオフ。
 * 送る中身は [build] だけが作る(同意が無ければ作らない)。
 */
data class FeedbackDraft(
    val kind: FeedbackSubmission.Kind = FeedbackSubmission.Kind.BUG,
    val message: String = "",
    val attachScreenshot: Boolean = false,
    val attachApp: Boolean = false,
    val attachDevice: Boolean = false,
    val agreed: Boolean = false,
) {
    val trimmed: String get() = message.trim()
    val isTooLong: Boolean get() = trimmed.length > FeedbackConsent.MAX_MESSAGE_LENGTH

    /** 送信ボタンを押せるか(送信中・送信済みは画面の側で見る)。 */
    val canSend: Boolean get() = trimmed.isNotEmpty() && !isTooLong && agreed

    sealed interface Built {
        data class Ready(val submission: FeedbackSubmission) : Built

        /** 同意のトグルが入っていない・本文が空・長すぎる */
        data object NotReady : Built

        /** スクショを上限まで縮められなかった(オフにして送ってもらう) */
        data object ScreenshotTooLarge : Built
    }

    /**
     * 送る中身を組み立てる。オンにした添付だけを入れる。
     * [screenshotJpeg] はスクショをオンにした時だけ呼ぶ(上限以下の JPEG、無理なら null)。
     */
    fun build(context: FeedbackContext, screenshotJpeg: () -> ByteArray?): Built {
        if (!canSend) return Built.NotReady
        val jpeg = if (attachScreenshot && context.hasScreenshot) {
            screenshotJpeg()?.takeIf { it.size <= FeedbackConsent.MAX_SCREENSHOT_BYTES } ?: return Built.ScreenshotTooLarge
        } else {
            null
        }
        return Built.Ready(
            FeedbackSubmission(
                kind = kind,
                message = trimmed,
                screenshotJpeg = jpeg,
                app = if (attachApp) context.app?.ref else null,
                device = if (attachDevice) context.device else null,
            ),
        )
    }
}

/** 下書きを送る。同意が無い・空の下書きは送らずに返す(最後の砦。画面でもボタンを押せなくしている)。 */
class FeedbackSender(private val client: FeedbackClient) {
    sealed interface Result {
        data class Sent(val receipt: FeedbackReceipt) : Result
        data object NotReady : Result
        data object ScreenshotTooLarge : Result
    }

    suspend fun send(draft: FeedbackDraft, context: FeedbackContext, screenshotJpeg: () -> ByteArray?): Result =
        when (val built = draft.build(context, screenshotJpeg)) {
            FeedbackDraft.Built.NotReady -> Result.NotReady
            FeedbackDraft.Built.ScreenshotTooLarge -> Result.ScreenshotTooLarge
            is FeedbackDraft.Built.Ready -> Result.Sent(client.send(built.submission))
        }
}
