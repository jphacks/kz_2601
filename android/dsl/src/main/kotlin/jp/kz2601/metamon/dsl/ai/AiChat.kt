package jp.kz2601.metamon.dsl.ai

import jp.kz2601.metamon.dsl.records.FieldCoding
import jp.kz2601.metamon.dsl.records.JSONValue
import jp.kz2601.metamon.dsl.records.Record
import java.time.Instant

/**
 * `aiChat` の会話の並び・役・送る依頼・保存するデータ(iOS の MetamonDSL `Records/AiChat.swift`)。描画は :renderer の `AiChatLeaf`。
 * 1 発言 = entity の 1 レコード。役は roleField(user / assistant)。
 * roleField の無い古い DSL(stg で作られたもの)だけ古い順に自分 → AI の交互で読む(その時は対で保存して交互を崩さない)。
 */
object AiChat {
    enum class Role(val wire: String) { USER("user"), ASSISTANT("assistant") }

    data class Turn(val id: String, val role: Role, val text: String)

    /** backend `dsl.MaxAIChatTurns` / `dsl.MaxAIChatBytes`。 */
    const val MAX_TURNS = 20
    const val MAX_BYTES = 16 * 1024

    /** 会話・参考の記録に使うのは自分の記録だけ。 */
    fun own(records: List<Record>, uid: String?, shared: Boolean): List<Record> = records.filter { isOwn(it, uid, shared) }

    /**
     * 自分の記録か。author(空文字は無いのと同じ)がサインイン中の uid と同じなら自分。
     * author が無い時: private の entity は全部自分。shared / public は未送信の記録だけ自分。
     */
    fun isOwn(record: Record, uid: String?, shared: Boolean): Boolean {
        val author = record.author
        if (!author.isNullOrEmpty()) return !uid.isNullOrEmpty() && author == uid
        return !shared || record.unsent
    }

    /** 作成時刻の古い順(時刻の無い = 作ったばかりの記録は後ろ・同じ時刻は id 順)。iOS の `ChatThread.ordered`。 */
    fun ordered(records: List<Record>): List<Record> =
        records.map { it to date(it.createdAt) }.sortedWith { a, b ->
            val x = a.second
            val y = b.second
            when {
                x != null && y != null && x != y -> x.compareTo(y)
                x == null && y != null -> 1
                x != null && y == null -> -1
                else -> a.first.id.compareTo(b.first.id)
            }
        }.map { it.first }

    fun date(s: String?): Instant? {
        if (s.isNullOrEmpty()) return null
        return runCatching { Instant.parse(s) }.getOrNull() ?: FieldCoding.strictDatetime(s)
    }

    /** 古い順の発言。本文の無いレコードは飛ばす(交互の数え方にも入れない)。 */
    fun turns(records: List<Record>, textField: String, roleField: String?): List<Turn> {
        val out = mutableListOf<Turn>()
        for (r in ordered(records)) {
            val text = (r[textField].stringValue ?: "").trim()
            if (text.isEmpty()) continue
            val role = if (roleField != null) {
                if (r[roleField].stringValue == Role.ASSISTANT.wire) Role.ASSISTANT else Role.USER
            } else {
                if (out.size % 2 == 0) Role.USER else Role.ASSISTANT
            }
            out += Turn(r.id, role, text)
        }
        return out
    }

    /** `speak: true` で読む返事: 保存した直後の並びで、本文が [reply] と同じ一番新しい AI の発言。 */
    fun spokenReply(turns: List<Turn>, reply: String): Turn? {
        val text = reply.trim()
        if (text.isEmpty()) return null
        return turns.lastOrNull { it.role == Role.ASSISTANT && it.text == text }
    }

    /**
     * `POST /v1/text` の task chat。直近 20 発言・本文 16 KB に収まる分だけ(古い方から落とす)。最後は利用者の発言([next])。
     * [material] を付ける時は会話を残りの `16 KB - material` に収める。
     */
    fun request(
        persona: String,
        history: List<Turn>,
        next: String,
        long: Boolean,
        locale: String,
        saveTo: AiTextSaveTo? = null,
        material: AiTextMaterial? = null,
    ): AiTextRequest {
        val maxBytes = MAX_BYTES - (material?.text?.utf8Size() ?: 0)
        val last = AiTextLimits.clip(next.trim(), maxBytes)
        val picked = ArrayDeque<AiTextMessage>()
        picked.addFirst(AiTextMessage(Role.USER.wire, last))
        var used = last.utf8Size()
        for (t in history.asReversed()) {
            val n = t.text.utf8Size()
            if (picked.size >= MAX_TURNS || used + n > maxBytes) break
            picked.addFirst(AiTextMessage(t.role.wire, t.text))
            used += n
        }
        while (picked.firstOrNull()?.role == Role.ASSISTANT.wire) picked.removeFirst() // 先頭は利用者から
        val instruction = persona.trim().take(AiTextLimits.MAX_INSTRUCTION)
        return AiTextRequest(
            instruction = instruction,
            shape = AiTextShape.LINE,
            locale = locale,
            task = "chat",
            length = if (long) "long" else null,
            messages = picked.toList(),
            saveTo = saveTo,
            material = material,
        )
    }

    /** 保存する 1 件の data。本文が空なら null。[base] は entity の他の field の既定値。 */
    fun record(
        text: String,
        role: Role,
        textField: String,
        roleField: String?,
        base: Map<String, JSONValue> = emptyMap(),
    ): Map<String, JSONValue>? {
        val body = text.trim()
        if (body.isEmpty()) return null
        val data = base.toMutableMap()
        data[textField] = JSONValue.Str(body)
        if (roleField != null) data[roleField] = JSONValue.Str(role.wire)
        return data
    }
}
