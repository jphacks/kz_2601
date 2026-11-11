package jp.kz2601.metamon.renderer.components.inputs

import androidx.compose.material3.AlertDialog
import androidx.compose.material3.DatePicker
import androidx.compose.material3.DatePickerDialog
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.material3.TimePicker
import androidx.compose.material3.rememberDatePickerState
import androidx.compose.material3.rememberTimePickerState
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableIntStateOf
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalLocale
import androidx.compose.ui.res.stringResource
import jp.kz2601.metamon.dsl.Component
import jp.kz2601.metamon.dsl.records.FieldCoding
import jp.kz2601.metamon.dsl.records.FieldType
import jp.kz2601.metamon.renderer.R
import jp.kz2601.metamon.renderer.form.DateInput
import jp.kz2601.metamon.renderer.form.FormDraft
import jp.kz2601.metamon.renderer.form.TimeText
import jp.kz2601.metamon.renderer.form.date
import jp.kz2601.metamon.renderer.form.seedTimeIfEmpty
import jp.kz2601.metamon.renderer.form.setDate
import jp.kz2601.metamon.renderer.form.setTime
import jp.kz2601.metamon.renderer.form.time
import jp.kz2601.metamon.renderer.registry.RenderContext
import java.time.ZonedDateTime

// 日時の入力。date は "YYYY-MM-DD"、datetime は RFC 3339、timePicker の string は "HH:mm"(iOS と同じ形)。

/** `datePicker`: field が datetime(か includeTime)なら日付のあと時刻も選ぶ。 */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun DatePickerLeaf(c: Component, ctx: RenderContext, modifier: Modifier = Modifier) = FormInput(c, ctx, modifier) { form, field ->
    val includeTime = c.includeTime ?: (form.entity.field(field)?.type == FieldType.DATETIME)
    var step by remember { mutableIntStateOf(0) } // 0: 閉じる・1: 日付・2: 時刻
    val at = form.date(field)
    InputRow(InputRules.label(c, form).orEmpty()) {
        TextButton(onClick = { step = 1 }) { Text(DateInput.text(at, includeTime, LocalLocale.current.platformLocale)) }
    }
    if (step == 1) {
        val state = rememberDatePickerState(initialSelectedDateMillis = DateInput.pickerMillis(at))
        DatePickerDialog(
            onDismissRequest = { step = 0 },
            confirmButton = {
                TextButton(onClick = {
                    state.selectedDateMillis?.let { form.setDate(field, DateInput.fromPicker(it, at)) }
                    step = if (includeTime) 2 else 0
                }) { Text("OK") }
            },
            dismissButton = { TextButton(onClick = { step = 0 }) { Text(stringResource(R.string.inputs_cancel)) } },
        ) { DatePicker(state) }
    }
    if (step == 2) TimeDialog(form, field) { step = 0 }
}

/** `timePicker`: 時:分だけ。未入力なら見えている今の時刻を保存値にする(#229)。 */
@Composable
fun TimePickerLeaf(c: Component, ctx: RenderContext, modifier: Modifier = Modifier) = FormInput(c, ctx, modifier) { form, field ->
    LaunchedEffect(form, field) { form.seedTimeIfEmpty(field) }
    var open by remember { mutableStateOf(false) }
    val isDatetime = form.entity.field(field)?.type == FieldType.DATETIME
    val t = if (isDatetime) ZonedDateTime.ofInstant(form.date(field), FieldCoding.zone).toLocalTime() else form.time(field)
    InputRow(InputRules.label(c, form).orEmpty()) {
        TextButton(onClick = { open = true }) { Text(TimeText.format(t.hour, t.minute)) }
    }
    if (open) TimeDialog(form, field) { open = false }
}

@OptIn(ExperimentalMaterial3Api::class)
@Composable
private fun TimeDialog(form: FormDraft, field: String, onDone: () -> Unit) {
    val isDatetime = form.entity.field(field)?.type == FieldType.DATETIME
    val t = if (isDatetime) ZonedDateTime.ofInstant(form.date(field), FieldCoding.zone).toLocalTime() else form.time(field)
    val state = rememberTimePickerState(initialHour = t.hour, initialMinute = t.minute, is24Hour = true)
    AlertDialog(
        onDismissRequest = onDone,
        confirmButton = {
            TextButton(onClick = {
                form.setTime(field, state.hour, state.minute)
                onDone()
            }) { Text("OK") }
        },
        dismissButton = { TextButton(onClick = onDone) { Text(stringResource(R.string.inputs_cancel)) } },
        text = { TimePicker(state) },
    )
}
