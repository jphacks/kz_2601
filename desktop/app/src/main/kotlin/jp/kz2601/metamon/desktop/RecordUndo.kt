package jp.kz2601.metamon.desktop

import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.focus.onFocusChanged
import androidx.compose.ui.semantics.LiveRegionMode
import androidx.compose.ui.semantics.liveRegion
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.unit.dp
import jp.kz2601.metamon.desktop.i18n.Strings
import kotlinx.coroutines.delay

/** 一覧・表の行から消した 1 件(#3351)。[index] は消す前の並びの位置(戻す時に同じ場所へ入れる)。 */
data class DeletionTicket(val id: Long, val entity: String, val row: Row, val index: Int)

/**
 * 「削除しました・元に戻す」の規則(Android の `AppRuntime.deleteUndoably`・iOS の `RecordUndo`)。
 * 帯は [MILLIS] で閉じる。戻すのは最後に消した 1 件だけ。
 */
object RecordUndo {
    /** 帯を出しておく時間(Android の `TalkBack.RECORD_UNDO_MILLIS` と同じ)。 */
    const val MILLIS = 5_000L

    /** 消す前の記録から札を作る。無い記録は null。 */
    fun ticket(store: MockStore, entity: String, id: String, seq: Long): DeletionTicket? {
        val list = store.list(entity)
        val i = list.indexOfFirst { it.id == id }
        return if (i < 0) null else DeletionTicket(seq, entity, list[i], i)
    }

    /** 消した場所に戻す。同じ id が既にある(他のウィンドウで戻した)時は null。 */
    fun restore(store: MockStore, t: DeletionTicket): MockStore? {
        val list = store.list(t.entity)
        if (list.any { it.id == t.row.id }) return null
        val at = t.index.coerceIn(0, list.size)
        return store.copy(rows = store.rows + (t.entity to list.take(at) + t.row + list.drop(at)))
    }
}

/**
 * ウィンドウの下の「削除しました・元に戻す」の帯。読み上げは「削除しました」を 1 回知らせる(live region)。
 * 帯の中にフォーカスがある間は閉じない(キーボードで「元に戻す」へ移る間に消えないように)。
 */
@Composable
fun RecordUndoBar(session: DesktopSession) {
    val ticket = session.recordUndo ?: return
    var focused by remember(ticket.id) { mutableStateOf(false) }
    LaunchedEffect(ticket.id, focused) {
        if (focused) return@LaunchedEffect
        delay(RecordUndo.MILLIS)
        session.dismissRecordUndo(ticket)
    }
    Surface(
        tonalElevation = 3.dp,
        modifier = Modifier.fillMaxWidth().onFocusChanged { focused = it.hasFocus },
    ) {
        Row(Modifier.padding(horizontal = 12.dp, vertical = 2.dp), verticalAlignment = Alignment.CenterVertically) {
            Text(
                Strings["feedback_deleted"],
                style = MaterialTheme.typography.bodySmall,
                modifier = Modifier.weight(1f).semantics { liveRegion = LiveRegionMode.Polite },
            )
            TextButton(onClick = { session.undoDelete() }) { Text(Strings["feedback_undo"]) }
            TextButton(onClick = { session.dismissRecordUndo(ticket) }) { Text(Strings["feedback_close"]) }
        }
    }
}
