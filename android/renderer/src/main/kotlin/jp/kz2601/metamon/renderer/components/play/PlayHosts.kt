package jp.kz2601.metamon.renderer.components.play

import jp.kz2601.metamon.dsl.records.JSONValue

/**
 * 部品から記録を書く口(iOS の `DSLRuntime.updateRecord` / `notifyReadOnly`)。:app の AppRuntime が実装する。
 * DSLRuntime を広げずに別の口にしている(実装していない runtime では書き込みを出さない)。
 */
interface RecordWriter {
    /** 1 件の更新。保存できたら true。変更できない人なら一言を出して false。 */
    suspend fun updateRecord(entity: String, recordId: String, data: Map<String, JSONValue>): Boolean

    /** 変更できない記録を触った時の一言。 */
    fun notifyReadOnly(entity: String) = Unit
}

/**
 * timerDisplay の終了通知と走行状態の口(iOS の `TimerNotifier` / `TimerRunStore` / `TimerVisibility`)。
 * [timerChanged] の [endsAtEpochMillis] が null なら予約を取り消す。キーは `<画面>/<部品 id>`(runtime が appId を前に付ける)。
 */
interface TimerHost {
    fun timerChanged(key: String, name: String, endsAtEpochMillis: Long?)

    /** 保存した走行状態(アプリを終了しても残る)。無ければ null。 */
    fun loadTimer(key: String): TimerRun? = null

    fun saveTimer(key: String, run: TimerRun) = Unit

    /** 前面で見えている間は終了のバナーを出さない。 */
    fun timerVisible(key: String, visible: Boolean) = Unit
}

/** 書き込みの順と失敗の扱い(iOS の `OptimisticSave`)。 */
object PlaySave {
    /** 順に保存し、途中で失敗したら残りは送らない(`settle` は全件に呼ぶ)。全部保存できたら true。 */
    suspend fun updateInOrder(
        writer: RecordWriter,
        entity: String,
        writes: List<Pair<String, Map<String, JSONValue>>>,
        settle: (String) -> Unit,
    ): Boolean {
        var failed = false
        for ((id, data) in writes) {
            if (!failed && !writer.updateRecord(entity, id, data)) failed = true
            settle(id)
        }
        return !failed
    }
}
