package jp.kz2601.metamon.desktop.store

import java.util.UUID

/** 取り消せる削除の 1 件(iOS の `DeletionTicket`)。[UndoableDeletion.delete] が返し、[UndoableDeletion.undo] に渡す。 */
data class DeletionTicket(
    val appId: String,
    val entity: String,
    val recordId: String,
    val id: String = UUID.randomUUID().toString(),
)

/**
 * 一覧の「削除 → 元に戻す」の規則(iOS の `RecordSyncEngine+Undo`)。描画はしない。
 *
 * - 消した記録は [graceMillis] の間 [commit] を呼ばず、隠すだけにする([isHidden])。端末の記録にも送信待ちにも触れない
 * - その間に [undo] したら隠すのをやめるだけ。サーバーには何も送らないので、id・作成日時・作者は変わらない
 * - 時間が過ぎたら [tick] が [commit](普段の削除と同じ送信待ちに積む口)を呼ぶ
 * - 背面に回る・サインアウトの送り切りは [commitAll]。アプリの削除・アカウントの削除は [drop](送らずに捨てる)
 * - 待ちはメモリだけ。待ちの間に落ちたら削除されずに残る(消えすぎない側)
 *
 * @param now 時計(テストでは作り物を渡す)
 */
class UndoableDeletion(
    private val graceMillis: Long = GRACE_MILLIS,
    private val now: () -> Long = System::currentTimeMillis,
    private val commit: (DeletionTicket) -> Unit,
) {
    private class Pending(val ticket: DeletionTicket, val deadline: Long)

    private val pending = LinkedHashMap<String, Pending>()

    /** 隠して待ちに積む。[graceMillis] 後の [tick] で送る。 */
    @Synchronized
    fun delete(appId: String, entity: String, recordId: String): DeletionTicket {
        val ticket = DeletionTicket(appId, entity, recordId)
        pending[ticket.id] = Pending(ticket, now() + graceMillis)
        return ticket
    }

    /** 取り消す。送る前なら true(隠すのをやめる)。もう送った・捨てたなら false。 */
    @Synchronized
    fun undo(ticket: DeletionTicket): Boolean = pending.remove(ticket.id) != null

    /** 取り消しを待っている削除(積んだ順)。 */
    @get:Synchronized
    val pendingDeletions: List<DeletionTicket> get() = pending.values.map { it.ticket }

    /** 一覧から隠す記録か。 */
    @Synchronized
    fun isHidden(appId: String, entity: String, recordId: String): Boolean =
        pending.values.any { it.ticket.appId == appId && it.ticket.entity == entity && it.ticket.recordId == recordId }

    /** 一番早く時間が切れる時刻(待ちが無ければ null)。次に [tick] を呼ぶ時刻に使う。 */
    @Synchronized
    fun nextDeadline(): Long? = pending.values.minOfOrNull { it.deadline }

    /** 時間が切れた分を送る。送った分を返す。 */
    fun tick(): List<DeletionTicket> {
        val at = now()
        return commitWhere { it.deadline <= at }
    }

    /** 待たずに全部送る(背面に回った・サインアウト前の送り切り)。 */
    fun commitAll(): List<DeletionTicket> = commitWhere { true }

    /** 送らずに捨てる(アプリの削除・アカウントの削除。端末もサーバーも別の経路で消す)。捨てた数。 */
    @Synchronized
    fun drop(match: (DeletionTicket) -> Boolean = { true }): Int {
        val ids = pending.values.filter { match(it.ticket) }.map { it.ticket.id }
        ids.forEach { pending.remove(it) }
        return ids.size
    }

    /** そのアプリの待ちを捨てる(アプリを消した時)。 */
    fun dropApp(appId: String): Int = drop { it.appId == appId }

    private fun commitWhere(match: (Pending) -> Boolean): List<DeletionTicket> {
        val due = synchronized(this) {
            pending.values.filter(match).map { it.ticket }.onEach { pending.remove(it.id) }
        }
        // 積む口は待ちの外で呼ぶ(積む途中に undo・delete が来ても止まらない)
        due.forEach(commit)
        return due
    }

    companion object {
        /** Mac の `MacAppRuntime.undoSeconds` と同じ 5 秒。 */
        const val GRACE_MILLIS = 5_000L
    }
}
