package jp.kz2601.metamon.renderer.components.inputs

import androidx.compose.foundation.border
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.RowScope
import androidx.compose.foundation.layout.defaultMinSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.semantics
import androidx.compose.ui.semantics.testTag
import androidx.compose.ui.unit.dp
import jp.kz2601.metamon.dsl.Component
import jp.kz2601.metamon.dsl.ComponentType
import jp.kz2601.metamon.dsl.records.Field
import jp.kz2601.metamon.dsl.records.FieldType
import jp.kz2601.metamon.renderer.R
import jp.kz2601.metamon.renderer.form.FormDraft
import jp.kz2601.metamon.renderer.registry.RenderContext
import jp.kz2601.metamon.renderer.theme.RenderTheme
import jp.kz2601.metamon.renderer.theme.colors

/** 入力部品の描き分け(iOS の `RenderTree.state` と `FormFieldOverride`)。View と適合テストの木が同じ規則を使う。 */
object InputRules {
    /** 束縛する field の型で描き替える入力(11 種・iOS の `FormFieldOverride.inputTypes`)。 */
    val overridable: Set<ComponentType> = setOf(
        ComponentType.TEXT_FIELD, ComponentType.NUMBER_FIELD, ComponentType.TEXT_EDITOR, ComponentType.SECURE_FIELD,
        ComponentType.TOGGLE, ComponentType.PICKER, ComponentType.DATE_PICKER, ComponentType.TIME_PICKER,
        ComponentType.SLIDER, ComponentType.STEPPER, ComponentType.RATING,
    )

    /** field を書く入力(#858 の 12 種 + 写真・手書き #862)。 */
    val inputTypes: Set<ComponentType> =
        overridable + ComponentType.COLOR_PICKER + ComponentType.DRAWING_FIELD + ComponentType.PHOTO_FIELD + ComponentType.VIDEO_FIELD

    /** form の field に書く部品(iOS の `RenderTree.formBound`)。form が無いと needsForm、props に value と label を出す。 */
    val formBound: Set<ComponentType> = inputTypes + setOf(
        ComponentType.QR_SCANNER,
    )

    /** ラベルを `c.label ?: field.name` で補う入力(1 行の器)。他は `c.label` だけ。 */
    val labelFallsBackToFieldName: Set<ComponentType> =
        setOf(ComponentType.TOGGLE, ComponentType.DATE_PICKER, ComponentType.COLOR_PICKER, ComponentType.STEPPER, ComponentType.TIME_PICKER)

    /** computed / ref / refs / member / members の field なら、部品の種類によらずその入力で描く。 */
    fun override(c: Component, form: FormDraft?): Field? {
        if (form == null || c.type !in overridable) return null
        val f = form.entity.field(c.field ?: return null) ?: return null
        return f.takeIf { it.isComputed || it.type in overrideTypes }
    }

    private val overrideTypes = setOf(FieldType.REF, FieldType.REFS, FieldType.MEMBER, FieldType.MEMBERS)

    /** rendered / hidden / needsForm(登録済みの入力部品について)。 */
    fun state(c: Component, form: FormDraft?, currentUid: String?): String {
        if (form != null && form.hides(c, currentUid)) return "hidden"
        if (override(c, form) != null) return "rendered"
        if (form == null || c.field == null) return "needsForm"
        return "rendered"
    }

    /** 部品のラベル。 */
    fun label(c: Component, form: FormDraft): String? {
        val fid = c.field ?: return c.label
        if (c.type !in labelFallsBackToFieldName) return c.label
        return c.label ?: form.entity.field(fid)?.name ?: (if (c.type == ComponentType.TIME_PICKER) "" else fid)
    }
}

/** 入力部品の入口: 隠す・フォームの外・型での描き替えを先に決めてから [content] を描く。 */
@Composable
fun FormInput(c: Component, ctx: RenderContext, modifier: Modifier, content: @Composable (form: FormDraft, field: String) -> Unit) {
    val form = ctx.form
    when (InputRules.state(c, form, ctx.runtime.currentUid())) {
        "hidden" -> return
        "needsForm" -> return NeedsForm(c, modifier)
    }
    form!!
    val override = InputRules.override(c, form)
    Column(modifier.fillMaxWidth().semantics { testTag = "field.${c.field}" }) {
        if (override != null) FieldOverrideRow(c, override, form, ctx) else content(form, c.field!!)
        if (form.isFlagged(c.field!!)) {
            Text(
                stringResource(R.string.inputs_required),
                style = MaterialTheme.typography.labelSmall,
                color = MaterialTheme.colorScheme.error,
                modifier = Modifier.padding(start = 4.dp, top = 4.dp),
            )
        }
    }
}

/** フォームの外(screen.entity 無し)に置かれた入力。 */
@Composable
fun NeedsForm(c: Component, modifier: Modifier = Modifier) {
    Row(modifier.fillMaxWidth().inputChrome(), verticalAlignment = Alignment.CenterVertically) {
        Text(
            stringResource(R.string.inputs_needs_form, c.type.wire),
            style = MaterialTheme.typography.labelSmall,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
        )
    }
}

/** 入力の上の小さなラベル + 中身(iOS の `LabeledInput`)。 */
@Composable
fun LabeledInput(label: String?, modifier: Modifier = Modifier, content: @Composable () -> Unit) {
    Column(modifier.fillMaxWidth(), verticalArrangement = Arrangement.spacedBy(6.dp)) {
        if (!label.isNullOrEmpty()) {
            Text(
                label,
                style = MaterialTheme.typography.labelMedium,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
                modifier = Modifier.padding(start = 2.dp),
            )
        }
        content()
    }
}

/** コントロール型の入力(toggle / datePicker / timePicker / colorPicker / stepper)の 1 行: ラベル … 操作(iOS の `InputRow`)。 */
@Composable
fun InputRow(label: String, modifier: Modifier = Modifier, trailing: @Composable RowScope.() -> Unit) {
    Row(
        modifier.fillMaxWidth().inputChrome(),
        verticalAlignment = Alignment.CenterVertically,
        horizontalArrangement = Arrangement.spacedBy(RenderTheme.Spacing.sm),
    ) {
        Text(label, style = MaterialTheme.typography.bodyLarge, modifier = Modifier.weight(1f))
        trailing()
    }
}

/** 入力の器(角丸の細い枠)。 */
@Composable
fun Modifier.inputChrome(color: Color = RenderTheme.colors.hairline): Modifier =
    this
        .defaultMinSize(minHeight = 48.dp)
        .border(1.dp, color, RoundedCornerShape(RenderTheme.Radius.control))
        .padding(horizontal = RenderTheme.Spacing.inputH, vertical = RenderTheme.Spacing.inputV / 2)

/** 選べない理由を器の中に出す(参照先が無い・参加者を読めない)。 */
@Composable
fun InputNotice(text: String) {
    Row(Modifier.fillMaxWidth().inputChrome(), verticalAlignment = Alignment.CenterVertically) {
        Text(text, style = MaterialTheme.typography.bodyMedium, color = MaterialTheme.colorScheme.onSurfaceVariant)
    }
}
