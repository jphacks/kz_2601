package jp.kz2601.metamon.desktop.chat

import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.setValue
import java.util.UUID

/**
 * 会話の履歴 1 件(iOS の `ChatHistoryEntry`・Android の `voice/ChatHistory.kt` と同じ形)。
 * USER / ASSISTANT = 吹き出し、APPLIED = 反映の区切り([text] は反映の要約)。
 */
data class ChatHistoryEntry(
    val kind: Kind,
    val text: String,
    /** APPLIED の時だけ。反映後の version */
    val version: Int? = null,
    val id: String = UUID.randomUUID().toString(),
) {
    enum class Kind { USER, ASSISTANT, APPLIED }
}

/** 履歴の保存先(端末の保存は #3258 の `ChatHistoryStore` が実装する)。 */
interface ChatHistoryPort {
    fun load(appId: String): List<ChatHistoryEntry>

    fun save(entries: List<ChatHistoryEntry>, appId: String)

    fun remove(appId: String)

    /** メモリだけ(テスト・保存先がまだ無い時)。 */
    class InMemory : ChatHistoryPort {
        private val map = HashMap<String, List<ChatHistoryEntry>>()

        override fun load(appId: String): List<ChatHistoryEntry> = map[appId].orEmpty()

        override fun save(entries: List<ChatHistoryEntry>, appId: String) {
            if (entries.isEmpty()) map.remove(appId) else map[appId] = ChatHistoryState.trimmed(entries)
        }

        override fun remove(appId: String) {
            map.remove(appId)
        }
    }
}

/**
 * 会話の画面の履歴(iOS の `AppRuntime+ChatHistory.swift`)。
 * - [history] = 閉じた会話と反映の区切り(薄く出す)。[live] = 今の会話(その後ろに続ける)
 * - 端末には `history + live` を残す(吹き出しが増えるたび)。読み直すと全部が閉じた会話になる
 * - 「最初から」は今の会話だけ消す。これまでの会話があれば、それも消すかを確認する
 */
class ChatHistoryState(private val port: ChatHistoryPort, appId: String) {
    var appId: String = appId
        private set

    var history: List<ChatHistoryEntry> by mutableStateOf(emptyList())
        private set

    var live: List<ChatHistoryEntry> by mutableStateOf(emptyList())
        private set

    /** 「最初から」の確認を出している。 */
    var restartConfirmOpen: Boolean by mutableStateOf(false)
        private set

    val hasHistory: Boolean get() = history.isNotEmpty()

    /** 会話の画面を開いた・アプリを替えた: 端末から読み直す。前の会話は全部閉じた会話になる。 */
    fun reload(appId: String = this.appId) {
        this.appId = appId
        history = trimmed(port.load(appId))
        live = emptyList()
        restartConfirmOpen = false
    }

    /** 今の会話に吹き出しを 1 つ足して残す。 */
    fun append(kind: ChatHistoryEntry.Kind, text: String) {
        require(kind != ChatHistoryEntry.Kind.APPLIED) { "applied は noteApplied で入れる" }
        live = live + ChatHistoryEntry(kind, text)
        persist()
    }

    /** 今の会話を閉じた会話に移す。 */
    fun archiveLive() {
        if (live.isEmpty()) return
        history = trimmed(history + live)
        live = emptyList()
        persist()
    }

    /** 反映した所に区切りを入れる(作り直しの成功時)。会話を使っていないアプリ(履歴が空)には入れない。 */
    fun noteApplied(version: Int?, summary: String?) {
        archiveLive()
        if (!hasHistory) return
        history = trimmed(history + ChatHistoryEntry(ChatHistoryEntry.Kind.APPLIED, summary.orEmpty(), version))
        persist()
    }

    /** 「最初から」を押した。これまでの会話があれば確認を出し、無ければ今の会話だけ消す。 */
    fun requestRestart() {
        if (hasHistory) restartConfirmOpen = true else restartLive()
    }

    /** 今の会話だけ消す。これまでの会話と区切りは残す。 */
    fun restartLive() {
        restartConfirmOpen = false
        live = emptyList()
        persist()
    }

    /** 確認で「これまでの会話も消す」: 今の会話もこのアプリの履歴も消す(戻せない)。 */
    fun restartAll() {
        restartConfirmOpen = false
        live = emptyList()
        history = emptyList()
        port.remove(appId)
    }

    fun dismissRestart() {
        restartConfirmOpen = false
    }

    private fun persist() {
        val all = history + live
        if (all.isEmpty()) port.remove(appId) else port.save(all, appId)
    }

    companion object {
        /** iOS・Android と同じ上限(区切りも 1 件と数える)。 */
        const val LIMIT = 200

        fun trimmed(entries: List<ChatHistoryEntry>): List<ChatHistoryEntry> =
            if (entries.size > LIMIT) entries.takeLast(LIMIT) else entries
    }
}
