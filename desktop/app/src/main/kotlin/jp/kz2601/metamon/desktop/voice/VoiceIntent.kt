package jp.kz2601.metamon.desktop.voice

/**
 * AI に送る前に端末で拾う発話の意図(#3005・Android の `VoiceIntent` と同じ規則)。会話の画面が送る前に見る。
 * - [Undo]: 「元に戻して」「取り消して」「undo」→ 版の履歴で戻す(AI を呼ばない)
 * - [NewApp]: 発話頭の「別のアプリ」「新しいアプリ」「new app」→ appId を作り直してから AI へ
 * - [SwitchApp]: 「家計簿に切り替えて」「switch to budget」→ アプリ一覧の名前と照合して切り替え
 * - [ListApps]: 「アプリ一覧」「別のアプリに切り替えて」「list apps」→ 変身一覧を開く
 * - [Morph]: それ以外は従来どおり AI へ
 */
sealed interface VoiceIntent {
    data object Undo : VoiceIntent
    data object NewApp : VoiceIntent
    data class SwitchApp(val name: String) : VoiceIntent
    data object ListApps : VoiceIntent
    data object Morph : VoiceIntent

    companion object {
        internal val undoPrefixes = listOf(
            "元に戻", "もとに戻", "取り消", "とりけし", "アンドゥ", "あんどぅ", "undo", "ひとつ前", "一つ前", "さっきの状態", "前の状態",
        )

        /** 「戻して」単独のような短い発話だけ undo(「ボタンを右に戻して」は拾わない) */
        internal val shortUndoWords = listOf("戻して", "戻す", "もどして", "もどす")
        internal const val SHORT_UNDO_MAX_LENGTH = 8
        internal val newAppPrefixes = listOf(
            "別のアプリ", "別アプリ", "新しいアプリ", "新規アプリ", "違うアプリ", "次のアプリ", "べつのアプリ", "あたらしいアプリ",
            "newapp", "anewapp", "anotherapp", "adifferentapp",
        )

        internal val listAppsWords: List<String> = run {
            val subjects = listOf("アプリ", "変身", "へんしん")
            val lists = listOf("一覧", "いちらん", "リスト", "list")
            subjects.flatMap { s -> lists.flatMap { l -> listOf(s + l, s + "の" + l) } } +
                listOf("applist", "listapps", "listmyapps", "listofapps", "showmyapps", "showallapps", "openmyapps")
        }

        /** 「<名前>に切り替えて」など。名前 = 最初に現れた切替語より前 */
        internal val switchWords = listOf(
            "に切り替え", "に切替", "にきりかえ", "を切り替え", "を切替", "をきりかえ", "へ切り替え", "へ切替",
            "を開い", "をひらい", "を開け", "をひらけ", "に変身", "にへんしん",
        )

        /** 英語は文頭の切替語の後ろが名前(「switch to budget」) */
        internal val englishSwitchPrefixes = listOf("switchto", "switchover", "changeappto", "opentheapp", "openapp")
        internal val nameSuffixes = listOf("のアプリ", "アプリ", "の方", "のほう", "の", "app")
        internal val genericNames = setOf(
            "", "アプリ", "別", "別の", "べつの", "他", "他の", "ほか", "ほかの", "違う", "ちがう", "次", "次の", "つぎの", "前", "前の", "まえの",
            "どれか", "何か", "なにか",
            "another", "other", "theother", "different", "adifferent", "next", "thenext", "previous", "app", "apps", "myapps",
        )
        internal val freshNames = setOf("新しい", "あたらしい", "新規", "新しく", "new", "anew", "afresh", "fresh", "anewone")
        private val leadWords = listOf("じゃあ", "じゃ", "では", "次は", "つぎは", "今度は", "こんどは", "やっぱり", "やっぱ")

        fun detect(transcript: String): VoiceIntent {
            val text = normalize(transcript)
            if (text.isEmpty()) return Morph
            if (undoPrefixes.any { text.contains(it) }) return Undo
            if (text.length <= SHORT_UNDO_MAX_LENGTH && shortUndoWords.any { text.contains(it) }) return Undo
            if (listAppsWords.any { text.contains(it) }) return ListApps
            switchTarget(text)?.let { raw ->
                val name = cleanAppName(raw)
                return when (name) {
                    in freshNames -> NewApp
                    in genericNames -> ListApps
                    else -> SwitchApp(name)
                }
            }
            if (newAppPrefixes.any { text.startsWith(it) }) return NewApp
            return Morph
        }

        /** 切替語のうち最も手前のものより前(英語は文頭の切替語より後ろ)。切替語が無ければ null */
        internal fun switchTarget(text: String): String? {
            englishSwitchPrefixes.firstOrNull { text.startsWith(it) }?.let { return text.removePrefix(it) }
            val best = switchWords.mapNotNull { w -> text.indexOf(w).takeIf { it >= 0 } }.minOrNull() ?: return null
            return text.substring(0, best)
        }

        /** 「家計簿アプリ」「家計簿の方」「the budget app」→ 名前だけ。先頭の接続語も落とす */
        internal fun cleanAppName(raw: String): String {
            var s = raw
            for (lead in leadWords) if (s.startsWith(lead) && s.length > lead.length) s = s.substring(lead.length)
            var changed = true
            while (changed) {
                changed = false
                for (suf in nameSuffixes) {
                    if (s.endsWith(suf) && s.length > suf.length) {
                        s = s.dropLast(suf.length)
                        changed = true
                    }
                }
            }
            return s
        }

        /** 空白・句読点・記号を落として小文字に(「元に 戻して!」の揺れを吸収) */
        internal fun normalize(s: String): String {
            val out = StringBuilder()
            s.lowercase().codePoints().forEach { cp ->
                if (!Character.isWhitespace(cp) && !Character.isSpaceChar(cp) && Character.getType(cp) !in dropTypes) out.appendCodePoint(cp)
            }
            return out.toString()
        }

        private val dropTypes: Set<Int> = setOf(
            Character.CONNECTOR_PUNCTUATION, Character.DASH_PUNCTUATION, Character.START_PUNCTUATION, Character.END_PUNCTUATION,
            Character.INITIAL_QUOTE_PUNCTUATION, Character.FINAL_QUOTE_PUNCTUATION, Character.OTHER_PUNCTUATION,
            Character.MATH_SYMBOL, Character.CURRENCY_SYMBOL, Character.MODIFIER_SYMBOL, Character.OTHER_SYMBOL,
            Character.CONTROL,
        ).map { it.toInt() }.toSet()
    }
}

/** 発話のアプリ名を一覧の名前と照合する(完全一致 → 前方一致 → 部分一致。同じ段で複数なら曖昧・iOS の `AppNameMatch`)。 */
sealed interface AppNameMatch {
    data object None : AppNameMatch
    data class One(val index: Int) : AppNameMatch
    data class Ambiguous(val indices: List<Int>) : AppNameMatch

    companion object {
        fun match(query: String, names: List<String>): AppNameMatch {
            val q = key(query)
            if (q.isEmpty()) return None
            val keys = names.map(::key)
            val tiers: List<(String) -> Boolean> = listOf(
                { it == q },
                { it.startsWith(q) || q.startsWith(it) },
                { it.contains(q) || q.contains(it) },
            )
            for (tier in tiers) {
                val hits = keys.indices.filter { keys[it].isNotEmpty() && tier(keys[it]) }
                if (hits.size == 1) return One(hits[0])
                if (hits.size > 1) return Ambiguous(hits)
            }
            return None
        }

        /** 正規化 + 末尾の「アプリ」を落とす */
        internal fun key(s: String): String = VoiceIntent.cleanAppName(VoiceIntent.normalize(s))
    }
}
