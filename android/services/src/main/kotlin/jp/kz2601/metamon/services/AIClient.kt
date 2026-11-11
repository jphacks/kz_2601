package jp.kz2601.metamon.services

import jp.kz2601.metamon.dsl.AppDocument
import jp.kz2601.metamon.dsl.LenientEnumSerializer
import jp.kz2601.metamon.dsl.ai.AiTextError
import jp.kz2601.metamon.dsl.ai.AiTextRequest
import jp.kz2601.metamon.dsl.ai.AiTextResult
import kotlinx.serialization.Serializable
import kotlinx.serialization.json.JsonObject

/**
 * ai サービス(iOS の `AIClient`)。発話 → DSL の生成と、生成の前の相談(chat)。
 * 契約: docs/backend/openapi-ai.yaml・backend/internal/ai/generate.go / chat.go。
 */
interface AIClient {
    /**
     * `POST /v1/generate`。`currentVersion` が 0 以下なら baseVersion を送らない(サーバーの現行)。
     * 409 は現行を基準に 1 回だけやり直す。
     */
    suspend fun morph(
        transcript: String,
        appId: String,
        currentVersion: Int,
        locale: String? = null,
        mode: GenerateMode = GenerateMode.AUTO,
    ): MorphResult

    /**
     * 途中経過付きの生成(`?stream=1`・#636)。[progress] には届いた順に仮置きの経過を渡す。戻り値(最後の結果)が正。
     * SSE が使えない・途中で切れた時は [GenerateProgress.Restart](attempt 0)を出して通常の呼び方に戻る。
     */
    suspend fun morphStreaming(
        transcript: String,
        appId: String,
        currentVersion: Int,
        locale: String? = null,
        mode: GenerateMode = GenerateMode.AUTO,
        progress: (GenerateProgress) -> Unit,
    ): MorphResult

    /** `POST /v1/chat`(#74・#184)。手元の全履歴を毎回送る。 */
    suspend fun chat(appId: String, messages: List<ChatMessage>, locale: String? = null, mode: ChatMode? = null): ChatTurn

    /** `DELETE /v1/chat/{appId}`。失敗は握りつぶす(サーバー側は 30 分で消える)。 */
    suspend fun resetChat(appId: String)

    /**
     * `POST /v1/text`(#417): aiButton のオンライン版。端末内 AI が使えない時だけ呼ぶ。
     * 失敗は [AiTextError](402 pro_required → NeedsPro / CreatorNeedsPro など)。持たない実装は Unsupported。
     */
    suspend fun text(request: AiTextRequest): AiTextResult = throw AiTextError.Unsupported
}

/** generate の `mode`。iOS は auto だけを使う(「元に戻して」などはサーバーがキーワードで判定)。 */
enum class GenerateMode(val wire: String) {
    AUTO("auto"),
    CREATE("create"),
    EDIT("edit"),
    UNDO("undo"),
}

/**
 * 端末が描ける範囲の申告(#245・#482)。null は送らない(= 全部使える)。
 * アプリでは描画器の登録表から作る(:renderer の `DeviceCapabilities`・#845)。
 */
data class ClientCapabilities(
    val allowedComponents: List<String>? = null,
    val disabledFeatures: List<String>? = null,
    val supportedFeatures: List<String> = emptyList(),
)

/** `POST /v1/generate` の結果(iOS の `MorphResult`)。 */
data class MorphResult(
    val document: AppDocument,
    /** 変更の一文要約(UI に表示・読み上げ) */
    val summary: String? = null,
    /** full | patch(診断用) */
    val mode: String? = null,
    /** 生成しても何も変わらなかった(#211) */
    val unchanged: Boolean = false,
    /** #253 `meta.publicPersonalData`(誰でも見られる記録に入った個人の情報の欄) */
    val publicPersonalData: List<JsonObject> = emptyList(),
    /** appconfig の `DSLEnvelope` と同じ形の JSON(キャッシュ用・知らないキーも残る) */
    val envelopeJson: String,
)

/** 生成の途中経過(#636)。どれも仮置きで、最後の結果で置き直す。 */
sealed interface GenerateProgress {
    /** `event: app`(create)。アプリ名とタブの見出し */
    data class App(val name: String, val tabs: List<String>, val attempt: Int) : GenerateProgress

    /** `event: summary`(edit)。読み上げ用の仮の一文。iOS は使っていない */
    data class Summary(val text: String, val attempt: Int) : GenerateProgress

    /** `event: component`。最初に見える画面の部品 1 つ */
    data class Component(val component: StreamedComponent) : GenerateProgress

    /** `event: restart`。それまでの仮置きを捨てる。端末側でやり直す時(stream を諦めた・409)は attempt 0 */
    data class Restart(val attempt: Int) : GenerateProgress
}

/** `event: component` の部品(backend `ComponentEvent` + attempt / screen)。 */
data class StreamedComponent(
    val attempt: Int,
    val screen: String = "",
    /** screens[].components の位置 */
    val index: Int,
    val id: String? = null,
    val type: String,
    val area: String? = null,
    /** label | title | text | prompt の最初の値 */
    val label: String? = null,
)

// MARK: - 相談(chat)

@Serializable
enum class ChatMode {
    @kotlinx.serialization.SerialName("create")
    CREATE,

    @kotlinx.serialization.SerialName("edit")
    EDIT,
}

/** iOS が表示できる追加の kind(#184)。backend はここに無い kind を返さない。 */
object ChatCapability {
    val all: List<String> = listOf("answer", "proposal")
}

@Serializable
data class ChatMessage(val role: Role, val text: String) {
    @Serializable
    enum class Role {
        @kotlinx.serialization.SerialName("user")
        USER,

        @kotlinx.serialization.SerialName("assistant")
        ASSISTANT,
    }
}

@Serializable
data class ChatBrief(
    val name: String = "",
    val purpose: String = "",
    val entities: List<String> = emptyList(),
    val features: List<String> = emptyList(),
    val native: List<String> = emptyList(),
)

@Serializable
data class ChatPaidFeature(val id: String, val title: String? = null, val reason: String = "") {
    val displayTitle: String get() = title ?: id
}

/** proposal の案 1 つ(#184)。id / instruction が無ければ title で埋める(iOS と同じ)。 */
@Serializable
data class ChatOption(
    private val id: String? = null,
    val title: String,
    val detail: String = "",
    private val instruction: String? = null,
    val requiresPro: Boolean? = null,
    val paidFeatures: List<ChatPaidFeature>? = null,
    val freeInstruction: String? = null,
) {
    val optionId: String get() = id ?: title
    val instructionText: String get() = instruction ?: title
}

/** `POST /v1/chat` の応答(openapi `ChatResponse`)。 */
@Serializable
data class ChatTurn(
    val kind: Kind,
    val text: String = "",
    val suggestions: List<String>? = null,
    val turn: Int = 0,
    val brief: ChatBrief? = null,
    /** ready の時だけ。generate にそのまま渡す依頼文 */
    val transcript: String? = null,
    val options: List<ChatOption>? = null,
    val multiSelect: Boolean? = null,
    val locale: String? = null,
    /** claude | shortcut | forced */
    val source: String? = null,
    /** 実際に使った mode(create | edit) */
    val mode: String? = null,
    val requiresPro: Boolean? = null,
    val paidFeatures: List<ChatPaidFeature>? = null,
    val freeTranscript: String? = null,
    val confirmPublic: List<String>? = null,
) {
    /** 知らない kind は answer 扱い(吹き出しに出すだけ・落ちない) */
    @Serializable(with = ChatKindSerializer::class)
    enum class Kind(val wire: String) {
        ANSWER("answer"),
        QUESTION("question"),
        PROPOSAL("proposal"),
        READY("ready"),
    }

    val isMultiSelect: Boolean get() = multiSelect == true
    val resolvedMode: ChatMode? get() = ChatMode.entries.firstOrNull { it.name.lowercase() == mode }
}

internal object ChatKindSerializer : LenientEnumSerializer<ChatTurn.Kind>(
    "jp.kz2601.metamon.services.ChatTurn.Kind",
    ChatTurn.Kind.entries,
    { it.wire },
    ChatTurn.Kind.ANSWER,
)
