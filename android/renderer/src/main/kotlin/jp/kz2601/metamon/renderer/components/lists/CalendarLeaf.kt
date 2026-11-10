package jp.kz2601.metamon.renderer.components.lists

import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.widthIn
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Brush
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.semantics.clearAndSetSemantics
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.semantics.heading
import androidx.compose.ui.semantics.selected
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.semantics.stateDescription
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.unit.dp
import jp.kz2601.metamon.dsl.Component
import jp.kz2601.metamon.dsl.ListItem
import jp.kz2601.metamon.dsl.expr.DSLClock
import jp.kz2601.metamon.dsl.records.FieldCoding
import jp.kz2601.metamon.dsl.records.Record
import jp.kz2601.metamon.renderer.ActionContext
import jp.kz2601.metamon.renderer.form.FormPrefill
import jp.kz2601.metamon.renderer.registry.RenderContext
import jp.kz2601.metamon.renderer.theme.RenderTheme
import jp.kz2601.metamon.renderer.theme.SymbolIcon
import jp.kz2601.metamon.renderer.theme.colors
import jp.kz2601.metamon.renderer.theme.renderCard
import java.time.LocalDate
import java.time.YearMonth
import java.time.format.DateTimeFormatter
import java.time.format.FormatStyle
import java.time.format.TextStyle
import java.time.temporal.WeekFields
import java.util.Locale

/** 月の表示に使う値(テストする)。 */
object CalendarMonth {
    /** 先頭の空白(null)+ 月の各日。週の始まりは端末の地域の設定。 */
    fun cells(month: YearMonth, locale: Locale = Locale.getDefault()): List<LocalDate?> {
        val first = month.atDay(1)
        val firstDow = WeekFields.of(locale).firstDayOfWeek
        val lead = (first.dayOfWeek.value - firstDow.value + 7) % 7
        return List(lead) { null } + (1..month.lengthOfMonth()).map { month.atDay(it) }
    }

    /** 日 → その日の記録(dateField の日付・端末のタイムゾーン)。 */
    fun byDay(records: List<Record>, dateField: String): Map<LocalDate, List<Record>> {
        val out = LinkedHashMap<LocalDate, MutableList<Record>>()
        for (r in records) {
            val d = FieldCoding.date(r[dateField]) ?: continue
            out.getOrPut(LocalDate.ofInstant(d, FieldCoding.zone)) { mutableListOf() }.add(r)
        }
        return out
    }
}

/**
 * `calendar`: 自前の月カレンダー。iOS の `CalendarLeaf`。`dateField` で記録を日に載せ、記録のある日に点を打つ。
 * 日をタップ → その日の記録を下に並べる。行タップ = `onDateTap` に recordId を渡す(編集)、
 * 「追加」= 選んだ日を `dateField` の初期値にして `onDateTap` を実行(新規・`FormPrefill`)。
 */
@Composable
fun CalendarLeaf(c: Component, ctx: RenderContext, modifier: Modifier = Modifier) {
    val data = rememberLeafData(ctx.runtime)
    val locale = Locale.getDefault()
    val today = LocalDate.ofInstant(DSLClock.now(), FieldCoding.zone)
    var month by remember { mutableStateOf(YearMonth.from(today)) }
    var selected by remember { mutableStateOf(today) }
    val entityId = c.entity ?: ""
    val entity = data.entity(entityId)
    val dateField = c.dateField ?: ""
    val days = CalendarMonth.byDay(data.viewRecords(entityId), dateField)
    val colors = RenderTheme.colors
    val dayLabel = DateTimeFormatter.ofLocalizedDate(FormatStyle.FULL).withLocale(locale)

    Column(modifier.renderCard(), verticalArrangement = Arrangement.spacedBy(10.dp)) {
        Row(verticalAlignment = Alignment.CenterVertically) {
            c.title?.takeIf { it.isNotEmpty() }?.let {
                Text(it, style = RenderTheme.Fonts.sectionTitle, color = MaterialTheme.colorScheme.onSurfaceVariant)
            }
            Spacer(Modifier.weight(1f))
            IconButton(onClick = { month = month.minusMonths(1) }, Modifier.semantics { contentDescription = "前の月" }) {
                SymbolIcon("chevron.left", 22.dp, MaterialTheme.colorScheme.onSurface)
            }
            Text(
                month.atDay(1).format(DateTimeFormatter.ofPattern(if (locale.language == "ja") "yyyy年M月" else "MMMM yyyy", locale)),
                Modifier.widthIn(min = 110.dp).semantics { heading() },
                style = MaterialTheme.typography.titleMedium.copy(fontWeight = FontWeight.SemiBold, fontFeatureSettings = "tnum"),
                textAlign = TextAlign.Center,
                maxLines = 1,
            )
            IconButton(onClick = { month = month.plusMonths(1) }, Modifier.semantics { contentDescription = "次の月" }) {
                SymbolIcon("chevron.right", 22.dp, MaterialTheme.colorScheme.onSurface)
            }
        }
        // 曜日は各日の読み上げに含めるので見出し行は飛ばす
        val firstDow = WeekFields.of(locale).firstDayOfWeek
        Row(Modifier.fillMaxWidth().clearAndSetSemantics { }) {
            repeat(7) { i ->
                Text(
                    firstDow.plus(i.toLong()).getDisplayName(TextStyle.NARROW_STANDALONE, locale),
                    Modifier.weight(1f),
                    style = MaterialTheme.typography.labelSmall,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                    textAlign = TextAlign.Center,
                )
            }
        }
        CalendarMonth.cells(month, locale).chunked(7).forEach { week ->
            Row(Modifier.fillMaxWidth()) {
                week.forEach { date ->
                    Box(Modifier.weight(1f)) {
                        if (date != null) {
                            DayCell(date, days[date]?.size ?: 0, date == selected, date == today, dayLabel) { selected = date }
                        }
                    }
                }
                repeat(7 - week.size) { Spacer(Modifier.weight(1f)) }
            }
        }
        HorizontalDivider(color = colors.hairline)
        Row(verticalAlignment = Alignment.CenterVertically) {
            Text(
                selected.format(DateTimeFormatter.ofPattern(if (locale.language == "ja") "M月d日(E)" else "EEE, MMM d", locale)),
                Modifier.weight(1f),
                style = MaterialTheme.typography.bodyMedium.copy(fontWeight = FontWeight.SemiBold),
            )
            c.onDateTap?.let { action ->
                TextButton(onClick = {
                    entity?.field(dateField)?.let { f ->
                        FormPrefill.set(mapOf(dateField to jp.kz2601.metamon.dsl.records.JSONValue.Str(
                            FieldCoding.encode(selected.atStartOfDay(FieldCoding.zone).toInstant(), f),
                        )))
                    }
                    ctx.dispatch(action, ActionContext(ctx.route, ctx.form))
                }) {
                    SymbolIcon("plus", 18.dp, colors.brandText)
                    Text("追加", color = colors.brandText, fontWeight = FontWeight.SemiBold)
                }
            }
        }
        val rows = days[selected].orEmpty()
        if (rows.isEmpty()) {
            Text("予定はありません", style = MaterialTheme.typography.labelMedium, color = MaterialTheme.colorScheme.onSurfaceVariant)
        } else {
            val item = ListItem(title = entity?.titleField ?: dateField)
            rows.forEach { row ->
                ListRow(
                    item = item,
                    record = row,
                    entity = entity,
                    data = data,
                    onTap = c.onDateTap?.let { a -> { ctx.dispatch(a, ActionContext(ctx.route, ctx.form, recordId = row.id)) } },
                )
            }
        }
    }
}

/** 1 日。読み上げは「2026年10月24日金曜日, 記録2件(、今日)」+ 選択中。 */
@Composable
private fun DayCell(date: LocalDate, count: Int, isSelected: Boolean, isToday: Boolean, label: DateTimeFormatter, onClick: () -> Unit) {
    val colors = RenderTheme.colors
    val bg = if (isSelected) Brush.linearGradient(listOf(colors.brandFillStart, colors.brandFillEnd)) else Brush.linearGradient(listOf(Color.Transparent, Color.Transparent))
    Column(
        Modifier
            .fillMaxWidth()
            .height(40.dp)
            .background(bg, RoundedCornerShape(10.dp))
            .clickable(onClick = onClick)
            .clearAndSetSemantics {
                contentDescription = date.format(label) + if (isToday) "、今日" else ""
                stateDescription = if (count > 0) "記録${count}件" else "記録なし"
                selected = isSelected
            },
        horizontalAlignment = Alignment.CenterHorizontally,
        verticalArrangement = Arrangement.Center,
    ) {
        Text(
            "${date.dayOfMonth}",
            style = MaterialTheme.typography.bodyMedium.copy(fontWeight = if (isToday) FontWeight.Bold else FontWeight.Normal, fontFeatureSettings = "tnum"),
            color = when {
                isSelected -> Color.White
                isToday -> colors.brandText
                else -> MaterialTheme.colorScheme.onSurface
            },
        )
        Box(Modifier.size(5.dp).background(if (count > 0) (if (isSelected) Color.White else colors.brandA) else Color.Transparent, CircleShape))
    }
}
