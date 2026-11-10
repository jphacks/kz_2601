package jp.kz2601.metamon.dsl.ai

import jp.kz2601.metamon.dsl.logic.RecordImport
import jp.kz2601.metamon.dsl.records.Entity
import jp.kz2601.metamon.dsl.records.FieldCoding
import jp.kz2601.metamon.dsl.records.FieldType
import jp.kz2601.metamon.dsl.records.JSONValue
import kotlinx.serialization.Serializable
import kotlinx.serialization.json.JsonElement
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive
import kotlinx.serialization.json.contentOrNull

/**
 * aiButton の依頼と答え(iOS の MetamonDSL/Model/AIText.swift・AISaveTo.swift)。
 * 依頼は `POST /v1/text` の body と同じ形で、端末内 AI にも使う。写真・会話(aiChat)・outputs の書き方はまだ持たない。
 */
@Serializable
enum class AiTextShape {
    @kotlinx.serialization.SerialName("line")
    LINE,

    @kotlinx.serialization.SerialName("list")
    LIST,

    @kotlinx.serialization.SerialName("fields")
    FIELDS,
    ;

    companion object {
        fun of(wire: String?): AiTextShape? = when (wire) {
            "line" -> LINE
            "list" -> LIST
            "fields" -> FIELDS
            else -> null
        }
    }
}

/** 上限(DSL の検証・`POST /v1/text` と同じ値)。 */
object AiTextLimits {
    const val MAX_INSTRUCTION = 500
    const val MAX_ITEMS = 10
    const val MAX_INPUT_BYTES = 4096
    const val MAX_RECORDS = 20
    const val DEFAULT_COUNT = 3
    const val MAX_ITEM_CHARACTERS = 200
    const val MAX_TASK_CHARACTERS = 1200
    const val MAX_LONG_INPUT_BYTES = 16384
    const val MAX_LONG_CHARACTERS = 3000

    /** classify の候補の上限(backend `TextMaxOptions`)。 */
    const val MAX_OPTIONS = 20

    fun maxInputBytes(long: Boolean): Int = if (long) MAX_LONG_INPUT_BYTES else MAX_INPUT_BYTES

    /** UTF-8 で [maxBytes] 以下に切る(文字の途中では切らない)。 */
    fun clip(s: String, maxBytes: Int = MAX_INPUT_BYTES): String {
        if (s.toByteArray(Charsets.UTF_8).size <= maxBytes) return s
        val out = StringBuilder()
        var used = 0
        var i = 0
        while (i < s.length) {
            val cp = s.codePointAt(i)
            val ch = String(Character.toChars(cp))
            val n = ch.toByteArray(Charsets.UTF_8).size
            if (used + n > maxBytes) break
            out.append(ch)
            used += n
            i += Character.charCount(cp)
        }
        return out.toString()
    }
}

/** `POST /v1/text` の body(openapi `TextRequest`)。null は送らない。 */
@Serializable
data class AiTextRequest(
    val instruction: String,
    val shape: AiTextShape,
    /** list の個数(list 以外は null) */
    val count: Int? = null,
    /** fields の項目名(fields 以外は null) */
    val items: List<String>? = null,
    /** 材料(最近の記録・フォームの値) */
    val input: String? = null,
    /** ja | en */
    val locale: String? = null,
    /** summarize | classify | rewrite | extract | suggest のオンライン版(null = generate) */
    val task: String? = null,
    /** classify の候補 */
    val options: List<String>? = null,
    /** "long" = 長い文(オンライン AI だけ) */
    val length: String? = null,
    /** ボタンがあるアプリ(ホストが付ける) */
    val appId: String? = null,
) {
    /** 同じ依頼に appId を付ける(空なら付けない)。 */
    fun inApp(appId: String): AiTextRequest = copy(appId = appId.ifEmpty { null })

    val isLong: Boolean get() = length == "long"

    /** 入力を変える task(要約など)か。答えは 1 つの文で、output の field に入れる。 */
    val isTransform: Boolean get() = task != null && task != "generate"

    /** list の実際の個数(既定 3・1〜10)。 */
    val resolvedCount: Int get() = (count ?: AiTextLimits.DEFAULT_COUNT).coerceIn(1, AiTextLimits.MAX_ITEMS)
}

@Serializable
data class AiTextField(val name: String, val value: String)

/** 形に合わせて整えた結果。[text] は記録の string / text にそのまま入れる形。 */
@Serializable
data class AiTextResult(
    val shape: AiTextShape,
    val text: String,
    val items: List<String>? = null,
    val fields: List<AiTextField>? = null,
) {
    companion object {
        /**
         * 生の出力を依頼の形に合わせる(backend の `parseTextOutput` と同じ規則)。合わなければ null。
         * line = 最初の空でない行 / list = 1 個以上(count を超えた分は切る)/ fields = 依頼の名前が全部そろう(依頼の順)。
         */
        fun normalize(text: String?, items: List<String>?, fields: List<AiTextField>?, request: AiTextRequest): AiTextResult? {
            if (request.isLong) return trimmed(text)?.let { AiTextResult(AiTextShape.LINE, it.take(AiTextLimits.MAX_LONG_CHARACTERS)) }
            if (request.isTransform) return transform(text, request)
            return when (request.shape) {
                AiTextShape.LINE -> text.orEmpty().lines().map(::clean).firstOrNull { it.isNotEmpty() }
                    ?.let { AiTextResult(AiTextShape.LINE, it) }
                AiTextShape.LIST -> items.orEmpty().map(::clean).filter { it.isNotEmpty() }.take(request.resolvedCount)
                    .takeIf { it.isNotEmpty() }
                    ?.let { list -> AiTextResult(AiTextShape.LIST, list.joinToString("\n") { "・$it" }, items = list) }
                AiTextShape.FIELDS -> {
                    val got = linkedMapOf<String, String>()
                    for (f in fields.orEmpty()) {
                        val v = clean(f.value)
                        if (v.isNotEmpty()) got.putIfAbsent(f.name.trim(), v)
                    }
                    val out = request.items.orEmpty().map { name -> AiTextField(name, got[name] ?: return null) }
                    if (out.isEmpty()) null else AiTextResult(AiTextShape.FIELDS, out.joinToString("\n") { "${it.name}: ${it.value}" }, fields = out)
                }
            }
        }

        /** 要約などの答え(backend `parseTaskOutput`)。classify は候補そのものか、候補をちょうど 1 つ含む答えだけ。 */
        private fun transform(text: String?, request: AiTextRequest): AiTextResult? {
            val t = trimmed(text) ?: return null
            if (request.task == "classify") {
                val options = request.options.orEmpty()
                if (t in options) return AiTextResult(AiTextShape.LINE, t)
                val hits = options.filter { t.contains(it) }
                return if (hits.size == 1) AiTextResult(AiTextShape.LINE, hits[0]) else null
            }
            return AiTextResult(AiTextShape.LINE, t.take(AiTextLimits.MAX_TASK_CHARACTERS))
        }

        private val QUOTES = charArrayOf('"', '「', '」')

        private fun trimmed(text: String?): String? =
            text.orEmpty().trim { it.isWhitespace() || it in QUOTES }.ifEmpty { null }

        private val LEADING_MARK = Regex("""^(?:[-*・•●]|\d+[.)、.])\s*""")

        /** 前後の空白・行頭の記号や番号・かぎかっこを落とし、長すぎれば切る。 */
        internal fun clean(s: String): String {
            val t = LEADING_MARK.replaceFirst(s.trim(), "")
            return t.trim { it.isWhitespace() || it in QUOTES }.take(AiTextLimits.MAX_ITEM_CHARACTERS)
        }
    }
}

/**
 * generate / 要約などの失敗の理由。文言は描画器(strings_ai.xml)が kind ごとに出す。
 * [Failed] はサーバーの文をそのまま出す(利用上限など)。
 */
sealed class AiTextError(message: String) : Exception(message) {
    /** オンライン版が無い(プレビュー・テスト) */
    data object Unsupported : AiTextError("unsupported")

    /** AI へのデータ送信に同意していない */
    data object NeedsConsent : AiTextError("needs_consent")

    /** Tailor It Pro が要る(402 pro_required) */
    data object NeedsPro : AiTextError("needs_pro")

    /** 共有のアプリの作成者の Pro が要る(402 pro_required + creator_needs_pro。課金画面は出さない) */
    data object CreatorNeedsPro : AiTextError("creator_needs_pro")

    /** AI が書くのを断った(refused) */
    data object Refused : AiTextError("refused")

    /** 答えが形に合わない(bad_output) */
    data object BadOutput : AiTextError("bad_output")

    /** 運営が AI を一時停止(503 ai_paused) */
    data object Paused : AiTextError("ai_paused")

    /** 材料が多すぎる(413) */
    data object TooLarge : AiTextError("payload_too_large")

    /** その他(サーバーのメッセージ・通信の失敗) */
    class Failed(val detail: String) : AiTextError(detail)
}

/**
 * `aiButton.saveTo`: generate の list の 1 項目 = entity の 1 件(field に入れる)。
 * 保存は利用者が押した時だけ(「全部保存」か 1 つずつ)。検証は backend `dsl/ai_save_to.go`。
 */
data class AiSaveTo(val entity: String, val field: String) {
    /** 1 項目の記録の data。field が entity の string / text でなければ null。ほかの field は新しいフォームと同じ既定値。 */
    fun record(item: String, entity: Entity): Map<String, JSONValue>? {
        if (entity.id != this.entity) return null
        val f = entity.storedFields.firstOrNull { it.id == field && (it.type == FieldType.STRING || it.type == FieldType.TEXT) } ?: return null
        val text = item.trim().ifEmpty { return null }
        val limit = RecordImport.characterLimit(f)
        val clipped = if (text.codePointCount(0, text.length) > limit) text.substring(0, text.offsetByCodePoints(0, limit)) else text
        val data = linkedMapOf<String, JSONValue>(f.id to JSONValue.Str(clipped))
        for (x in entity.storedFields) {
            if (x.id in data) continue
            val d = FieldCoding.defaultValue(x)
            if (!d.isNull) data[x.id] = d
        }
        return data
    }

    companion object {
        /** DSL の `saveTo`(`{entity, field}`)。形が違えば null。 */
        fun from(e: JsonElement?): AiSaveTo? {
            val o = e as? JsonObject ?: return null
            val entity = (o["entity"] as? JsonPrimitive)?.takeIf { it.isString }?.contentOrNull ?: return null
            val field = (o["field"] as? JsonPrimitive)?.takeIf { it.isString }?.contentOrNull ?: return null
            return AiSaveTo(entity, field)
        }

        /** 今回足せる件数: entity の上限までの残り。 */
        fun room(existing: Int, wanted: Int): Int = maxOf(0, minOf(wanted, RecordImport.MAX_RECORDS_PER_ENTITY - existing))
    }
}

/** aiButton.prompt の `{{id}}`(Go の `placeholderRe` と同じ・括弧の内側の空白は許す)。 */
object AiPlaceholder {
    private val REGEX = Regex("""\{\{[\t\n\u000C\r ]*([A-Za-z0-9_-]+)[\t\n\u000C\r ]*\}\}""")

    fun ids(template: String): Set<String> = REGEX.findAll(template).map { it.groupValues[1] }.toSet()

    /** 1 回の走査で置き換える(置換後の文字は再展開しない)。[value] が null の id は文字のまま残す。 */
    fun replace(template: String, value: (String) -> String?): String =
        REGEX.replace(template) { m -> value(m.groupValues[1]) ?: m.value }
}
