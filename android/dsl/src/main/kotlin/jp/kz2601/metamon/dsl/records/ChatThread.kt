package jp.kz2601.metamon.dsl.records

import jp.kz2601.metamon.dsl.expr.DSLClock
import java.time.Instant
import java.time.OffsetDateTime

/** `chatThread`(#260・#875)の並び・自分の判定・送るデータ。iOS の Records/ChatThread.swift。描画は :renderer の `ChatThreadLeaf`。 */
object ChatThread {
    /** 古い順(最新が末尾)。時刻の無い旧データは先頭、同時刻は id で安定させる。未送信の自分の分はローカルの時刻なので末尾に来る。 */
    fun ordered(records: List<Record>): List<Record> =
        records.map { it to date(it.createdAt) }
            .sortedWith { a, b ->
                val x = a.second
                val y = b.second
                when {
                    x != null && y != null && x != y -> x.compareTo(y)
                    x == null && y != null -> -1
                    x != null && y == null -> 1
                    else -> a.first.id.compareTo(b.first.id)
                }
            }
            .map { it.first }

    /** 自分のメッセージか(右に出す)。private の entity・author の無い行(未送信の自分の分・旧データ)は自分。uid が分からなければ他人の分は相手側。 */
    fun isMine(record: Record, entity: Entity?, uid: String?): Boolean {
        if (entity?.isShared != true) return true
        val author = record.author ?: return true
        return author == uid
    }

    /** 相手の吹き出しの上に送信者名を出すか。同じ人が続けて送った 2 通目以降は省く。 */
    fun showsSender(record: Record, previous: Record?, entity: Entity?, uid: String?): Boolean {
        if (entity?.isShared != true || isMine(record, entity, uid)) return false
        previous ?: return true
        return isMine(previous, entity, uid) || previous.author != record.author
    }

    /** 送るレコードの data。本文の前後の空白は落とし、本文も写真も無ければ null。[base] は他の field の既定値。 */
    fun message(
        text: String,
        photo: String?,
        textField: String,
        photoField: String?,
        base: Map<String, JSONValue> = emptyMap(),
    ): Map<String, JSONValue>? {
        val body = text.trim()
        val pic = if (photoField == null) null else photo?.takeIf { it.isNotEmpty() }
        if (body.isEmpty() && pic == null) return null
        val data = base.toMutableMap()
        if (body.isNotEmpty()) data[textField] = JSONValue.Str(body)
        if (pic != null && photoField != null) data[photoField] = JSONValue.Str(pic)
        return data
    }

    /** 送る 1 件に入れる他の field の既定値(defaultValue を持つ保存 field だけ)。 */
    fun defaultData(entity: Entity, skipping: Collection<String>, now: Instant = DSLClock.now()): Map<String, JSONValue> =
        entity.fields
            .filter { !it.isComputed && it.defaultValue != null && it.id !in skipping }
            .associate { it.id to FieldCoding.defaultValue(it, now) }

    /** RFC 3339(小数秒あり / なし)→ Instant。 */
    fun date(s: String?): Instant? = s?.let { runCatching { OffsetDateTime.parse(it).toInstant() }.getOrNull() }
}
