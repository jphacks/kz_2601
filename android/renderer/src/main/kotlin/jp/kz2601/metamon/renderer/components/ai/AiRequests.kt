package jp.kz2601.metamon.renderer.components.ai

import jp.kz2601.metamon.dsl.Component
import jp.kz2601.metamon.dsl.Length
import jp.kz2601.metamon.dsl.Task
import jp.kz2601.metamon.dsl.ai.AiPlaceholder
import jp.kz2601.metamon.dsl.ai.AiTextLimits
import jp.kz2601.metamon.dsl.ai.AiTextRequest
import jp.kz2601.metamon.dsl.ai.AiTextShape
import jp.kz2601.metamon.dsl.records.Entity
import jp.kz2601.metamon.dsl.records.Field
import jp.kz2601.metamon.dsl.records.FieldCoding
import jp.kz2601.metamon.dsl.records.FieldType
import jp.kz2601.metamon.dsl.records.JSONValue
import jp.kz2601.metamon.dsl.records.Record

/*
 * aiButton の純粋な部分(iOS の OnDeviceAI.swift / AIGenerate.swift / AIOnlineTask.swift / AIInputRecords.swift)。
 * どこで動かすか・何を渡すか・どの記録を渡すかを決める。画面の文言は AiButtonLeaf が理由ごとに出す。
 */

/** 組み立て・記録の選び方の失敗。利用者の入力待ち([EMPTY_INPUT] / [NO_RECORDS])は端末内 AI の失敗に数えない。 */
class AiBuildError(val kind: Kind) : Exception(kind.name) {
    enum class Kind {
        /** task が無い・知らない(設定の誤り) */
        UNKNOWN_TASK,

        /** 先に入力してください */
        EMPTY_INPUT,

        /** 分類の候補が無い */
        NO_OPTIONS,

        /** generate の指示が無い */
        NO_INSTRUCTION,

        /** 先に記録を追加してください */
        NO_RECORDS,

        /** クラウドに渡せる自分の記録が無い */
        NO_OWN_RECORDS,
    }

    val isWaitingForInput: Boolean get() = kind == Kind.EMPTY_INPUT || kind == Kind.NO_RECORDS
}

/** 端末内 AI を使えるか(iOS の `AIAvailability`)。 */
sealed interface OnDeviceAvailability {
    data object Available : OnDeviceAvailability

    data class Unavailable(val reason: Reason) : OnDeviceAvailability

    enum class Reason(val metric: String) {
        /** 端末内 AI(Gemini Nano)に対応していない端末 */
        DEVICE_NOT_ELIGIBLE("deviceNotEligible"),

        /** 対応端末だが設定で OFF */
        NOT_ENABLED("appleIntelligenceOff"),

        /** モデルのダウンロード・準備中 */
        MODEL_NOT_READY("modelNotReady"),

        /** OS が古い */
        OS_TOO_OLD("osTooOld"),

        OTHER("other"),
    }

    val metric: String get() = when (this) {
        Available -> "available"
        is Unavailable -> reason.metric
    }
}

/** どこで書くか(iOS の `AIGenerateRoute`)。 */
sealed interface AiRoute {
    data object OnDevice : AiRoute

    data object Online : AiRoute

    data class Unavailable(val reason: UnavailableReason) : AiRoute

    sealed interface UnavailableReason {
        data class Device(val reason: OnDeviceAvailability.Reason) : UnavailableReason

        /** length long はオンラインだけ */
        data object LongOnlyOnline : UnavailableReason

        /** inputPhoto はオンラインだけ */
        data object PhotoOnlyOnline : UnavailableReason
    }

    companion object {
        /**
         * - mock: モックの構成はホストの決まった文を使う(端末内 AI の答えは毎回変わるため)
         * - long / photo: 端末内 AI を使わず、いつもオンライン(端末内 AI は短い答え向け・写真を読めない)
         */
        fun choose(availability: OnDeviceAvailability, hostOnline: Boolean, mock: Boolean, long: Boolean = false, photo: Boolean = false): AiRoute {
            if (photo) return if (hostOnline) Online else Unavailable(UnavailableReason.PhotoOnlyOnline)
            if (long) return if (hostOnline) Online else Unavailable(UnavailableReason.LongOnlyOnline)
            if (mock && hostOnline) return Online
            if (availability == OnDeviceAvailability.Available) return OnDevice
            if (hostOnline) return Online
            val reason = (availability as? OnDeviceAvailability.Unavailable)?.reason ?: OnDeviceAvailability.Reason.OTHER
            return Unavailable(UnavailableReason.Device(reason))
        }
    }
}

/** 端末内 AI に渡す 1 回分の指示(要約・分類など)。 */
data class AiOnDeviceRequest(
    val task: Task,
    /** セッションの instructions(役割・出力形式) */
    val instructions: String,
    /** 入力本文を含むプロンプト */
    val prompt: String,
    /** classify の候補(それ以外は空) */
    val options: List<String>,
)

/** 要約・分類・書き直し・抜き出し・提案の依頼(iOS の `AIRequestBuilder`)。 */
object AiRequestBuilder {
    /** 端末内モデルのコンテキストに収める入力の上限(文字)。 */
    const val MAX_INPUT_CHARACTERS = 3000
    const val DEFAULT_LIMIT = 20
    const val MAX_LIMIT = 50

    private val transformTasks = setOf(Task.SUMMARIZE, Task.CLASSIFY, Task.REWRITE, Task.EXTRACT, Task.SUGGEST)

    private fun task(c: Component): Task = c.task?.takeIf { it in transformTasks } ?: throw AiBuildError(AiBuildError.Kind.UNKNOWN_TASK)

    private fun options(c: Component, entity: Entity): List<String> =
        c.options.ifEmpty { c.output?.let { entity.field(it)?.options }.orEmpty() }

    /** 端末内 AI の依頼。 */
    fun build(c: Component, entity: Entity, values: Map<String, JSONValue>, sourceEntity: Entity? = null, records: List<Record> = emptyList()): AiOnDeviceRequest {
        val task = task(c)
        val options = if (task == Task.CLASSIFY) options(c, entity).ifEmpty { throw AiBuildError(AiBuildError.Kind.NO_OPTIONS) } else emptyList()
        val input = inputText(c, entity, values, sourceEntity, records).take(MAX_INPUT_CHARACTERS)
        if (input.isBlank()) throw AiBuildError(AiBuildError.Kind.EMPTY_INPUT)
        val template = c.prompt.orEmpty()
        var prompt = if (template.isNotEmpty()) {
            expand(template, input, entity, values) + if (embedsInput(template, c, sourceEntity)) "" else "\n\n$input"
        } else {
            input
        }
        if (task == Task.CLASSIFY) prompt += "\n\n候補: " + options.joinToString(" / ")
        return AiOnDeviceRequest(task, instructions(task), prompt, options)
    }

    /** `POST /v1/text` の依頼(iOS の `AIRequestBuilder.online`)。材料は 4 KB(long は 16 KB)に切り、`{{input}}` は目印に置き換える。 */
    fun online(c: Component, entity: Entity, values: Map<String, JSONValue>, sourceEntity: Entity? = null, records: List<Record> = emptyList(), locale: String): AiTextRequest {
        val task = task(c)
        var options: List<String>? = null
        if (task == Task.CLASSIFY) {
            val list = options(c, entity)
            if (list.size < 2) throw AiBuildError(AiBuildError.Kind.NO_OPTIONS)
            options = list.take(AiTextLimits.MAX_OPTIONS)
        }
        val long = c.length == Length.LONG && task != Task.CLASSIFY
        val input = AiTextLimits.clip(inputText(c, entity, values, sourceEntity, records), AiTextLimits.maxInputBytes(long))
        val hasText = input.isNotBlank()
        if (!hasText && c.inputPhoto == null) throw AiBuildError(AiBuildError.Kind.EMPTY_INPUT)
        val template = c.prompt.orEmpty()
        val instruction = if (template.isEmpty()) "" else expand(template, marker(locale), entity, values).trim().take(AiTextLimits.MAX_INSTRUCTION)
        return AiTextRequest(
            instruction = instruction, shape = AiTextShape.LINE, input = if (hasText) input else null, locale = locale,
            task = task.wire, options = options, length = if (long) "long" else null,
        )
    }

    fun instructions(task: Task): String {
        val common = "入力と同じ言語で、結果だけを返してください。前置き・引用符・説明は不要です。"
        return when (task) {
            Task.SUMMARIZE -> "あなたは要約アシスタントです。与えられた文章の要点を短くまとめてください。$common"
            Task.CLASSIFY -> "あなたは分類アシスタントです。与えられた文章に最も当てはまる候補を 1 つだけ選んでください。"
            Task.REWRITE -> "あなたは文章の清書アシスタントです。意味を変えずに、読みやすく自然な文章に書き直してください。$common"
            Task.EXTRACT -> "あなたは情報抽出アシスタントです。与えられた文章から重要な点や値を短い箇条書きで抜き出してください。$common"
            else -> "あなたは前向きなアシスタントです。与えられた内容を踏まえて、短い一言や提案を 1 つだけ返してください。$common"
        }
    }

    /** AI に渡す本文: entity のレコード群 / input field の値 / フォーム全体(output 以外)。 */
    fun inputText(c: Component, entity: Entity, values: Map<String, JSONValue>, sourceEntity: Entity?, records: List<Record>): String {
        if (sourceEntity != null) {
            val n = (c.limit ?: DEFAULT_LIMIT).coerceIn(1, MAX_LIMIT)
            return AiInputRecords.recent(records).take(n).joinToString("\n") { r ->
                "- " + describe(sourceEntity.storedFields.filter { it.id != c.output }, { r.data[it] ?: JSONValue.Null }, " / ")
            }
        }
        c.input?.let { return FieldCoding.display(values[it] ?: JSONValue.Null, entity.field(it)) }
        return describe(entity.storedFields.filter { it.id != c.output }, { values[it] ?: JSONValue.Null }, "\n")
    }

    /** テンプレが本文を含むか: `{{input}}`、または input field 自身の `{{fieldId}}`(entity 入力でない時)。 */
    fun embedsInput(template: String, c: Component, sourceEntity: Entity?): Boolean {
        val ids = AiPlaceholder.ids(template)
        return "input" in ids || (sourceEntity == null && c.input != null && c.input in ids)
    }

    /** `{{input}}` → 本文、`{{fieldId}}` → フォームの値。 */
    fun expand(template: String, input: String, entity: Entity, values: Map<String, JSONValue>): String =
        AiPlaceholder.replace(template) { id ->
            if (id == "input") input else entity.fields.firstOrNull { it.id == id }?.let { FieldCoding.display(values[it.id] ?: JSONValue.Null, it) }
        }

    /** 「名前: 値」を並べる(写真・動画・音声・位置・空は落とす)。 */
    fun describe(fields: List<Field>, value: (String) -> JSONValue, separator: String): String =
        fields.mapNotNull { f ->
            if (f.type in MEDIA) return@mapNotNull null
            FieldCoding.display(value(f.id), f).takeIf { it.isNotEmpty() }?.let { "${f.name}: $it" }
        }.joinToString(separator)

    /** モデルの返答を output に入れる形に整える。classify は候補に寄せる(候補外なら null)。 */
    fun normalize(raw: String, request: AiOnDeviceRequest): String? {
        val s = raw.trim { it.isWhitespace() || it == '"' || it == '「' || it == '」' }
        if (request.task != Task.CLASSIFY) return s.ifEmpty { null }
        if (s in request.options) return s
        return request.options.firstOrNull { s.contains(it) }
    }

    internal fun marker(locale: String): String = if (locale == "en") "the data below" else "下の材料"

    private val MEDIA = setOf(FieldType.PHOTO, FieldType.VIDEO, FieldType.AUDIO, FieldType.LOCATION)
}

/** `task: "generate"` の依頼(iOS の `AIGenerateBuilder`)。 */
object AiGenerateBuilder {
    fun build(c: Component, form: Entity?, values: Map<String, JSONValue>, sourceEntity: Entity?, records: List<Record>, locale: String): AiTextRequest {
        val instruction = AiPlaceholder.replace(c.prompt.orEmpty()) { id ->
            if (id == "input") AiRequestBuilder.marker(locale)
            else form?.fields?.firstOrNull { it.id == id }?.let { FieldCoding.display(values[it.id] ?: JSONValue.Null, it) }
        }.trim().take(AiTextLimits.MAX_INSTRUCTION)
        if (instruction.isEmpty()) throw AiBuildError(AiBuildError.Kind.NO_INSTRUCTION)

        var input: String? = null
        if (sourceEntity != null) {
            val n = (c.limit ?: AiTextLimits.MAX_RECORDS).coerceIn(1, AiTextLimits.MAX_RECORDS)
            val recent = AiInputRecords.recent(records).take(n)
            if (recent.isEmpty()) throw AiBuildError(AiBuildError.Kind.NO_RECORDS)
            input = recent.joinToString("\n") { r ->
                "- " + AiRequestBuilder.describe(sourceEntity.storedFields, { r.data[it] ?: JSONValue.Null }, " / ")
            }
        } else if (c.input != null && form != null) {
            input = FieldCoding.display(values[c.input!!] ?: JSONValue.Null, form.field(c.input!!))
        }
        val long = c.length == Length.LONG
        input = input?.let { AiTextLimits.clip(it, AiTextLimits.maxInputBytes(long)) }?.takeIf { it.isNotBlank() }

        val shape = AiTextShape.of(c.shape?.wire) ?: AiTextShape.LINE
        return AiTextRequest(
            instruction = instruction,
            shape = shape,
            count = if (shape == AiTextShape.LIST) (c.count ?: AiTextLimits.DEFAULT_COUNT).coerceIn(1, AiTextLimits.MAX_ITEMS) else null,
            items = if (shape == AiTextShape.FIELDS) c.items.take(AiTextLimits.MAX_ITEMS) else null,
            input = input,
            locale = locale,
            length = if (long) "long" else null,
        )
    }
}

/**
 * aiButton の `entity` 入力で AI に渡す記録(iOS の `AIInputRecords`)。
 * 並びは iOS と同じ「更新日時(無ければ作成日時)の新しい順」。:dsl の Aggregator.sorted は作成日時だけなので使わない。
 * クラウド(オンライン版)へは shared / public なら自分が書いた記録だけ・最近の 20 件まで。端末内 AI は絞らない。
 */
object AiInputRecords {
    /** 更新日時(無ければ作成日時)の新しい順。 */
    fun recent(records: List<Record>): List<Record> = records.sortedByDescending { it.updatedAt ?: it.createdAt ?: "" }

    fun select(records: List<Record>, entity: Entity, route: AiRoute, uid: String?): List<Record> {
        if (route != AiRoute.Online) return records
        val own = if (entity.isShared) records.filter { isOwn(it, uid, shared = true) } else records
        if (own.isEmpty() && records.isNotEmpty()) throw AiBuildError(AiBuildError.Kind.NO_OWN_RECORDS)
        return recent(own).take(AiTextLimits.MAX_RECORDS)
    }

    /** author がサインイン中の uid と同じなら自分。author が無い時は private なら自分、shared / public は未送信の分だけ。 */
    fun isOwn(record: Record, uid: String?, shared: Boolean): Boolean {
        val author = record.author?.takeIf { it.isNotEmpty() } ?: return !shared || record.unsent
        if (uid.isNullOrEmpty()) return false
        return author == uid
    }
}
