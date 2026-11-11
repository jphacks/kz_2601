package jp.kz2601.metamon.desktop

import androidx.compose.foundation.ContextMenuArea
import androidx.compose.foundation.ContextMenuItem
import androidx.compose.foundation.background
import androidx.compose.foundation.border
import androidx.compose.foundation.clickable
import androidx.compose.foundation.gestures.Orientation
import androidx.compose.foundation.gestures.draggable
import androidx.compose.foundation.gestures.rememberDraggableState
import androidx.compose.foundation.horizontalScroll
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.BoxWithConstraints
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.IntrinsicSize
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxHeight
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.itemsIndexed
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.Card
import androidx.compose.material3.Checkbox
import androidx.compose.material3.DropdownMenu
import androidx.compose.material3.DropdownMenuItem
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateListOf
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.input.key.Key
import androidx.compose.ui.input.key.KeyEventType
import androidx.compose.ui.input.key.key
import androidx.compose.ui.input.key.onKeyEvent
import androidx.compose.ui.input.key.type
import androidx.compose.ui.input.pointer.PointerIcon
import androidx.compose.ui.input.pointer.pointerHoverIcon
import androidx.compose.ui.platform.LocalDensity
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import jp.kz2601.metamon.dsl.Action
import jp.kz2601.metamon.dsl.Component
import jp.kz2601.metamon.dsl.ComponentType
import jp.kz2601.metamon.dsl.ListItem
import jp.kz2601.metamon.dsl.Resets
import jp.kz2601.metamon.dsl.TableColumn
import jp.kz2601.metamon.dsl.expr.DSLClock
import jp.kz2601.metamon.dsl.expr.DailyCheck
import jp.kz2601.metamon.dsl.records.Aggregator
import jp.kz2601.metamon.dsl.records.Entity
import jp.kz2601.metamon.dsl.records.Field
import jp.kz2601.metamon.dsl.records.FieldCoding
import jp.kz2601.metamon.dsl.records.FieldType
import jp.kz2601.metamon.dsl.records.GridTableLayout
import jp.kz2601.metamon.dsl.records.JSONValue
import jp.kz2601.metamon.dsl.records.Record
import jp.kz2601.metamon.dsl.records.RecordQuery
import jp.kz2601.metamon.dsl.records.Sort
import jp.kz2601.metamon.dsl.records.TableGrouping
import jp.kz2601.metamon.renderer.form.FormPrefill
import java.awt.Cursor
import java.time.Instant

// データの部品(#1744): list・table・timeline・gridTable・searchBar・feed・matchList。stat・chart・calendar は DisplayLeaves.kt。
// 値は Android の renderer/components/lists/ListValues.kt(= iOS の各 Leaf の static な関数)と同じ規則。

val DATA_LEAVES: Set<ComponentType> = setOf(
    ComponentType.LIST, ComponentType.TABLE, ComponentType.TIMELINE, ComponentType.GRID_TABLE,
    ComponentType.SEARCH_BAR, ComponentType.FEED, ComponentType.MATCH_LIST,
)

/** データの部品が画面から受け取るもの。 */
class DataEnv(
    val data: LeafData,
    /** 画面の検索欄の文字(searchBar.field → 文字)。DesktopSession.queries(route)。 */
    val search: MutableMap<String, String>,
    val tap: (Action, String?) -> Unit,
    /** 1 件の記録の field を書き換える(list のチェック)。 */
    val update: (entity: String, id: String, patch: Map<String, String>) -> Unit,
    val delete: (entity: String, id: String) -> Unit,
    /** list の上に出す道具(表をコピー・貼り付け)。 */
    val listBar: @Composable (entity: String) -> Unit = {},
)

@Composable
fun DataLeaf(c: Component, env: DataEnv) {
    when (c.type) {
        ComponentType.LIST -> ListLeaf(c, env)
        ComponentType.TABLE -> TableLeaf(c, env.data)
        ComponentType.TIMELINE -> TimelineLeaf(c, env.data)
        ComponentType.GRID_TABLE -> GridTableLeaf(c, env)
        ComponentType.SEARCH_BAR -> SearchBarLeaf(c, env.search)
        ComponentType.FEED -> OfflineCard(c.title, "フィードはデスクトップ版ではまだ取得しません")
        ComponentType.MATCH_LIST -> OfflineCard(c.title, "仲間探しのつながりはサインインした後に出ます")
        else -> Unit
    }
}

// MARK: - 値(画面と適合テストが同じ関数を使う)

/** 表示用の文字(Android の LeafData.display)。member の名簿はまだ無いので「参加者」。 */
object DataText {
    const val EMPTY = "まだありません"
    const val EVERYONE = "全員"
    const val MEMBER = "参加者"

    fun join(items: List<String>): String = items.joinToString("、")

    fun display(data: LeafData, value: JSONValue, field: Field?): String = when (field?.type) {
        FieldType.REFS -> Aggregator.refIds(value).mapNotNull { refTitle(data, it, field.refEntity) }.let { if (it.isEmpty()) EVERYONE else join(it) }
        FieldType.MEMBER -> member(value.stringValue)
        FieldType.MEMBERS -> join(Aggregator.refIds(value).map(::member))
        FieldType.REF -> {
            val id = value.stringValue?.takeIf { it.isNotEmpty() }
            when {
                id == null -> ""
                // enum から ref に変えた後の古い記録は選択肢の名前そのものを持つ(#194)
                else -> refTitle(data, id, field.refEntity) ?: if (field.options?.contains(id) == true) id else FieldCoding.UNAVAILABLE
            }
        }
        else -> FieldCoding.display(value, field, data.locale, DSLClock.now())
    }

    fun text(data: LeafData, record: Record, fieldId: String?, entity: Entity?): String =
        fieldId?.let { display(data, record[it], entity?.field(it)) } ?: ""

    private fun member(uid: String?): String = if (uid.isNullOrEmpty()) "" else MEMBER

    private fun refTitle(data: LeafData, id: String, refEntity: String?): String? {
        val target = data.entity(refEntity ?: return null) ?: return null
        val record = data.evaluator.viewRecords(target.id).firstOrNull { it.id == id } ?: return null
        val tf = target.displayTitleField ?: return record.id
        return FieldCoding.display(record[tf], target.field(tf), data.locale, DSLClock.now())
    }
}

/** `list`: filter → sort → 検索。iOS の `ListLeaf.rows`・`ListRow.texts`。 */
object ListRows {
    data class Texts(val title: String, val subtitle: String?, val trailing: String?)

    fun rows(c: Component, data: LeafData, query: String?): List<Record> {
        val entity = data.entity(c.entity)
        val sorted = Aggregator.sorted(data.filtered(c), RecordQuery.sort(c.sort), entity)
        if (c.searchField == null) return sorted
        val q = query?.trim()?.takeIf { it.isNotEmpty() } ?: return sorted
        val fields = listOfNotNull(c.item?.title ?: entity?.titleField, c.item?.subtitle)
        return sorted.filter { r -> fields.any { DataText.display(data, r[it], entity?.field(it)).contains(q, ignoreCase = true) } }
    }

    fun texts(item: ListItem?, record: Record, entity: Entity?, data: LeafData): Texts {
        fun t(f: String?) = f?.let { DataText.display(data, record[it], entity?.field(it)) }
        return Texts(
            t(item?.title ?: entity?.displayTitleField) ?: "",
            t(item?.subtitle)?.takeIf { it.isNotEmpty() },
            t(item?.trailing)?.takeIf { it.isNotEmpty() },
        )
    }
}

/** `list.item.toggle`(#108)。iOS の `ListToggle`。 */
object ListToggle {
    fun field(item: ListItem?, entity: Entity?): Field? {
        val f = entity?.field(item?.toggle ?: return null) ?: return null
        if (f.isComputed) return null
        return f.takeIf { it.type == if (isDaily(item)) FieldType.DATE else FieldType.BOOLEAN }
    }

    fun isDaily(item: ListItem?): Boolean = item?.resets == Resets.DAILY

    fun isOn(record: Record, field: String, daily: Boolean, today: Instant): Boolean {
        if (daily) return DailyCheck.isDone(record[field], today)
        return when (val v = record[field]) {
            is JSONValue.Bool -> v.value
            is JSONValue.Num -> v.value != 0.0
            is JSONValue.Str -> v.value == "true"
            else -> false
        }
    }
}

/** `table`: 見出しと行の文字。iOS の `TableLeaf.cellRows` / `header`。 */
object TableCells {
    data class Row(val id: String, val texts: List<String>)

    fun rows(c: Component, data: LeafData): List<Row> {
        val entity = data.entity(c.entity)
        if (TableGrouping.isGrouped(c)) {
            return TableGrouping.rows(c, data.filtered(c)).map { row ->
                Row(row.key, c.columns.mapIndexed { i, col ->
                    if (col.aggregate == null) {
                        DataText.display(data, row.sample[col.field], entity?.field(col.field))
                    } else {
                        aggregateText(row.values[i], col.aggregate?.wire, entity?.field(col.field), data)
                    }
                })
            }
        }
        val all = Aggregator.sorted(data.filtered(c), RecordQuery.sort(c.sort), entity)
        val limit = c.limit ?: 0
        return (if (limit > 0) all.take(limit) else all).map { r -> Row(r.id, c.columns.map { DataText.text(data, r, it.field, entity) }) }
    }

    /** count は件数、sum/min/max は field の書式、avg は小数 1 桁(currency は円の整数)。 */
    private fun aggregateText(value: Double?, op: String?, field: Field?, data: LeafData): String {
        value ?: return FieldCoding.UNAVAILABLE
        if (op == "count") return FieldCoding.plainNumber(value)
        val shown = if (op == "avg" && field?.type != FieldType.CURRENCY) swiftRounded(value * 10) / 10 else value
        field ?: return FieldCoding.plainNumber(shown)
        return DataText.display(data, JSONValue.Num(shown), field)
    }

    fun header(col: TableColumn, entity: Entity?): String {
        col.title?.let { return it }
        val name = entity?.field(col.field)?.name ?: col.field
        return when (col.aggregate?.wire) {
            "count" -> "件数"
            "sum" -> "${name}の合計"
            "avg" -> "${name}の平均"
            "min" -> "${name}の最小"
            "max" -> "${name}の最大"
            else -> name
        }
    }

    fun isNumeric(col: TableColumn, entity: Entity?): Boolean = col.aggregate != null || entity?.field(col.field)?.type?.isNumeric == true

    /** 見出しを押した並べ替え(表示だけ・DSL の sort は変えない)。数として読める列は数で、それ以外は文字で比べる。 */
    fun viewSorted(rows: List<Row>, column: Int?, ascending: Boolean): List<Row> {
        column ?: return rows
        val cmp = Comparator<Row> { a, b ->
            val x = a.texts.getOrElse(column) { "" }
            val y = b.texts.getOrElse(column) { "" }
            val nx = number(x)
            val ny = number(y)
            if (nx != null && ny != null) nx.compareTo(ny) else TableGrouping.nameCompare(x, y)
        }
        return rows.sortedWith(if (ascending) cmp else cmp.reversed())
    }

    /** 「¥1,200」「3.5 km」→ 数。数字が無ければ null。 */
    internal fun number(s: String): Double? {
        val m = Regex("-?[0-9][0-9,]*(\\.[0-9]+)?").find(s) ?: return null
        return m.value.replace(",", "").toDoubleOrNull()
    }
}

/** `timeline`: dateField の新しい順・limit。iOS の `TimelineLeaf.rows`。 */
object TimelineRows {
    fun rows(c: Component, data: LeafData): List<Record> {
        val all = Aggregator.sorted(data.filtered(c), Sort(c.dateField ?: "", "desc"), data.entity(c.entity))
        val limit = c.limit ?: 0
        return if (limit > 0) all.take(limit) else all
    }
}

/** Swift の `rounded()`(0.5 は 0 から遠い方)。 */
private fun swiftRounded(x: Double): Double = if (x >= 0) kotlin.math.floor(x + 0.5) else -kotlin.math.floor(-x + 0.5)

// MARK: - 描画

private val LIST_MAX_HEIGHT = 520.dp

/**
 * `list`: 行は LazyColumn(1000 行でも見えている分だけ描く)。swipeToDelete はデスクトップでは右クリックの「削除」と Delete キー。
 */
@Composable
private fun ListLeaf(c: Component, env: DataEnv) {
    val data = env.data
    val entity = data.entity(c.entity)
    val rows = ListRows.rows(c, data, c.searchField?.let { env.search[it] })
    c.entity?.let { env.listBar(it) }
    if (rows.isEmpty()) {
        Text(c.emptyText ?: DataText.EMPTY, color = Color.Gray)
        return
    }
    val toggle = ListToggle.field(c.item, entity)
    val daily = ListToggle.isDaily(c.item)
    val now = DSLClock.now()
    val deletable = c.swipeToDelete == true && c.entity != null
    LazyColumn(Modifier.fillMaxWidth().heightIn(max = LIST_MAX_HEIGHT)) {
        itemsIndexed(rows, key = { _, r -> r.id }) { i, r ->
            val t = ListRows.texts(c.item, r, entity, data)
            val tap = c.onItemTap
            val delete = { env.delete(c.entity!!, r.id) }
            val row = @Composable {
                Row(
                    Modifier.fillMaxWidth()
                        .then(if (deletable) Modifier.onKeyEvent { e -> (e.type == KeyEventType.KeyUp && e.key == Key.Delete).also { if (it) delete() } } else Modifier)
                        .then(if (tap != null) Modifier.clickable { env.tap(tap, r.id) } else Modifier)
                        .padding(vertical = 6.dp),
                    verticalAlignment = Alignment.CenterVertically,
                    horizontalArrangement = Arrangement.spacedBy(8.dp),
                ) {
                    toggle?.let { f ->
                        val on = ListToggle.isOn(r, f.id, daily, now)
                        // 毎日リセットのチェックは日付の履歴を持つので、デスクトップ版はまだ見せるだけ
                        Checkbox(checked = on, onCheckedChange = if (daily) null else { v -> env.update(c.entity!!, r.id, mapOf(f.id to v.toString())) })
                    }
                    Column(Modifier.weight(1f)) {
                        Text(t.title, maxLines = 2, overflow = TextOverflow.Ellipsis)
                        t.subtitle?.let { Text(it, style = MaterialTheme.typography.bodySmall, color = Color.Gray, maxLines = 1, overflow = TextOverflow.Ellipsis) }
                    }
                    t.trailing?.let { Text(it, style = MaterialTheme.typography.bodyMedium, color = Color.Gray, maxLines = 1) }
                }
            }
            if (deletable) ContextMenuArea(items = { listOf(ContextMenuItem("削除", delete)) }) { row() } else row()
            if (i < rows.size - 1) HorizontalDivider()
        }
    }
}

private val MIN_COLUMN_W = 48.dp

/** `table`: 見出しの境目を引いて列の幅を変え、見出しを押して並べ替える(どちらも表示だけ)。 */
@Composable
private fun TableLeaf(c: Component, data: LeafData) {
    val entity = data.entity(c.entity)
    val columns = c.columns
    var sortColumn by remember(c.id) { mutableStateOf<Int?>(null) }
    var ascending by remember(c.id) { mutableStateOf(true) }
    val rows = TableCells.viewSorted(TableCells.rows(c, data), sortColumn, ascending)
    val density = LocalDensity.current
    BoxWithConstraints(Modifier.fillMaxWidth()) {
        val initial = with(density) { ((maxWidth - 8.dp * columns.size).coerceAtLeast(MIN_COLUMN_W) / columns.size.coerceAtLeast(1)).toPx() }
        val widths = remember(c.id, columns.size) { mutableStateListOf<Float>().apply { repeat(columns.size) { add(initial) } } }
        Column(Modifier.horizontalScroll(rememberScrollState())) {
            Row(Modifier.height(IntrinsicSize.Min), verticalAlignment = Alignment.CenterVertically) {
                columns.forEachIndexed { i, col ->
                    val mark = if (sortColumn == i) (if (ascending) " ▲" else " ▼") else ""
                    Text(
                        TableCells.header(col, entity) + mark,
                        Modifier.width(with(density) { widths[i].toDp() }).clickable {
                            if (sortColumn == i) ascending = !ascending else {
                                sortColumn = i
                                ascending = true
                            }
                        },
                        style = MaterialTheme.typography.labelLarge,
                        color = Color.Gray,
                        textAlign = if (TableCells.isNumeric(col, entity)) TextAlign.End else TextAlign.Start,
                        maxLines = 1,
                        overflow = TextOverflow.Ellipsis,
                    )
                    // 列の境目。左右に引くと列の幅が変わる
                    Box(
                        Modifier.width(8.dp).fillMaxHeight()
                            .pointerHoverIcon(PointerIcon(Cursor(Cursor.E_RESIZE_CURSOR)))
                            .draggable(
                                rememberDraggableState { d -> widths[i] = (widths[i] + d).coerceAtLeast(with(density) { MIN_COLUMN_W.toPx() }) },
                                Orientation.Horizontal,
                            ),
                    )
                }
            }
            HorizontalDivider(Modifier.padding(vertical = 6.dp))
            rows.forEach { row ->
                Row(Modifier.padding(vertical = 5.dp)) {
                    columns.forEachIndexed { i, col ->
                        Text(
                            row.texts.getOrElse(i) { "" },
                            Modifier.width(with(density) { widths[i].toDp() }),
                            textAlign = if (TableCells.isNumeric(col, entity)) TextAlign.End else TextAlign.Start,
                            maxLines = 1,
                            overflow = TextOverflow.Ellipsis,
                        )
                        Box(Modifier.width(8.dp))
                    }
                }
            }
        }
    }
    if (rows.isEmpty()) Text(DataText.EMPTY, color = Color.Gray)
}

/** `timeline`: 左に日付、レールと点、右に見出しと副題。 */
@Composable
private fun TimelineLeaf(c: Component, data: LeafData) {
    val entity = data.entity(c.entity)
    val rows = TimelineRows.rows(c, data)
    if (rows.isEmpty()) {
        Text(DataText.EMPTY, color = Color.Gray)
        return
    }
    val accent = MaterialTheme.colorScheme.primary
    Column {
        rows.forEachIndexed { index, r ->
            val sub = c.subtitleField?.let { DataText.text(data, r, it, entity) }?.takeIf { it.isNotEmpty() }
            Row(Modifier.fillMaxWidth().height(IntrinsicSize.Min), horizontalArrangement = Arrangement.spacedBy(12.dp)) {
                Text(
                    DataText.text(data, r, c.dateField, entity),
                    Modifier.width(84.dp),
                    style = MaterialTheme.typography.labelMedium,
                    color = Color.Gray,
                    textAlign = TextAlign.End,
                )
                Column(Modifier.fillMaxHeight(), horizontalAlignment = Alignment.CenterHorizontally) {
                    Box(Modifier.padding(top = 4.dp).size(10.dp).background(accent, CircleShape))
                    if (index < rows.size - 1) Box(Modifier.width(2.dp).weight(1f).background(Color(0x22000000)))
                }
                Column(Modifier.weight(1f).padding(bottom = 16.dp), verticalArrangement = Arrangement.spacedBy(2.dp)) {
                    Text(DataText.text(data, r, c.titleField, entity), fontWeight = FontWeight.SemiBold)
                    sub?.let { Text(it, style = MaterialTheme.typography.labelMedium, color = Color.Gray) }
                }
            }
        }
    }
}

/** `gridTable`(#234): 時間割のマス目。マスを押すと onItemTap(1 件は編集・空は新規・複数はメニュー)。 */
@Composable
private fun GridTableLeaf(c: Component, env: DataEnv) {
    val data = env.data
    val entity = data.entity(c.entity)
    val layout = GridTableLayout(c, entity, data.filtered(c))
    val today = GridTableLayout.todayColumn(layout.columns.map { it.label }, DSLClock.now())
    val accent = MaterialTheme.colorScheme.primary
    Column(verticalArrangement = Arrangement.spacedBy(4.dp)) {
        c.title?.takeIf { it.isNotEmpty() }?.let { Text(it, style = MaterialTheme.typography.titleSmall, color = Color.Gray) }
        Column(Modifier.horizontalScroll(rememberScrollState()), verticalArrangement = Arrangement.spacedBy(4.dp)) {
            Row(horizontalArrangement = Arrangement.spacedBy(4.dp)) {
                Box(Modifier.width(30.dp))
                layout.columns.forEachIndexed { i, col ->
                    Text(
                        col.label,
                        Modifier.width(80.dp),
                        textAlign = TextAlign.Center,
                        fontWeight = if (i == today) FontWeight.Bold else FontWeight.SemiBold,
                        color = if (i == today) accent else Color.Gray,
                        maxLines = 1,
                    )
                }
            }
            layout.rows.forEachIndexed { ri, row ->
                Row(horizontalArrangement = Arrangement.spacedBy(4.dp), verticalAlignment = Alignment.CenterVertically) {
                    Text(row.label, Modifier.width(30.dp), textAlign = TextAlign.Center, color = Color.Gray, maxLines = 2)
                    layout.columns.indices.forEach { ci -> GridCell(c, env, layout, entity, GridTableLayout.Cell(ci, ri), ci == today) }
                }
            }
        }
    }
}

@Composable
private fun GridCell(c: Component, env: DataEnv, layout: GridTableLayout, entity: Entity?, cell: GridTableLayout.Cell, isToday: Boolean) {
    val data = env.data
    val rows = layout.records(cell)
    val tap = c.onItemTap
    var menu by remember { mutableStateOf(false) }
    val titles = rows.map { r -> DataText.text(data, r, c.titleField ?: entity?.displayTitleField, entity).ifEmpty { "(無題)" } }
    val accent = MaterialTheme.colorScheme.primary
    val shape = RoundedCornerShape(6.dp)
    Box(
        Modifier.size(80.dp, 58.dp).clickable(enabled = tap != null) {
            when {
                rows.size > 1 -> menu = true
                rows.size == 1 -> env.tap(tap!!, rows[0].id)
                else -> addAt(env, tap!!, layout, cell)
            }
        },
    ) {
        if (rows.isEmpty()) {
            Box(Modifier.fillMaxSize().background(if (isToday) accent.copy(alpha = 0.06f) else Color(0x08000000), shape).border(1.dp, Color(0x1A000000), shape))
        } else {
            Column(
                Modifier.fillMaxSize().background(accent.copy(alpha = 0.14f), shape)
                    .border(if (isToday) 1.5.dp else 1.dp, accent.copy(alpha = if (isToday) 1f else 0.4f), shape)
                    .padding(horizontal = 5.dp, vertical = 4.dp),
            ) {
                Text(titles[0], style = MaterialTheme.typography.labelMedium, fontWeight = FontWeight.SemiBold, maxLines = 2, overflow = TextOverflow.Ellipsis)
                c.subtitleField?.takeIf { it.isNotEmpty() }?.let { f -> DataText.text(data, rows[0], f, entity) }
                    ?.takeIf { it.isNotEmpty() && it != FieldCoding.UNAVAILABLE }
                    ?.let { Text(it, style = MaterialTheme.typography.labelSmall, color = Color.Gray, maxLines = 1) }
            }
            if (rows.size > 1) {
                Text(
                    "+${rows.size - 1}",
                    Modifier.align(Alignment.BottomEnd).padding(3.dp).background(accent, CircleShape).padding(horizontal = 4.dp),
                    style = MaterialTheme.typography.labelSmall,
                    color = Color.White,
                )
            }
        }
        DropdownMenu(expanded = menu, onDismissRequest = { menu = false }) {
            rows.forEachIndexed { i, r -> DropdownMenuItem(text = { Text(titles[i]) }, onClick = { menu = false; tap?.let { env.tap(it, r.id) } }) }
            DropdownMenuItem(text = { Text("追加") }, onClick = { menu = false; tap?.let { addAt(env, it, layout, cell) } })
        }
    }
}

/** 空のマス・「追加」: 列・行の値を新規フォームの初期値にして開く(Android の FormPrefill)。 */
private fun addAt(env: DataEnv, tap: Action, layout: GridTableLayout, cell: GridTableLayout.Cell) {
    FormPrefill.set(layout.prefill(cell))
    env.tap(tap, null)
}

/** `searchBar`: 同じ画面の list(searchField が同じ)を絞る。 */
@Composable
private fun SearchBarLeaf(c: Component, search: MutableMap<String, String>) {
    val key = c.field ?: return
    val text = search[key].orEmpty()
    OutlinedTextField(
        value = text,
        onValueChange = { search[key] = it },
        placeholder = { Text(c.placeholder ?: "検索") },
        singleLine = true,
        trailingIcon = if (text.isEmpty()) null else { { TextButton(onClick = { search[key] = "" }) { Text("消す") } } },
        modifier = Modifier.fillMaxWidth(),
    )
}

/** 通信・サインインが要る部品(feed・matchList)。見出しと案内だけ出す。 */
@Composable
private fun OfflineCard(title: String?, note: String) {
    Card(Modifier.fillMaxWidth()) {
        Column(Modifier.padding(12.dp), verticalArrangement = Arrangement.spacedBy(4.dp)) {
            title?.takeIf { it.isNotEmpty() }?.let { Text(it, style = MaterialTheme.typography.titleSmall) }
            Text(note, style = MaterialTheme.typography.bodySmall, color = Color.Gray)
        }
    }
}
