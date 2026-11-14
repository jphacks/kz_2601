package jp.kz2601.metamon.desktop.marks

import jp.kz2601.metamon.dsl.records.Entity
import jp.kz2601.metamon.dsl.records.Mark
import jp.kz2601.metamon.dsl.records.MarkState
import jp.kz2601.metamon.dsl.records.Record
import jp.kz2601.metamon.services.AppConfigClient

/** 1 件の記録の印 1 つ(宣言 + 件数・自分が押したか)。 */
data class RecordMark(val mark: Mark, val state: MarkState)

/**
 * 共有のアプリの印(いいね・保存・#2810・docs/backend/features/shared-data.md)。描画はしない。
 * 件数と自分が押したかは記録の `marks`(サーバーの `markCounts` から作った物)を読む。
 */
object RecordMarks {
    /** 出す印。共有(shared / public)の entity が宣言している物だけ、宣言の順。 */
    fun read(entity: Entity?, record: Record): List<RecordMark> {
        val e = entity?.takeIf { it.isShared } ?: return emptyList()
        return e.marks.map { RecordMark(it, record.markState(it.id)) }
    }

    /** 1 つだけ。宣言されていなければ null。 */
    fun read(entity: Entity?, record: Record, markId: String): RecordMark? =
        entity?.takeIf { it.isShared }?.mark(markId)?.let { RecordMark(it, record.markState(markId)) }
}

/**
 * 押した時の増減(Android の `ListMarks`・iOS の `ListLeaf.toggleMark` と同じ楽観的 UI)。
 * - 1 人 1 回: 押していなければ付け(`PUT`)、押していれば取り消す(`DELETE`)
 * - 押した瞬間に件数を 1 増やす / 減らす(0 未満にしない)。返事が来たら上書きを外し、成功なら [onConfirmed] で記録に書く
 * - 失敗したら元に戻す(連打で別の値になっていたら、後の押下を残す)
 *
 * @param send `PUT` / `DELETE .../records/{id}/marks/{markId}`。返り値はサーバーの件数
 */
class MarkToggle(
    private val send: suspend (recordId: String, markId: String, on: Boolean) -> MarkState,
    private val onConfirmed: (recordId: String, markId: String, state: MarkState) -> Unit = { _, _, _ -> },
) {
    private val overrides = HashMap<Pair<String, String>, MarkState>()

    /** 表示する状態(押した直後の上書き > 記録の値)。 */
    @Synchronized
    fun shown(record: Record, markId: String): MarkState = overrides[record.id to markId] ?: record.markState(markId)

    /** 押した。成功ならサーバーの状態、失敗なら例外(上書きは戻してある)。 */
    suspend fun toggle(record: Record, markId: String): MarkState {
        val key = record.id to markId
        val next = synchronized(this) { shown(record, markId).toggled().also { overrides[key] = it } }
        val confirmed = try {
            send(record.id, markId, next.mine)
        } catch (e: Throwable) {
            // 失敗・取り消し: 元の値に戻す
            settle(key, next)
            throw e
        }
        if (settle(key, next)) onConfirmed(record.id, markId, confirmed)
        return confirmed
    }

    /** 上書きがこの時の値のままなら外す。外したら true(後から押した分が残っていれば false)。 */
    @Synchronized
    private fun settle(key: Pair<String, String>, sent: MarkState): Boolean {
        if (overrides[key] != sent) return false
        overrides.remove(key)
        return true
    }

    companion object {
        /** appconfig の `setMark` で送る。 */
        fun sender(client: AppConfigClient, appId: String, entity: String): suspend (String, String, Boolean) -> MarkState =
            { recordId, markId, on -> client.setMark(appId, entity, recordId, markId, on).let { MarkState(it.count, it.mine) } }
    }
}
