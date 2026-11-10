package jp.kz2601.metamon.services.mock

import jp.kz2601.metamon.services.AIClient
import jp.kz2601.metamon.services.ChatBrief
import jp.kz2601.metamon.services.ChatMessage
import jp.kz2601.metamon.services.ChatMode
import jp.kz2601.metamon.services.ChatOption
import jp.kz2601.metamon.services.ChatPaidFeature
import jp.kz2601.metamon.services.ChatTurn
import jp.kz2601.metamon.services.GenerateMode
import jp.kz2601.metamon.services.GenerateProgress
import jp.kz2601.metamon.services.MorphResult
import jp.kz2601.metamon.services.StreamedComponent
import kotlinx.coroutines.delay
import kotlinx.serialization.json.JsonArray
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive
import kotlinx.serialization.json.contentOrNull

/**
 * 発話のキーワードで同梱サンプル DSL を返す ai のモック(iOS の `MockAIClient`)。
 * 生成の途中経過(SSE)は、サンプルのアプリ名・タブと最初の画面の部品を [stepDelayMillis] おきに流して擬似する。
 */
class MockAIClient(
    private val samples: DslSamples,
    private val delayMillis: Long = 1_200,
    private val stepDelayMillis: Long = 150,
) : AIClient {
    override suspend fun morph(
        transcript: String,
        appId: String,
        currentVersion: Int,
        locale: String?,
        mode: GenerateMode,
    ): MorphResult {
        delay(delayMillis)
        return result(transcript, appId, currentVersion)
    }

    override suspend fun morphStreaming(
        transcript: String,
        appId: String,
        currentVersion: Int,
        locale: String?,
        mode: GenerateMode,
        progress: (GenerateProgress) -> Unit,
    ): MorphResult {
        val result = result(transcript, appId, currentVersion)
        val dsl = samples.json(resolvedName(transcript)) ?: JsonObject(emptyMap())
        val tabs = ((dsl["app"] as? JsonObject)?.get("tabs") as? JsonArray).orEmpty()
            .mapNotNull { ((it as? JsonObject)?.get("title") as? JsonPrimitive)?.contentOrNull }
        delay(stepDelayMillis)
        progress(GenerateProgress.App(result.document.dsl.app.name, tabs, attempt = 0))
        for (c in firstScreenComponents(dsl)) {
            delay(stepDelayMillis)
            progress(GenerateProgress.Component(c))
        }
        delay((delayMillis - stepDelayMillis).coerceAtLeast(0))
        return result
    }

    private fun resolvedName(transcript: String): String =
        if (wantsSharedBoard(transcript) && samples.has("post_comments")) "post_comments" else sampleName(transcript, samples)

    private fun result(transcript: String, appId: String, currentVersion: Int): MorphResult {
        val name = resolvedName(transcript)
        val doc = samples.document(name, appId, currentVersion + 1)
        // 改変らしい指示(作った後 / 「アプリ」も句点も無い短い指示)は指示をそのまま要約に返す(iOS #186 と同じ推定)
        val looksLikeEdit = currentVersion > 0 || (!transcript.contains("アプリ") && !transcript.contains("。"))
        val summary = if (looksLikeEdit && name != DslSamples.INITIAL) {
            "「$transcript」を反映しました(モック)"
        } else {
            "「${doc.document.dsl.app.name}」を用意しました(モック)"
        }
        return MorphResult(document = doc.document, summary = summary, mode = "full", envelopeJson = doc.envelopeJson)
    }

    /**
     * 相談のモック(状態は持たず、毎回送られる全履歴だけで決める・iOS と同じ分岐)。
     * 問いかけ → answer / 「見やすく」「おすすめ」→ proposal 2 案 / 「足したい」→ multiSelect の proposal 3 案 /
     * create の 1 発目に「項目」→ multiSelect の question / edit はそれ以外 ready / create の短い 1 発目は 1 問だけ聞く。
     */
    override suspend fun chat(appId: String, messages: List<ChatMessage>, locale: String?, mode: ChatMode?): ChatTurn {
        delay(delayMillis / 2)
        val users = messages.filter { it.role == ChatMessage.Role.USER }.map { it.text }
        val turn = users.size
        val last = users.lastOrNull().orEmpty()
        val goAhead = isGoAhead(last)
        val requests = users.filter { !isGoAhead(it) && !isQuestion(it) }
        val m = mode ?: ChatMode.CREATE
        val wire = m.name.lowercase()
        if (last.contains("足したい")) {
            return ChatTurn(
                ChatTurn.Kind.PROPOSAL, "組み合わせて選べます。", turn = turn, options = multiProposals,
                multiSelect = true, locale = "ja", source = "mock", mode = wire,
            )
        }
        if (m == ChatMode.CREATE && turn == 1 && last.contains("項目")) {
            return ChatTurn(
                ChatTurn.Kind.QUESTION, "どの項目を一覧に出しますか?", suggestions = listOf("金額", "日付", "メモ"),
                turn = turn, multiSelect = true, locale = "ja", source = "mock", mode = wire,
            )
        }
        if (isQuestion(last)) {
            return ChatTurn(
                ChatTurn.Kind.ANSWER, answer(last),
                suggestions = if (m == ChatMode.EDIT) listOf("金額を大きくして", "見やすくして") else null,
                turn = turn, locale = "ja", source = "mock", mode = wire,
            )
        }
        if (listOf("見やすく", "おすすめ", "オススメ", "案を").any(last::contains)) {
            return ChatTurn(
                ChatTurn.Kind.PROPOSAL, "こんな案はどうでしょう。よければ「これでいく」を押してください。", turn = turn,
                options = proposals(m), locale = "ja", source = "mock", mode = wire,
            )
        }
        if (m == ChatMode.EDIT) {
            val instruction = if (goAhead) requests.joinToString("。") else last
            return markingPaid(
                ChatTurn(
                    ChatTurn.Kind.READY, "この内容で変えますね。", turn = turn,
                    transcript = instruction.ifEmpty { last }, locale = "ja", source = if (goAhead) "shortcut" else "mock",
                ),
            )
        }
        if (turn == 1 && !goAhead && last.length <= 10) {
            return ChatTurn(
                ChatTurn.Kind.QUESTION, "写真も一緒に残したいですか?", suggestions = listOf("写真も残す", "文字だけでOK", "お任せ"),
                turn = turn, locale = "ja", source = "mock",
            )
        }
        val transcript = if (requests.isEmpty()) "家計簿アプリを作って。" else requests.joinToString("。")
        val title = titles[sampleName(transcript, samples)] ?: "記録アプリ"
        return markingPaid(
            ChatTurn(
                ChatTurn.Kind.READY, "では作りますね。", turn = turn,
                brief = ChatBrief(name = title, purpose = requests.firstOrNull() ?: title, features = requests.drop(1)),
                transcript = transcript, locale = "ja", source = if (goAhead) "shortcut" else "mock",
            ),
        )
    }

    override suspend fun resetChat(appId: String) = Unit

    companion object {
        /** 発話 → サンプル名(iOS `MockAIClient.sampleName` と同じ順で見る)。手元に無いサンプルは飛ばす。 */
        fun sampleName(transcript: String, samples: DslSamples? = null): String {
            val t = transcript
            val rules = listOf(
                "receipt_ai" to t.contains("レシート"),
                "form_check" to (t.contains("フォーム") && (t.contains("チェック") || t.contains("角度"))),
                "training" to listOf("トレ", "筋", "運動").any(t::contains),
                "walk" to listOf("散歩", "写真", "場所").any(t::contains),
                "shopping" to listOf("買い物", "チェックリスト").any(t::contains),
                "medicine" to listOf("薬", "服薬").any(t::contains),
                "timetable" to listOf("時間割", "時限").any(t::contains),
                "drive" to listOf("ナビ", "道案内", "ドライブ").any(t::contains),
                "dinner" to listOf("ランダム", "くじ", "晩ごはん").any(t::contains),
                "dashcam" to listOf("ドラレコ", "車載").any(t::contains),
                "sensors" to listOf("傾き", "水平器", "気圧").any(t::contains),
                "drive_music" to listOf("音楽", "BGM").any(t::contains),
                DslSamples.INITIAL to listOf("最初", "戻").any(t::contains),
            )
            return rules.firstOrNull { it.second && (samples?.has(it.first) ?: true) }?.first ?: "kakeibo"
        }

        /** #414: 「掲示板」は誰でも見られるひとこと投稿(無料版・自分だけの指示が付けば通常のサンプル)。 */
        fun wantsSharedBoard(t: String): Boolean = t.contains("掲示板") && !t.contains("無料版") && !t.contains("自分だけ")

        /** 最初に見える画面の部品(生成の SSE `event: component` の擬似)。 */
        internal fun firstScreenComponents(dsl: JsonObject): List<StreamedComponent> {
            fun JsonObject.s(k: String) = (this[k] as? JsonPrimitive)?.takeIf { it.isString }?.content
            val initial = dsl.s("initialScreen") ?: return emptyList()
            val screen = (dsl["screens"] as? JsonArray).orEmpty().filterIsInstance<JsonObject>()
                .firstOrNull { it.s("id") == initial } ?: return emptyList()
            return (screen["components"] as? JsonArray).orEmpty().filterIsInstance<JsonObject>().mapIndexedNotNull { i, c ->
                val type = c.s("type") ?: return@mapIndexedNotNull null
                StreamedComponent(
                    attempt = 0, screen = initial, index = i, id = c.s("id"), type = type, area = c.s("area"),
                    label = listOf("label", "title", "text", "prompt").firstNotNullOfOrNull { c.s(it) },
                )
            }
        }

        private val titles = mapOf(
            "training" to "筋トレ記録", "walk" to "散歩記録", "kakeibo" to "家計簿",
            "shopping" to "買い物リスト", "medicine" to "服薬チェック",
        )

        fun isQuestion(text: String): Boolean {
            val t = text.trim()
            if (t.endsWith("?") || t.endsWith("？")) return true
            return listOf("できる", "可能", "ですか", "WebView", "どうなって").any(t::contains)
        }

        fun isGoAhead(text: String): Boolean {
            val t = text.trim()
            if (t.length > 14) return false
            return listOf("作って", "お任せ", "おまかせ", "それでいい", "よろしく").any(t::contains)
        }

        private fun answer(q: String): String = when {
            q.contains("WebView") -> "いいえ、WebView ではありません。AI が作った画面の設計図(JSON)を、端末の部品でそのまま描いています。"
            q.contains("大き") -> "できます。「金額を大きくして」のように言ってもらえれば、その部分の文字を大きくします。"
            else -> "はい、できます。どこをどう変えたいか教えてもらえれば、案を出します。"
        }

        val multiProposals = listOf(
            ChatOption("opt1", "グラフを足す", "ホームにカテゴリ別の円グラフを置きます。", "ホームにカテゴリ別の円グラフを追加して"),
            ChatOption("opt2", "予算バーを足す", "今月の予算の進み具合をバーで見せます。", "ホームに今月の予算の進捗バーを追加して"),
            ChatOption("opt3", "並べ替えを足す", "一覧を金額の大きい順に並べ替えられます。", "一覧を金額で並べ替えられるようにして"),
        )

        private fun proposals(mode: ChatMode): List<ChatOption> = when (mode) {
            ChatMode.EDIT -> listOf(
                ChatOption("opt1", "数字を大きく", "金額や合計を大きな太字にして、ひと目で分かるようにします。", "金額と合計の数字を大きな太字にして、ひと目で分かるようにして"),
                ChatOption("opt2", "カードで区切る", "記録を 1 件ずつカードにして、余白を広げます。", "一覧の記録を 1 件ずつカードにして、行の間の余白を広げて"),
            )
            ChatMode.CREATE -> listOf(
                ChatOption("opt1", "家計簿", "金額とカテゴリを記録して、今月の合計を見る。", "家計簿アプリを作って。金額とカテゴリを記録して、今月の合計を見たい。"),
                ChatOption("opt2", "筋トレ記録", "種目と回数を記録して、今日の合計を見る。", "筋トレ記録アプリを作って。種目と回数を記録して、今日の合計を見たい。"),
            )
        }

        /** 有料機能(#77)のモック。sharedData の代表的な語だけ見る。 */
        private val paidKeywords = listOf("みんなで", "共有", "掲示板", "投稿")
        private const val FREE_SUFFIX = "(無料版で作る: 他のユーザーとデータを共有する機能は入れない)"
        val sharedDataFeature = ChatPaidFeature(
            id = "sharedData",
            title = "みんなで共有するデータ",
            reason = "みんなで書き込むデータはクラウドに保存して全員の端末に同期するため、サーバーの費用がかかります。",
        )

        private fun markingPaid(turn: ChatTurn): ChatTurn {
            val t = turn.transcript ?: return turn
            if (turn.kind != ChatTurn.Kind.READY || paidKeywords.none(t::contains)) return turn
            return turn.copy(requiresPro = true, paidFeatures = listOf(sharedDataFeature), freeTranscript = t + "\n" + FREE_SUFFIX)
        }
    }
}
