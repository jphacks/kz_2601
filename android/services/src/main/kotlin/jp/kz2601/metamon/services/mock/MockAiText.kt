package jp.kz2601.metamon.services.mock

import jp.kz2601.metamon.dsl.ai.AiTextField
import jp.kz2601.metamon.dsl.ai.AiTextRequest
import jp.kz2601.metamon.dsl.ai.AiTextResult
import jp.kz2601.metamon.dsl.ai.AiTextShape
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive

/** `POST /v1/text` のモックの答え(iOS `MockAIClient+Text` と同じ文)。 */
object MockAiText {
    /** aiChat: 決まった返事。saveTo があれば最初の string / text の field に最後の発言を入れた案を付ける。 */
    fun chat(request: AiTextRequest): AiTextResult? {
        val reply = AiTextResult.normalize("話してくれてありがとう。続きを聞かせてください(モック)", null, null, request) ?: return null
        val last = request.messages?.lastOrNull()?.text ?: return reply
        val field = request.saveTo?.fields?.firstOrNull { it.type == "string" || it.type == "text" } ?: return reply
        return reply.copy(suggest = JsonObject(mapOf(field.id to JsonPrimitive(last))))
    }

    val ideas = listOf(
        "変顔で自撮りを 1 枚", "好きな歌をワンフレーズ", "語尾を「にゃん」にして 1 分話す", "得意なモノマネを 1 つ",
        "最近あった小さな失敗を話す", "全員を 1 人ずつほめる", "10 秒で早口言葉", "利き手と逆の手で名前を書く",
        "今日のラッキーカラーを発表", "次の乾杯の音頭をとる",
    )

    fun answer(request: AiTextRequest): AiTextResult? {
        if (request.isChat) return chat(request)
        if (request.isLong) {
            val text = (1..4).joinToString("\n\n") { "$it 段落目の長い文章です。決まった文を返しています(モック)。" }
            return AiTextResult.normalize(text, null, null, request)
        }
        if (request.isTransform) {
            // 要約など: classify は最初の候補、それ以外は決まった文
            val text = if (request.task == "classify") request.options?.firstOrNull() else "${request.task}の結果です(モック)"
            return AiTextResult.normalize(text, null, null, request)
        }
        return when (request.shape) {
            AiTextShape.LINE -> AiTextResult.normalize("今日もよくがんばりました。少し休んでいきましょう(モック)", null, null, request)
            AiTextShape.LIST -> AiTextResult.normalize(null, ideas.take(request.resolvedCount), null, request)
            AiTextShape.FIELDS -> AiTextResult.normalize(null, null, request.items.orEmpty().map { AiTextField(it, "${it}の案(モック)") }, request)
        }
    }
}
