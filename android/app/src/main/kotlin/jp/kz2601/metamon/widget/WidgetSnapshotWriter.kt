package jp.kz2601.metamon.widget

import jp.kz2601.metamon.dsl.AppDocument
import jp.kz2601.metamon.dsl.records.Record
import jp.kz2601.metamon.renderer.DSLRuntime
import jp.kz2601.metamon.renderer.EntityLoadState
import jp.kz2601.metamon.renderer.components.lists.LeafData
import jp.kz2601.metamon.renderer.components.lists.ValueLabels
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.FlowPreview
import kotlinx.coroutines.Job
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.combine
import kotlinx.coroutines.flow.debounce
import kotlinx.coroutines.launch

/**
 * DSL の適用・記録の変化でウィジェットの中身を書き直し、変わった時だけ再描画を頼む(iOS の `WidgetSnapshotWriter.write`)。
 * 時刻で読みに行かない(変わった時だけ)。
 */
class WidgetSnapshotWriter(
    private val store: WidgetSnapshotStore,
    private val labels: () -> ValueLabels = { ValueLabels.JAPANESE },
    /** 中身が変わった(消えた)時。ウィジェットの再描画(`updateAll`)を頼む。 */
    private val onChanged: () -> Unit = {},
) {
    /** 1 回分。書いた・消したら true。 */
    fun write(
        document: AppDocument,
        runtime: DSLRuntime,
        records: Map<String, List<Record>>,
        loadStates: Map<String, EntityLoadState> = emptyMap(),
    ): Boolean {
        val previous = store.load()
        val data = LeafData(runtime, records, labels = labels(), loadStates = loadStates)
        val next = WidgetSnapshotBuilder.build(document.appId, document.dsl, data, previous)
        if (next == null) {
            // 初期状態は消して「アプリを作るとここに出ます」に戻す
            if (previous == null) return false
            store.clear()
        } else {
            if (next.hasSameContent(previous)) return false
            store.save(next)
        }
        onChanged()
        return true
    }

    /** 消す(サインアウト・端末のデータの後片付け)。 */
    fun clear() {
        store.clear()
        onChanged()
    }

    /** 表示中のアプリを消した時だけ消す。 */
    fun forgetApp(appId: String) {
        if (store.clearIfShowing(appId)) onChanged()
    }

    /** ランタイムの DSL・記録・読み込み状態を見張る。続けて変わる間はまとめる。 */
    @OptIn(FlowPreview::class)
    fun start(
        scope: CoroutineScope,
        runtime: DSLRuntime,
        documents: Flow<AppDocument>,
        records: Flow<Map<String, List<Record>>>,
        loadStates: Flow<Map<String, EntityLoadState>>,
    ): Job = scope.launch {
        combine(documents, records, loadStates, ::Triple)
            .debounce(DEBOUNCE_MILLIS)
            .collect { (doc, recs, states) -> write(doc, runtime, recs, states) }
    }

    companion object {
        const val DEBOUNCE_MILLIS = 500L
    }
}
