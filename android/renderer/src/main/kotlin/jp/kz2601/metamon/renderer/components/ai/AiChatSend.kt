package jp.kz2601.metamon.renderer.components.ai

import jp.kz2601.metamon.dsl.AIChatSaveTo
import jp.kz2601.metamon.dsl.Component
import jp.kz2601.metamon.dsl.Length
import jp.kz2601.metamon.dsl.ai.AiChat
import jp.kz2601.metamon.dsl.ai.AiChatMaterial
import jp.kz2601.metamon.dsl.ai.AiChatSaveToRules
import jp.kz2601.metamon.dsl.ai.AiSaveTo
import jp.kz2601.metamon.dsl.ai.AiTextError
import jp.kz2601.metamon.dsl.ai.AiTextMaterial
import jp.kz2601.metamon.dsl.ai.chatSaveTo
import jp.kz2601.metamon.dsl.records.Entity
import jp.kz2601.metamon.dsl.records.Field
import jp.kz2601.metamon.dsl.records.FieldCoding
import jp.kz2601.metamon.dsl.records.JSONValue
import jp.kz2601.metamon.dsl.records.Record
import jp.kz2601.metamon.renderer.DSLRuntime
import jp.kz2601.metamon.renderer.R
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.delay
import kotlinx.coroutines.withContext

/**
 * aiChat の 1 往復(iOS の `AiChatLeaf.send` / `saveReply` / `addSuggestion`)。Compose に依らないので JVM テストから呼ぶ。
 * 送る = ホストの [AiHost.generateText](task chat・同意 → Pro の確認もホスト)。発言と返事を entity に 1 件ずつ保存する。
 * roleField がある: 自分の発言を先に保存(返事に失敗しても残る)。無い(古い DSL): 返事が来てから 2 件を対で保存する。
 * 発言・返事の本文はログに出さない。
 */
object AiChatSend {
    /** 部品の設定(DSL の値)。 */
    data class Spec(
        val entity: String,
        val textField: String,
        val roleField: String?,
        val persona: String,
        val long: Boolean,
        val saveTo: AIChatSaveTo?,
        val inputEntity: String?,
        val limit: Int?,
    ) {
        companion object {
            /** entity / textField の無い aiChat は描かない(null)。 */
            fun of(c: Component): Spec? {
                val entity = c.entity ?: return null
                val textField = c.textField ?: return null
                return Spec(
                    entity = entity,
                    textField = textField,
                    roleField = c.roleField,
                    persona = c.persona.orEmpty(),
                    long = c.length == Length.LONG,
                    saveTo = c.chatSaveTo,
                    inputEntity = c.inputEntity,
                    limit = c.limit,
                )
            }
        }
    }

    sealed interface Outcome {
        /** 返事を保存できた。[turnId] は返事の吹き出し(読み上げ・案の位置)。見つからなければ null */
        data class Sent(val reply: String, val turnId: String?, val suggestion: AiChatSuggestion?) : Outcome

        /** 失敗。[restore] なら送った文を入力欄に戻す(何も残っていない) */
        data class Failed(val message: AiMessage, val restore: Boolean) : Outcome
    }

    /** 表示と送る履歴に使う記録(自分の分だけ)。 */
    fun ownRecords(rt: DSLRuntime, entity: Entity): List<Record> =
        AiChat.own(rt.recordsFlow?.value?.get(entity.id).orEmpty(), rt.currentUid(), entity.isShared)

    fun turns(rt: DSLRuntime, spec: Spec): List<AiChat.Turn> {
        val entity = rt.entityModel(spec.entity) ?: return emptyList()
        return AiChat.turns(ownRecords(rt, entity), spec.textField, spec.roleField)
    }

    /** 新しいフォームと同じ既定値(本文・役の field は除く・iOS の `ChatThreadLeaf.defaultData`)。 */
    fun base(entity: Entity, skipping: Set<String>): Map<String, JSONValue> =
        entity.storedFields.filter { it.id !in skipping }
            .mapNotNull { f -> FieldCoding.defaultValue(f).takeIf { !it.isNull }?.let { f.id to it } }
            .toMap()

    /**
     * 1 往復。[onPending] は roleField の無い時の「返事待ちの自分の発言」(null で消す)。
     * [text] は送る文(前後の空白は落とす・空なら何もしない)。
     */
    suspend fun send(
        rt: DSLRuntime,
        spec: Spec,
        text: String,
        locale: String,
        onPending: (String?) -> Unit = {},
    ): Outcome {
        val body = text.trim()
        val entity = rt.entityModel(spec.entity) ?: return Outcome.Failed(AiMessage(R.string.ai_chat_save_failed), restore = true)
        val host = rt.ai
        if (body.isEmpty()) return Outcome.Failed(AiMessage(R.string.ai_chat_save_failed), restore = false)
        if (host == null || !host.supportsOnlineText) return Outcome.Failed(AiMessage(R.string.ai_chat_offline), restore = true)
        val roleField = spec.roleField
        val base = base(entity, setOfNotNull(spec.textField, roleField))
        val history = AiChat.turns(ownRecords(rt, entity), spec.textField, roleField)
        val saveEntity = spec.saveTo?.let { rt.entityModel(it.entity) }
        val saveRequest = if (spec.saveTo != null && saveEntity != null) AiChatSaveToRules.request(spec.saveTo, saveEntity) else null
        if (roleField != null) {
            // 自分の発言を先に残す(返事に失敗しても消えない・次の送信で続けて読まれる)
            val mine = AiChat.record(body, AiChat.Role.USER, spec.textField, roleField, base)
            if (mine == null || !rt.addRecord(entity.id, mine)) return Outcome.Failed(AiMessage(R.string.ai_chat_save_failed), restore = true)
        } else {
            onPending(body)
        }
        val reply = try {
            val material = spec.inputEntity?.let { material(rt, it, spec.limit, body) }
            val request = AiChat.request(spec.persona, history, body, spec.long, locale, saveRequest, material)
            host.generateText(request)
        } catch (e: CancellationException) {
            throw e
        } catch (e: Exception) {
            if (roleField == null) onPending(null)
            return Outcome.Failed(message(e), restore = roleField == null)
        }
        onPending(null)
        if (!saveReply(rt, reply.text, body, entity.id, spec.textField, roleField, base)) {
            // 対で残さなかった = 自分の発言も消えている
            val msg = if (roleField == null) R.string.ai_chat_save_failed else R.string.ai_chat_reply_save_failed
            return Outcome.Failed(AiMessage(msg), restore = roleField == null)
        }
        val turn = AiChat.spokenReply(AiChat.turns(ownRecords(rt, entity), spec.textField, roleField), reply.text)
        val suggest = reply.suggest?.mapValues { JSONValue.from(it.value) }
        val suggestion = turn?.let {
            AiChatSuggestion.make(it.id, suggest, spec.saveTo, saveEntity) { v, f -> FieldCoding.display(v, f) }
        }
        return Outcome.Sent(reply.text, turn?.id, suggestion)
    }

    /** inputEntity: 自分の記録を「参考の記録」に(今の発言に近い記録から)。記録が多いと選びに時間がかかるので外のスレッドで。 */
    private suspend fun material(rt: DSLRuntime, inputEntity: String, limit: Int?, query: String): AiTextMaterial? {
        val input = rt.entityModel(inputEntity) ?: return null
        rt.loadRecords(setOf(input.id))
        val records = rt.recordsFlow?.value?.get(input.id).orEmpty()
        val uid = rt.currentUid()
        return withContext(Dispatchers.Default) { AiChatMaterial.request(input, records, uid, limit, query) }
    }

    /**
     * 返事が届いた後の保存。保存できたら true。
     * roleField がある: 自分の発言は保存済みなので返事だけ。無い: 自分 → AI を対で保存し、片方だけは残さない。
     */
    suspend fun saveReply(
        rt: DSLRuntime,
        reply: String,
        text: String,
        entity: String,
        textField: String,
        roleField: String?,
        base: Map<String, JSONValue>,
    ): Boolean {
        val ai = AiChat.record(reply, AiChat.Role.ASSISTANT, textField, roleField, base) ?: return false
        if (roleField != null) return rt.addRecord(entity, ai)
        val mine = AiChat.record(text, AiChat.Role.USER, textField, null, base) ?: return false
        return saveTurnPair(rt, entity, mine, ai)
    }

    /** 自分の発言と AI の返事を続けて保存する。[AiChatPairSaving] ならホストが対で扱う。 */
    suspend fun saveTurnPair(rt: DSLRuntime, entity: String, user: Map<String, JSONValue>, assistant: Map<String, JSONValue>): Boolean {
        (rt as? AiChatPairSaving)?.let { return it.saveAiChatPair(entity, user, assistant) }
        if (!rt.addRecord(entity, user)) return false
        delay(5) // 作成時刻で並べるので、自分 → AI の順を同じ時刻にしない
        return rt.addRecord(entity, assistant)
    }

    /** 記録の案の「追加」: いつもの保存で 1 件足す。entity の上限に達していれば足さない。null は成功。 */
    suspend fun addSuggestion(rt: DSLRuntime, s: AiChatSuggestion): AiMessage? {
        rt.loadRecords(setOf(s.entity))
        val existing = rt.recordsFlow?.value?.get(s.entity).orEmpty().size
        if (AiSaveTo.room(existing, 1) == 0) return AiMessage(R.string.ai_chat_suggest_full)
        return if (rt.addRecord(s.entity, s.data)) null else AiMessage(R.string.ai_chat_suggest_failed)
    }

    /** 画面に出す文。Pro が要る時は aiChat の言い方にする。参加者には課金の案内を出さない(#704)。 */
    fun message(e: Throwable): AiMessage = when (e) {
        AiTextError.NeedsPro -> AiMessage(R.string.ai_chat_needs_pro)
        AiTextError.CreatorNeedsPro -> AiMessage(R.string.ai_chat_creator_needs_pro)
        else -> AiCopy.error(e)
    }
}

/**
 * roleField の無い aiChat の 1 往復を対で保存するホスト(iOS の `AIChatPairSaving`)。片方を保存できなければ何も残さない。
 * 持たないランタイムは addRecord を 2 回。
 */
interface AiChatPairSaving {
    suspend fun saveAiChatPair(entity: String, user: Map<String, JSONValue>, assistant: Map<String, JSONValue>): Boolean
}

/** saveTo の記録の案: 最新の AI の返事の下に出すカード。保存は「追加」を押した時だけ。 */
data class AiChatSuggestion(
    /** 案が付いた AI の発言(この吹き出しの下に出す) */
    val turnId: String,
    val entity: String,
    /** 保存する data(案の値 + ほかの field の既定値) */
    val data: Map<String, JSONValue>,
    /** カードに出す行(field の名前・表示する値) */
    val rows: List<Pair<String, String>>,
    val state: SaveState = SaveState.Idle,
) {
    sealed interface SaveState {
        data object Idle : SaveState
        data object Saving : SaveState
        data object Saved : SaveState
        data class Failed(val message: AiMessage) : SaveState
    }

    companion object {
        /** 返事の案から作る。合う値が無い・required が埋まらない時は null(カードを出さない)。 */
        fun make(
            turnId: String,
            suggest: Map<String, JSONValue>?,
            saveTo: AIChatSaveTo?,
            entity: Entity?,
            display: (JSONValue, Field) -> String,
        ): AiChatSuggestion? {
            if (suggest == null || saveTo == null || entity == null) return null
            val data = AiChatSaveToRules.record(saveTo, suggest, entity) ?: return null
            val rows = AiChatSaveToRules.values(saveTo, suggest, entity).map { (f, v) -> f.name to display(v, f) }
            return AiChatSuggestion(turnId, entity.id, data, rows)
        }
    }
}

/** 入力欄のマイク(話して入力)の状態(iOS の `AiChatMic`)。 */
enum class AiChatMic {
    /** ホストが話して入力を持たない */
    HIDDEN,

    /** 押すと録音を始める */
    START,

    /** 録音を始めている(二重に始めない) */
    STARTING,

    /** 録音中。押すと止めて文字にする */
    STOP,

    /** 文字にしている */
    TRANSCRIBING,

    /** 押せない(モデルの準備中・マイク FAB の音声入力中・AI と話せない版) */
    BUSY,

    /** モデルが無い・失敗した。押すと用意を始める */
    PREPARE,
    ;

    val enabled: Boolean get() = this == START || this == STOP || this == PREPARE

    companion object {
        fun state(phase: DictationPhase, readiness: DictationReadiness?, voiceInputActive: Boolean, online: Boolean): AiChatMic {
            readiness ?: return HIDDEN
            when (phase) {
                DictationPhase.STARTING -> return STARTING
                DictationPhase.LISTENING -> return STOP
                DictationPhase.TRANSCRIBING -> return TRANSCRIBING
                DictationPhase.IDLE -> Unit
            }
            if (!online || voiceInputActive) return BUSY
            return when (readiness) {
                DictationReadiness.READY -> START
                DictationReadiness.PREPARING -> BUSY
                DictationReadiness.NEEDS_MODEL, DictationReadiness.FAILED -> PREPARE
            }
        }

        /** 録音・文字起こしの間は送らない(聞き取った文が入る前に送ってしまう)。 */
        fun canSend(online: Boolean, sending: Boolean, draft: String, phase: DictationPhase): Boolean =
            online && !sending && phase == DictationPhase.IDLE && draft.isNotBlank()

        /** 吹き出しのスピーカー: 録音・文字起こしの間は押せない(読み上げの声をマイクが拾う)。 */
        fun canSpeak(phase: DictationPhase): Boolean = phase == DictationPhase.IDLE
    }
}

enum class DictationPhase { IDLE, STARTING, LISTENING, TRANSCRIBING }

enum class DictationReadiness { READY, PREPARING, NEEDS_MODEL, FAILED }

/** 届いた返事の自動の読み上げ(`speak: true`)。 */
enum class AiChatAutoRead {
    NONE,
    SPEAK,
    ;

    companion object {
        /** 画面を離れた後・話して入力の録音中は読まない。 */
        fun mode(speak: Boolean?, visible: Boolean, recording: Boolean): AiChatAutoRead =
            if (speak == true && visible && !recording) SPEAK else NONE
    }
}

/** 話して入力の文を入力欄に足す(iOS の `DictationText.appending`)。 */
fun appendDictation(text: String, draft: String): String {
    val t = text.trim()
    if (t.isEmpty()) return draft
    if (draft.isBlank()) return t
    return if (draft.endsWith("\n")) draft + t else draft + "\n" + t
}
