package jp.kz2601.metamon.renderer.components.lists

import androidx.compose.runtime.Composable
import androidx.compose.runtime.collectAsState
import androidx.compose.runtime.remember
import jp.kz2601.metamon.dsl.Component
import jp.kz2601.metamon.dsl.expr.DSLClock
import jp.kz2601.metamon.dsl.expr.ExprEvaluator
import jp.kz2601.metamon.dsl.records.Aggregator
import jp.kz2601.metamon.dsl.records.Entity
import jp.kz2601.metamon.dsl.records.Field
import jp.kz2601.metamon.dsl.records.FieldCoding
import jp.kz2601.metamon.dsl.records.FieldType
import jp.kz2601.metamon.dsl.records.Filter
import jp.kz2601.metamon.dsl.records.HiddenFields
import jp.kz2601.metamon.dsl.records.JSONValue
import jp.kz2601.metamon.dsl.records.Record
import jp.kz2601.metamon.dsl.records.RecordQuery
import jp.kz2601.metamon.dsl.records.filters
import jp.kz2601.metamon.renderer.DSLRuntime
import jp.kz2601.metamon.renderer.EntityLoadState
import jp.kz2601.metamon.renderer.components.social.followedFor
import jp.kz2601.metamon.renderer.components.social.follows
import java.time.Instant

/**
 * 一覧・集計・グラフの Leaf が読む記録と表示(iOS の `DSLRuntime` の `filteredRecords` / `display` / `evaluator`)。
 * 描画 1 回につき 1 つ作る(computed・agg の結果をメモする)。適合テストの描画の木(`RenderTree`)も同じものを使う。
 */
class LeafData(
    val runtime: DSLRuntime,
    private val records: Map<String, List<Record>>,
    val now: Instant = DSLClock.now(),
    /** 値の文言。既定は適合テストの値(iOS の日本語)。画面は [rememberLeafData] が端末の言語で渡す。 */
    val labels: ValueLabels = ValueLabels.JAPANESE,
    /** entity ごとの読み込み状態(#1116)。無い entity は読めた扱い。 */
    private val loadStates: Map<String, EntityLoadState> = emptyMap(),
) {
    val evaluator: ExprEvaluator = ExprEvaluator(runtime.entityModels, { records[it].orEmpty() }, now)

    fun entity(id: String?): Entity? = id?.let(runtime::entityModel)

    /** entity の読み込み状態。entity が無い部品・ホストが状態を持たない時は読めた扱い。 */
    fun loadState(entity: String?): EntityLoadState = entity?.let { loadStates[it] } ?: EntityLoadState.LOADED

    /** 0 件の時に出す形(骨組み・取得失敗・空の案内)。 */
    fun placeholder(entity: String?): RecordsPlaceholder = RecordsPlaceholder.of(loadState(entity))

    /** entity の記録(computed 合成前・filter 前)。 */
    fun records(entity: String): List<Record> = records[entity].orEmpty()

    /** computed を合成し filter(既定は部品の filter・単体も配列も AND)を通した記録(hidden は落とす)。 */
    fun filtered(c: Component, filters: List<Filter> = c.filters): List<Record> =
        RecordQuery.filtered(c, entity(c.entity), evaluator, runtime.currentUid(), runtime.followedFor(filters), filters)

    /** computed を合成した記録(filter なし・calendar)。 */
    fun viewRecords(entity: String): List<Record> = evaluator.viewRecords(entity)

    /** computed を合成し hidden を落とした記録(filter なし・計算・遊び系)。 */
    fun visibleRecords(entity: String): List<Record> = HiddenFields.redact(viewRecords(entity), entity(entity))

    /** 表示用の文字列。ref / refs は参照先の見出し、member は表示名にする。 */
    fun display(value: JSONValue, field: Field?): String {
        when (field?.type) {
            FieldType.REFS -> {
                val names = Aggregator.refIds(value).mapNotNull { refTitle(it, field.refEntity) }
                return if (names.isEmpty()) labels.everyone else labels.joinList(names)
            }
            FieldType.MEMBER -> return memberName(value.stringValue)
            FieldType.MEMBERS -> return labels.joinList(Aggregator.refIds(value).map { memberName(it) })
            FieldType.REF -> {
                val id = value.stringValue?.takeIf { it.isNotEmpty() } ?: return ""
                // enum から ref に変えた後の古い記録は選択肢の名前そのものを持つ(#194)
                return refTitle(id, field.refEntity) ?: if (field.options?.contains(id) == true) id else FieldCoding.UNAVAILABLE
            }
            else -> return FieldCoding.display(value, field, now = now)
        }
    }

    /** レコードの field を表示用に。field id が null なら空。 */
    fun text(record: Record, fieldId: String?, entity: Entity?): String =
        fieldId?.let { display(record[it], entity?.field(it)) } ?: ""

    private fun refTitle(id: String, refEntity: String?): String? {
        val ref = refEntity ?: return null
        val target = entity(ref) ?: return null
        val record = records(ref).firstOrNull { it.id == id } ?: return null
        val tf = target.displayTitleField ?: return record.id
        return FieldCoding.display(record[tf], target.field(tf), now = now)
    }

    private fun memberName(uid: String?): String = memberName(runtime, uid, labels.member)

    companion object {
        /**
         * type member の値(uid)の表示名。未選択は空。一覧・遊び系・チャットはここを使う。
         * 名簿に無い(抜けた・読む前)・表示名が空なら [fallback](iOS の `MembersHost` と同じ・#1076)。
         */
        fun memberName(runtime: DSLRuntime, uid: String?, fallback: String): String {
            if (uid.isNullOrEmpty()) return ""
            return runtime.formMembers.firstOrNull { it.uid == uid }?.displayName?.takeIf { it.isNotEmpty() } ?: fallback
        }
    }
}

/** 描画中の記録を読む。記録が変わると描き直す。 */
@Composable
fun rememberLeafData(runtime: DSLRuntime): LeafData {
    val flow = runtime.recordsFlow
    val records = flow?.collectAsState()?.value.orEmpty()
    val labels = rememberValueLabels()
    val loadStates = runtime.loadStatesFlow?.collectAsState()?.value.orEmpty()
    val follows = runtime.follows?.followsVersion?.collectAsState()?.value // {"myFollows": true} の一覧が変わったら描き直す
    return remember(runtime, records, labels, follows, loadStates) { LeafData(runtime, records, labels = labels, loadStates = loadStates) }
}
