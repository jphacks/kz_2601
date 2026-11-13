package jp.kz2601.metamon.desktop.marks

import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableIntStateOf
import androidx.compose.runtime.mutableStateMapOf
import androidx.compose.runtime.setValue
import jp.kz2601.metamon.dsl.ListItem
import jp.kz2601.metamon.dsl.records.Entity
import jp.kz2601.metamon.dsl.records.Mark
import jp.kz2601.metamon.dsl.records.MarkState
import jp.kz2601.metamon.dsl.records.Record
import jp.kz2601.metamon.services.AppConfigClient
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.withContext

/**
 * 一覧の行の印のボタン(#2811)。押した時の規則は [MarkToggle]、ここは画面に出す状態を Compose の状態で持つ。
 * - 返事で確定した件数は記録に書けない(デスクトップの記録は印を持たない)ので、ここに残して記録の値より先に出す
 * - サインインしていない(送り先が無い)時は作らない = ボタンを出さない
 *
 * @param send `PUT` / `DELETE .../records/{id}/marks/{markId}`。(entity, 画面の recordId, markId, 付けるか)
 */
class RowMarks(private val send: suspend (entity: String, recordId: String, markId: String, on: Boolean) -> MarkState) {
    private val confirmed = mutableStateMapOf<Key, MarkState>()
    private val toggles = HashMap<String, MarkToggle>()

    /** 押した直後の上書きは [MarkToggle] の中にあるので、変わったことを画面に知らせる。 */
    private var version by mutableIntStateOf(0)

    private data class Key(val entity: String, val recordId: String, val markId: String)

    /** 表示する状態(押した直後の上書き > 返事で確定した値 > 記録の値)。 */
    fun shown(entity: String, record: Record, markId: String): MarkState {
        version
        return toggle(entity).shown(withConfirmed(entity, record, markId), markId)
    }

    /** 押した。成功なら true、失敗なら false(表示は元に戻っている)。 */
    suspend fun press(entity: String, record: Record, markId: String): Boolean = try {
        toggle(entity).toggle(withConfirmed(entity, record, markId), markId)
        true
    } catch (e: Exception) {
        if (e is kotlinx.coroutines.CancellationException) throw e
        false
    } finally {
        version++
    }

    private fun withConfirmed(entity: String, record: Record, markId: String): Record {
        val c = confirmed[Key(entity, record.id, markId)] ?: return record
        return record.copy(marks = record.marks.orEmpty() + (markId to c))
    }

    private fun toggle(entity: String): MarkToggle = synchronized(toggles) {
        toggles.getOrPut(entity) {
            MarkToggle(
                send = { id, mark, on ->
                    version++ // 上書きを置いた後・送る前
                    send(entity, id, mark, on)
                },
                onConfirmed = { id, mark, state -> confirmed[Key(entity, id, mark)] = state },
            )
        }
    }

    companion object {
        /** 出す印。共有の entity が宣言していて、一覧の `item.mark` が指している時だけ(Android の ListMarks.mark)。 */
        fun mark(item: ListItem?, entity: Entity?): Mark? {
            val id = item?.mark ?: return null
            return entity?.takeIf { it.isShared }?.mark(id)
        }

        /** アイコン(SF Symbols 名)。塗りつぶしがあるのは heart だけ(Android の ListMarks.symbol)。 */
        fun symbol(icon: String, on: Boolean): String {
            val base = when (icon) {
                "star", "bookmark", "eye", "flag" -> icon
                "thumbsup" -> "hand.thumbsup"
                "check" -> "checkmark.circle"
                else -> "heart"
            }
            return if (on && base == "heart") "heart.fill" else base
        }

        /**
         * appconfig へ送る。[serverId] で画面の id(作成を送る前の仮 id のことがある)をサーバーの id に替える。
         * まだ送っていない作成(サーバーの id が無い)は送らずに失敗にする。
         */
        fun live(client: AppConfigClient, appId: String, serverId: suspend (entity: String, recordId: String) -> String?): RowMarks =
            RowMarks { entity, recordId, markId, on ->
                val id = serverId(entity, recordId) ?: throw IllegalStateException("not sent yet: $recordId")
                withContext(Dispatchers.IO) { MarkToggle.sender(client, appId, entity)(id, markId, on) }
            }
    }
}
