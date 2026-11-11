package jp.kz2601.metamon.dsl.ai

import jp.kz2601.metamon.dsl.records.Entity
import jp.kz2601.metamon.dsl.records.Field
import jp.kz2601.metamon.dsl.records.FieldCoding
import jp.kz2601.metamon.dsl.records.FieldType
import jp.kz2601.metamon.dsl.records.JSONValue
import jp.kz2601.metamon.dsl.records.Record
import java.text.Normalizer
import kotlin.math.ln

/**
 * `aiChat.inputEntity`: 会話に添える「参考の記録」(`POST /v1/text` の material)。iOS の `AiChatMaterial`。検証は backend `dsl/ai_chat_input.go`。
 * 送るのは自分の記録だけ([AiChat.isOwn])。最後の発言に近い記録から([AiChatRelevance])limit 件(既定・上限 20)、
 * 1 件 1 行の「名前: 値」で 8 KB まで。hidden の field・写真などの読めない型・computed は送らない。
 */
object AiChatMaterial {
    const val MAX_RECORDS = 20
    const val MAX_BYTES = 8 * 1024
    const val MAX_NAME_CODE_POINTS = 40
    val fieldTypes: Set<FieldType> = setOf(
        FieldType.STRING, FieldType.TEXT, FieldType.INTEGER, FieldType.NUMBER, FieldType.BOOLEAN,
        FieldType.DATE, FieldType.DATETIME, FieldType.ENUM, FieldType.CURRENCY,
    )

    fun fields(entity: Entity): List<Field> = entity.storedFields.filter { !it.hidden && it.type in fieldTypes }

    /** 添える記録。[query] に近い記録が先(同点は新しい順)、残りは新しい順。時刻の無い(作ったばかりの)記録は先頭。 */
    fun select(
        records: List<Record>,
        uid: String?,
        shared: Boolean,
        limit: Int?,
        query: String? = null,
        fields: List<Field> = emptyList(),
    ): List<Record> {
        val n = (limit ?: MAX_RECORDS).coerceIn(1, MAX_RECORDS)
        val newest = AiChat.own(records, uid, shared)
            .map { it to (AiChat.date(it.updatedAt) ?: AiChat.date(it.createdAt)) }
            .sortedWith { a, b ->
                val x = a.second
                val y = b.second
                when {
                    x != null && y != null && x != y -> y.compareTo(x)
                    x == null && y != null -> -1
                    x != null && y == null -> 1
                    else -> a.first.id.compareTo(b.first.id)
                }
            }
            .map { it.first }
        val q = AiChatRelevance.tokens(query ?: "")
        if (q.isEmpty() || fields.isEmpty()) return newest.take(n)
        val scores = AiChatRelevance.scores(q, newest.map { values(fields, it) })
        val order = newest.indices.sortedWith { a, b -> if (scores[a] != scores[b]) scores[b].compareTo(scores[a]) else a.compareTo(b) }
        return order.take(n).map { newest[it] }
    }

    fun values(fs: List<Field>, r: Record): String = fs.joinToString("\n") { FieldCoding.display(r.data[it.id] ?: JSONValue.Null, it) }

    /** 1 件を 1 行に保つ: 値の中の改行を「 ⏎ 」にする。 */
    fun oneLine(s: String): String = s.split(Regex("\r\n|[\n\r\u000B\u000C\u0085  ]")).joinToString(" ⏎ ")

    /** 1 件の「名前: 値」(aiButton の `AIRequestBuilder.describe` と同じ形)。 */
    fun describe(fs: List<Field>, r: Record, separator: String = " / "): String =
        fs.mapNotNull { f ->
            val s = FieldCoding.display(r.data[f.id] ?: JSONValue.Null, f)
            if (s.isEmpty()) null else "${f.name}: $s"
        }.joinToString(separator)

    /** 依頼に付ける material。送れる記録・値が無ければ null。 */
    fun request(entity: Entity, records: List<Record>, uid: String?, limit: Int?, query: String? = null): AiTextMaterial? {
        val fs = fields(entity)
        if (fs.isEmpty()) return null
        val q = AiChatRelevance.tokens(query ?: "")
        val lines = mutableListOf<String>()
        var used = 0
        for (r in select(records, uid, entity.isShared, limit, query, fs)) {
            val body = oneLine(describe(fs, r))
            if (body.isEmpty()) continue
            val line = "- $body"
            val sep = if (lines.isEmpty()) 0 else 1
            val size = line.utf8Size()
            if (used + sep + size <= MAX_BYTES) {
                lines += line
                used += sep + size
                continue
            }
            // 1 件目だけは切って入れる(発言に近い所から)。8 KB を使い切るので終わり
            if (lines.isEmpty()) {
                lines += AiChatRelevance.window(line, q, MAX_BYTES)
                break
            }
        }
        if (lines.isEmpty()) return null
        val trimmed = entity.name.trim()
        val name = (if (trimmed.codePointCount(0, trimmed.length) > MAX_NAME_CODE_POINTS) {
            trimmed.substring(0, trimmed.offsetByCodePoints(0, MAX_NAME_CODE_POINTS))
        } else {
            trimmed
        }).trim()
        return AiTextMaterial(entity = entity.id, name = name.ifEmpty { null }, text = lines.joinToString("\n"))
    }
}

/**
 * 記録選びに使う簡単なキーワード検索(iOS の `AiChatRelevance`)。NFKC + 小文字にして、かな・漢字・ハングルの並びは 2 文字ずつ
 * (1 文字だけの並びはその 1 文字)、英数字の並びは 2 文字以上の語。点は含む語の重み log(1 + 件数 / 含む件数) の和。
 */
object AiChatRelevance {
    fun tokens(s: String): Set<String> {
        val out = linkedSetOf<String>()
        val word = StringBuilder()
        var prev: Int? = null
        var run = 0
        fun flush() {
            if (word.codePointCount(0, word.length) >= 2) out += word.toString()
            word.setLength(0)
            val p = prev
            if (run == 1 && p != null) out += String(Character.toChars(p))
            prev = null
            run = 0
        }
        val text = Normalizer.normalize(s, Normalizer.Form.NFKC).lowercase()
        var i = 0
        while (i < text.length) {
            val u = text.codePointAt(i)
            i += Character.charCount(u)
            if (isCJK(u)) {
                if (word.isNotEmpty()) flush()
                prev?.let { out += String(Character.toChars(it)) + String(Character.toChars(u)) }
                prev = u
                run++
            } else if (Character.isLetter(u) || Character.isDigit(u) || Character.getNumericValue(u) >= 0) {
                if (run > 0) flush()
                word.appendCodePoint(u)
            } else {
                flush()
            }
        }
        flush()
        return out
    }

    fun isCJK(u: Int): Boolean = u == 0x3005 || u == 0x3007 ||
        u in 0x3040..0x30FF ||
        u in 0x3400..0x4DBF || u in 0x4E00..0x9FFF || u in 0xF900..0xFAFF ||
        u in 0x20000..0x323AF || u in 0x2F800..0x2FA1F ||
        u in 0xAC00..0xD7AF

    /** 各文書の点([documents] と同じ順)。0 = 発言の語を 1 つも含まない。 */
    fun scores(query: Set<String>, documents: List<String>): List<Double> {
        val terms = query.sorted()
        val sets = documents.map { tokens(it).intersect(query) }
        val df = mutableMapOf<String, Int>()
        for (s in sets) for (t in s) df[t] = (df[t] ?: 0) + 1
        val n = documents.size.toDouble()
        return sets.map { s -> terms.fold(0.0) { acc, t -> if (t in s) acc + ln(1 + n / (df[t] ?: 1)) else acc } }
    }

    /** 1 行に入りきらない記録を [maxBytes] に切る。発言の語が一番多く入る所を約 512 バイトの区切りで探す。 */
    fun window(line: String, query: Set<String>, maxBytes: Int, chunkBytes: Int = 512): String {
        val head = AiTextLimits.clip(line, maxBytes)
        if (line.utf8Size() <= maxBytes || query.isEmpty()) return head
        val chunks = mutableListOf<Pair<Int, Int>>() // [start, end) の文字位置
        var start = 0
        var used = 0
        var i = 0
        while (i < line.length) {
            val cp = line.codePointAt(i)
            val len = Character.charCount(cp)
            val n = String(Character.toChars(cp)).utf8Size()
            if (used + n > chunkBytes && i > start) {
                chunks += start to i
                start = i
                used = 0
            }
            used += n
            i += len
        }
        chunks += start to line.length
        val hits = chunks.map { (a, b) -> tokens(line.substring(a, b)).intersect(query).size }
        val bytes = chunks.map { (a, b) -> line.substring(a, b).utf8Size() }
        val marker = "- …"
        val budget = maxBytes - marker.utf8Size()
        var best = 0
        var bestScore = -1
        for (s in chunks.indices) {
            var total = 0
            var score = 0
            for (c in s until chunks.size) {
                total += bytes[c]
                if (total > budget) break
                score += hits[c]
            }
            if (score > bestScore) {
                best = s
                bestScore = score
            }
        }
        if (best == 0 || bestScore <= 0) return head
        return AiTextLimits.clip(marker + line.substring(chunks[best].first), maxBytes)
    }
}
