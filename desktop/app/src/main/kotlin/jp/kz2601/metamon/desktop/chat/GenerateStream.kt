package jp.kz2601.metamon.desktop.chat

import jp.kz2601.metamon.dsl.AppDSL
import jp.kz2601.metamon.dsl.DslJson
import jp.kz2601.metamon.services.GenerateProgress
import jp.kz2601.metamon.services.MorphResult
import jp.kz2601.metamon.services.StreamedComponent
import jp.kz2601.metamon.services.http.FailureKind
import jp.kz2601.metamon.services.http.SseParser
import jp.kz2601.metamon.services.live.GenerateStreamEvent
import jp.kz2601.metamon.services.live.HttpAIClient
import jp.kz2601.metamon.dsl.ComponentType
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.add
import kotlinx.serialization.json.addJsonObject
import kotlinx.serialization.json.buildJsonObject
import kotlinx.serialization.json.put
import kotlinx.serialization.json.putJsonArray
import kotlinx.serialization.json.putJsonObject
import java.util.TreeMap

/** 生成のストリームの終わり方(#2382)。 */
sealed interface GenerateEnd {
    /** `event: result` を読めた(通常の応答と同じ本文) */
    data class Completed(val result: MorphResult) : GenerateEnd

    /** `event: error`、または result が読めない */
    data class Failed(val kind: FailureKind) : GenerateEnd

    /** result も error も来ないまま行が尽きた。[last] は最後に描けた途中の DSL */
    data class Cut(val last: AppDSL?) : GenerateEnd
}

/**
 * `/v1/generate?stream=1` の SSE の行を受け、途中の DSL を流す(#2382・#2163 の 1)。画面は持たない。
 * - backend は途中の DSL そのものは送らず、`app`(名前・タブ)と `component`(最初の画面の部品)を送る。
 *   届いた分から組んだ DSL を `:dsl` の [DslJson] で読み、読めた時だけ [preview] を次の値にする(読めなければ前の値のまま)
 * - 壊れた行・知らない event は読み飛ばす(前の値のまま)。`restart` は組み立て中の物を捨てるが、描いている値は次に読めるまで残す
 * - 終わりは [GenerateEnd] の 3 つ(完成・失敗・途中で切れた)。終わった後の行は見ない
 */
class GenerateStream(private val appId: String) {
    private val parser = SseParser()
    private val _preview = MutableStateFlow<AppDSL?>(null)

    /** 今描ける途中の DSL(まだ何も読めていなければ null)。 */
    val preview: StateFlow<AppDSL?> = _preview.asStateFlow()

    var end: GenerateEnd? = null
        private set

    private var name: String? = null
    private var tabs: List<String> = emptyList()

    /** 届いた部品(画面 → 位置 → 部品)。画面は届いた順 */
    private val components = LinkedHashMap<String, TreeMap<Int, StreamedComponent>>()

    /** 1 行(改行を除いたもの)を流し込む。終わりに着いたらそれを返す。 */
    fun feed(line: String): GenerateEnd? {
        if (end != null) return end
        val frame = parser.feedLine(line) ?: return null
        when (val event = GenerateStreamEvent.decode(frame)) {
            is GenerateStreamEvent.Progress -> apply(event.progress)
            is GenerateStreamEvent.Result -> end = runCatching { HttpAIClient.parse(event.json, appId) }
                .fold({ GenerateEnd.Completed(it) }, { GenerateEnd.Failed(FailureKind.classify(it)) })
            is GenerateStreamEvent.Failure -> end = GenerateEnd.Failed(FailureKind.classify(event.error))
            null -> Unit
        }
        return end
    }

    /** 行が尽きた。終わりの印が無ければ「途中で切れた」。 */
    fun finish(): GenerateEnd = end ?: GenerateEnd.Cut(_preview.value).also { end = it }

    /** 全部の行を読んで終わり方を返す。 */
    fun readAll(lines: Sequence<String>): GenerateEnd {
        for (line in lines) if (feed(line) != null) break
        return finish()
    }

    /** 途中経過 1 つを足す(AIClient.morphStreaming の progress からも呼べる)。 */
    fun apply(progress: GenerateProgress) {
        if (end != null) return
        when (progress) {
            is GenerateProgress.Restart -> {
                name = null
                tabs = emptyList()
                components.clear()
                return
            }
            is GenerateProgress.App -> {
                name = progress.name
                tabs = progress.tabs
            }
            is GenerateProgress.Component -> {
                val c = progress.component
                components.getOrPut(c.screen) { TreeMap() }[c.index] = c
            }
            is GenerateProgress.Summary -> return
        }
        previewDsl(name, tabs, components.mapValues { it.value.values.toList() })?.let { _preview.value = it }
    }

    companion object {
        /** 最初の画面の id(component の screen が空の時) */
        internal const val FIRST_SCREEN = "home"

        /**
         * 届いた分から DSL を組み、`:dsl` で読む。読めない・知らない部品がある・描ける物が無ければ null(前の値のまま)。
         * タブは見出しだけ届くので、最初のタブを部品が届いた画面に、残りを空の画面に当てる。
         */
        fun previewDsl(name: String?, tabs: List<String>, components: Map<String, List<StreamedComponent>>): AppDSL? {
            if (name.isNullOrBlank() && components.values.all { it.isEmpty() }) return null
            val screens = linkedMapOf<String, List<StreamedComponent>>()
            components.forEach { (screen, list) -> screens[screen.ifEmpty { FIRST_SCREEN }] = list }
            if (screens.isEmpty()) screens[FIRST_SCREEN] = emptyList()
            val first = screens.keys.first()
            val tabScreens = tabs.mapIndexed { i, _ -> if (i == 0) first else "${FIRST_SCREEN}_tab$i" }
            tabScreens.drop(1).forEach { screens.putIfAbsent(it, emptyList()) }
            val json = buildJsonObject {
                put("schemaVersion", 1)
                putJsonObject("app") {
                    name?.takeIf { it.isNotBlank() }?.let { put("name", it) }
                    put("voiceEntry", "none")
                    if (tabs.size > 1) {
                        putJsonArray("tabs") {
                            tabs.forEachIndexed { i, t -> addJsonObject { put("screen", tabScreens[i]); put("title", t) } }
                        }
                    }
                }
                put("initialScreen", first)
                putJsonArray("screens") {
                    screens.forEach { (id, list) ->
                        addJsonObject {
                            put("id", id)
                            put("title", tabs.getOrNull(tabScreens.indexOf(id)).orEmpty())
                            putJsonArray("components") { list.forEach { add(componentJson(it)) } }
                        }
                    }
                }
            }
            val sent = screens.values.sumOf { it.size }
            // 部品が 1 つでも読めない(知らない type・落ちた)なら、この値は描かない
            return runCatching { DslJson.decodeFromJsonElement(AppDSL.serializer(), json) }.getOrNull()
                ?.takeIf { dsl ->
                    val read = dsl.screens.flatMap { it.components }
                    dsl.screens.any { it.id == dsl.initialScreen } && read.size == sent && read.none { it.type == ComponentType.UNKNOWN }
                }
        }

        /** 部品の見出し(label | title | text | prompt の最初の値)は、どの部品でも読まれるよう 3 つのキーに入れる。 */
        private fun componentJson(c: StreamedComponent): JsonObject = buildJsonObject {
            put("type", c.type)
            c.id?.let { put("id", it) }
            c.area?.let { put("area", it) }
            c.label?.let {
                put("label", it)
                put("title", it)
                put("text", it)
            }
        }
    }
}
