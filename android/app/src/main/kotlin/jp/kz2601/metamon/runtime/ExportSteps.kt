package jp.kz2601.metamon.runtime

import jp.kz2601.metamon.R
import jp.kz2601.metamon.dsl.Action
import jp.kz2601.metamon.dsl.ActionFormat
import jp.kz2601.metamon.dsl.logic.CsvWriter
import jp.kz2601.metamon.dsl.logic.ExportTable
import jp.kz2601.metamon.dsl.logic.RecordExport
import jp.kz2601.metamon.dsl.records.RecordQuery
import jp.kz2601.metamon.renderer.components.lists.LeafData
import jp.kz2601.metamon.renderer.components.lists.ValueLabels
import jp.kz2601.metamon.renderer.components.social.followedFor
import java.io.File
import java.util.UUID

/**
 * `export` の中身(#911・iOS の `makeExportFile` の前段)。Android の部品に触らないので JVM で確かめる。
 * 表は list / table と同じ経路(computed 合成・filter・display の書式)で作る。hidden は RecordExport が行ごとに落とす。
 */
object ExportSteps {
    sealed interface Prepared {
        data class Ready(val table: ExportTable, val format: ActionFormat) : Prepared

        /** 書き出さない。toast に出す一言 */
        data class Invalid(val message: UiText) : Prepared
    }

    val NO_ENTITY = UiText.Res(R.string.host_export_no_entity)
    val FAILED = UiText.Res(R.string.host_export_failed)

    /** [notes] は件数 → PDF の見出し下の補足(書き出し日・件数。画面の言語で作るのはホスト)。 */
    suspend fun prepare(
        action: Action,
        runtime: AppRuntime,
        labels: ValueLabels = ValueLabels.JAPANESE,
        notes: (count: Int) -> List<String> = { emptyList() },
    ): Prepared {
        val entityId = action.entity ?: return Prepared.Invalid(NO_ENTITY)
        val entity = runtime.entityModel(entityId) ?: return Prepared.Invalid(NO_ENTITY)
        val format = action.format?.takeIf { it != ActionFormat.UNKNOWN } ?: return Prepared.Invalid(FAILED)
        // 参照先の名前(ref の見出し)も表に出すので一緒に読む
        runtime.load(setOf(entityId) + entity.fields.mapNotNull { it.refEntity })
        val leaf = LeafData(runtime, runtime.recordsFlow.value, labels = labels)
        val filters = RecordQuery.filters(action.filter)
        val rows = leaf.viewRecords(entityId)
        val filtered = if (filters.isEmpty()) {
            rows
        } else {
            val followed = runtime.followedFor(filters)
            leaf.evaluator.apply(filters.map { it.resolvingFollows(followed).resolvingMe(runtime.currentUid()) }, entity, rows)
        }
        val records = RecordExport.chronological(filtered, entity)
        val title = action.title?.trim()?.ifEmpty { null } ?: entity.name
        val table = RecordExport.table(title, entity, records, action.fields, runtime.currentUid(), notes(records.size)) { v, f -> leaf.display(v, f) }
        return Prepared.Ready(table, format)
    }

    /** ファイルの中身と拡張子。PDF の描画は端末の部品なので渡してもらう。 */
    fun encode(table: ExportTable, format: ActionFormat, pdf: (ExportTable) -> ByteArray): Pair<ByteArray, String> = when (format) {
        ActionFormat.CSV -> CsvWriter.data(table) to "csv"
        ActionFormat.PDF -> pdf(table) to "pdf"
        ActionFormat.UNKNOWN -> throw IllegalArgumentException("unknown format")
    }

    /** 書き出しの置き場(cache/metamon_export・FileProvider の metamon_export と揃える)。 */
    const val DIR = "metamon_export"

    /**
     * [cacheDir]/metamon_export/<uuid>/<ファイル名> に書く。前回までの書き出し(記録の写し)は先に消す
     * (共有シートがいつ読み終えるか分からないので、iOS と同じく次の書き出しで消す)。
     */
    fun writeFile(cacheDir: File, fileName: String, bytes: ByteArray): File {
        val root = File(cacheDir, DIR)
        root.deleteRecursively()
        val dir = File(root, UUID.randomUUID().toString()).apply { mkdirs() }
        return File(dir, fileName).apply { writeBytes(bytes) }
    }

    /** 書き出しを全部消す(アカウントの削除・サインアウト)。 */
    fun removeAll(cacheDir: File) {
        File(cacheDir, DIR).deleteRecursively()
    }
}
