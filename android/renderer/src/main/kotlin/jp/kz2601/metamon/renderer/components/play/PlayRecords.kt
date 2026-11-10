package jp.kz2601.metamon.renderer.components.play

import androidx.compose.runtime.Composable
import androidx.compose.runtime.State
import androidx.compose.runtime.collectAsState
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import jp.kz2601.metamon.dsl.Component
import jp.kz2601.metamon.dsl.records.Field
import jp.kz2601.metamon.dsl.records.Filter
import jp.kz2601.metamon.dsl.records.HiddenFields
import jp.kz2601.metamon.dsl.records.JSONValue
import jp.kz2601.metamon.dsl.records.Record
import jp.kz2601.metamon.dsl.records.filters
import jp.kz2601.metamon.renderer.DSLRuntime
import jp.kz2601.metamon.renderer.components.lists.LeafData
import jp.kz2601.metamon.renderer.components.lists.ValueLabels

/**
 * 計算・遊び系の部品が読む記録。中身は一覧・集計系と同じ [LeafData](iOS の `ComputedValues.swift` に当たる 1 か所)。
 * Compose に依らない関数にして JVM テストから呼ぶ。[all] は `runtime.recordsFlow` の今の値。
 */
object PlayRecords {
    /** 保存した記録そのもの(書き込み・トーナメントの固定の判定に使う)。 */
    fun records(all: Map<String, List<Record>>, entity: String): List<Record> = all[entity].orEmpty()

    /** computed を合成し hidden を落とした表示用の記録。 */
    fun viewRecords(runtime: DSLRuntime, all: Map<String, List<Record>>, entity: String): List<Record> =
        HiddenFields.redact(LeafData(runtime, all).viewRecords(entity), runtime.entityModel(entity))

    /** 部品の entity + filter(単体も配列も)を当てた表示用の記録。 */
    fun filtered(runtime: DSLRuntime, all: Map<String, List<Record>>, c: Component, filters: List<Filter> = c.filters): List<Record> =
        LeafData(runtime, all).filtered(c, filters)

    /** 表示用の文字列(一覧・集計系と同じ)。[labels] は画面なら端末の言語(既定は適合テストの日本語)。 */
    fun display(runtime: DSLRuntime, all: Map<String, List<Record>>, value: JSONValue, field: Field?, labels: ValueLabels = ValueLabels.JAPANESE): String =
        LeafData(runtime, all, labels = labels).display(value, field)
}

/** 表示中の記録(変わると描き直す)。runtime が記録を持たなければ空。 */
@Composable
internal fun rememberRecords(runtime: DSLRuntime): State<Map<String, List<Record>>> =
    runtime.recordsFlow?.collectAsState() ?: remember { mutableStateOf(emptyMap()) }
