package jp.kz2601.metamon.renderer.components.inputs

import androidx.compose.foundation.clickable
import androidx.compose.foundation.horizontalScroll
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.selection.selectable
import androidx.compose.material3.DropdownMenu
import androidx.compose.material3.DropdownMenuItem
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.FilterChip
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.RadioButton
import androidx.compose.material3.SegmentedButton
import androidx.compose.material3.SegmentedButtonDefaults
import androidx.compose.material3.SingleChoiceSegmentedButtonRow
import androidx.compose.material3.Switch
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.res.stringResource
import androidx.compose.ui.semantics.Role
import androidx.compose.ui.text.style.TextOverflow
import jp.kz2601.metamon.dsl.Component
import jp.kz2601.metamon.dsl.records.Field
import jp.kz2601.metamon.dsl.records.FieldType
import jp.kz2601.metamon.renderer.R
import jp.kz2601.metamon.renderer.form.FormChoices
import jp.kz2601.metamon.renderer.form.FormDraft
import jp.kz2601.metamon.renderer.form.PickerRules
import jp.kz2601.metamon.renderer.form.bool
import jp.kz2601.metamon.renderer.form.selectedIds
import jp.kz2601.metamon.renderer.form.setBool
import jp.kz2601.metamon.renderer.form.setText
import jp.kz2601.metamon.renderer.form.text
import jp.kz2601.metamon.renderer.form.toggleId
import jp.kz2601.metamon.renderer.registry.RenderContext
import jp.kz2601.metamon.renderer.theme.RenderTheme

// 選ぶ入力(toggle / picker)と、field の型で描き替える選択(ref / refs / member / members)・computed の表示。

/** `toggle`: boolean の On / Off。 */
@Composable
fun ToggleLeaf(c: Component, ctx: RenderContext, modifier: Modifier = Modifier) = FormInput(c, ctx, modifier) { form, field ->
    InputRow(InputRules.label(c, form).orEmpty()) {
        Switch(checked = form.bool(field), onCheckedChange = { form.setBool(field, it) })
    }
}

/** `picker`: enum の options から選ぶ。style(segmented / menu / wheel・省略時は 4 件以下なら segmented)。 */
@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun PickerLeaf(c: Component, ctx: RenderContext, modifier: Modifier = Modifier) = FormInput(c, ctx, modifier) { form, field ->
    val current = form.text(field)
    val options = PickerRules.options(form.entity.field(field)?.options.orEmpty(), current)
    val select = { v: String -> form.setText(field, v) }
    LabeledInput(c.label) {
        when (PickerRules.style(c, options.size)) {
            "segmented" -> SingleChoiceSegmentedButtonRow(Modifier.fillMaxWidth()) {
                options.forEachIndexed { i, o ->
                    SegmentedButton(
                        selected = o == current,
                        onClick = { select(o) },
                        shape = SegmentedButtonDefaults.itemShape(i, options.size),
                    ) { Text(o, maxLines = 1, overflow = TextOverflow.Ellipsis) }
                }
            }
            // wheel は Material に無いので、全部見える縦の一覧で選ぶ
            "wheel" -> Column(Modifier.fillMaxWidth().inputChrome()) {
                options.forEach { o ->
                    Row(
                        Modifier.fillMaxWidth().selectable(selected = o == current, role = Role.RadioButton) { select(o) },
                        verticalAlignment = Alignment.CenterVertically,
                    ) {
                        RadioButton(selected = o == current, onClick = null)
                        Text(o)
                    }
                }
            }
            else -> MenuSelect(c.label.orEmpty(), current, options.map { it to it }, select)
        }
    }
}

/** menu 型の選択: 器の中に選択中の値、押すと一覧(iOS の `MenuSelect`)。 */
@Composable
fun MenuSelect(label: String, selection: String, options: List<Pair<String, String>>, onSelect: (String) -> Unit) {
    var open by remember { mutableStateOf(false) }
    val title = options.firstOrNull { it.first == selection }?.second
    Box(Modifier.fillMaxWidth()) {
        Row(
            Modifier.fillMaxWidth().inputChrome().clickable(onClickLabel = label) { open = true },
            verticalAlignment = Alignment.CenterVertically,
        ) {
            Text(
                if (title == null || selection.isEmpty()) stringResource(R.string.inputs_select) else title,
                color = if (title == null || selection.isEmpty()) MaterialTheme.colorScheme.onSurfaceVariant else MaterialTheme.colorScheme.onSurface,
                maxLines = 1,
                overflow = TextOverflow.Ellipsis,
                modifier = Modifier.weight(1f),
            )
            Text("▾", color = MaterialTheme.colorScheme.primary)
        }
        DropdownMenu(expanded = open, onDismissRequest = { open = false }) {
            options.forEach { (value, t) ->
                DropdownMenuItem(text = { Text(t) }, onClick = {
                    onSelect(value)
                    open = false
                })
            }
        }
    }
}

/** 型で描き替える行(iOS の `FormFieldOverride.view`)。 */
@Composable
internal fun FieldOverrideRow(c: Component, field: Field, form: FormDraft, ctx: RenderContext) {
    val label = c.label ?: field.name
    when {
        field.isComputed -> LabeledInput(label) { InputNotice(FormChoices.computedText(field, form, ctx.runtime)) }
        field.type == FieldType.REF -> {
            val options = FormChoices.refOptions(field, ctx.runtime)
            LabeledInput(label) {
                if (options.isEmpty()) {
                    InputNotice(stringResource(R.string.inputs_add_ref_first, FormChoices.refEntityName(field, ctx.runtime, stringResource(R.string.inputs_ref_entity))))
                } else {
                    MenuSelect(label, form.text(field.id), listOf("" to stringResource(R.string.inputs_select)) + options.map { it.id to it.title }) {
                        form.setText(field.id, it)
                    }
                }
            }
        }
        field.type == FieldType.REFS -> {
            val options = FormChoices.refOptions(field, ctx.runtime)
            LabeledInput(label) {
                if (options.isEmpty()) {
                    InputNotice(stringResource(R.string.inputs_add_ref_first, FormChoices.refEntityName(field, ctx.runtime, stringResource(R.string.inputs_ref_entity))))
                } else {
                    val selected = form.selectedIds(field.id).toSet()
                    Column {
                        Chips(options.map { it.id to it.title }, selected) { form.toggleId(field.id, it, options.map { o -> o.id }) }
                        Text(
                            if (selected.isEmpty()) stringResource(R.string.inputs_split_all) else stringResource(R.string.inputs_split_count, selected.size),
                            style = MaterialTheme.typography.labelSmall,
                            color = MaterialTheme.colorScheme.onSurfaceVariant,
                        )
                    }
                }
            }
        }
        field.type == FieldType.MEMBER -> {
            val members = ctx.runtime.formMembers
            LabeledInput(label) {
                if (members.isEmpty()) {
                    InputNotice(stringResource(R.string.inputs_members_not_loaded))
                } else {
                    MenuSelect(label, form.text(field.id), FormChoices.memberOptions(form.text(field.id), members, stringResource(R.string.inputs_select))) { form.setText(field.id, it) }
                }
            }
        }
        else -> {
            // members: 宛先に使うので自分は並べない
            val me = ctx.runtime.currentUid()
            val members = ctx.runtime.formMembers.filter { it.uid != me }
            LabeledInput(label) {
                if (members.isEmpty()) {
                    InputNotice(stringResource(R.string.inputs_members_not_loaded))
                } else {
                    val selected = form.selectedIds(field.id).toSet()
                    Chips(members.map { it.uid to it.displayName.ifEmpty { it.uid } }, selected) {
                        form.toggleId(field.id, it, members.map { m -> m.uid })
                    }
                }
            }
        }
    }
}

/** 複数選択のチップ(横に流す)。 */
@Composable
private fun Chips(options: List<Pair<String, String>>, selected: Set<String>, onToggle: (String) -> Unit) {
    Row(Modifier.horizontalScroll(rememberScrollState()), horizontalArrangement = Arrangement.spacedBy(RenderTheme.Spacing.sm)) {
        options.forEach { (id, title) ->
            FilterChip(selected = id in selected, onClick = { onToggle(id) }, label = { Text(title) })
        }
    }
}
