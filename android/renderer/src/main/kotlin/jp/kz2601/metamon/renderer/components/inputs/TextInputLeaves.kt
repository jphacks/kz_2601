package jp.kz2601.metamon.renderer.components.inputs

import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Modifier
import androidx.compose.ui.text.input.ImeAction
import androidx.compose.ui.text.input.KeyboardType
import androidx.compose.ui.text.input.PasswordVisualTransformation
import androidx.compose.foundation.shape.RoundedCornerShape
import jp.kz2601.metamon.dsl.Component
import jp.kz2601.metamon.dsl.records.FieldCoding
import jp.kz2601.metamon.dsl.records.FieldType
import jp.kz2601.metamon.renderer.form.FormDraft
import jp.kz2601.metamon.renderer.form.TextInputRules
import jp.kz2601.metamon.renderer.form.numberText
import jp.kz2601.metamon.renderer.form.setNumberText
import jp.kz2601.metamon.renderer.form.setText
import jp.kz2601.metamon.renderer.form.text
import jp.kz2601.metamon.renderer.registry.RenderContext
import jp.kz2601.metamon.renderer.theme.RenderTheme

// 文字の入力(textField / textEditor / secureField / numberField)。値は FormDraft の field に書く。

private val fieldShape = RoundedCornerShape(RenderTheme.Radius.control)

/** `textField`: 1 行(return で次の欄へ)か、複数行(文字に合わせて伸びる・#196)。 */
@Composable
fun TextFieldLeaf(c: Component, ctx: RenderContext, modifier: Modifier = Modifier) = FormInput(c, ctx, modifier) { form, field ->
    val multiline = TextInputRules.isMultiline(c, form.entity.field(field), form.entity.titleField)
    LabeledInput(c.label) {
        OutlinedTextField(
            value = form.text(field),
            onValueChange = { form.setText(field, it) },
            modifier = Modifier.fillMaxWidth(),
            placeholder = c.placeholder?.let { { Text(it) } },
            textStyle = RenderTheme.Fonts.inputValue,
            singleLine = !multiline,
            minLines = if (multiline) TextInputRules.multilineLines.first else 1,
            maxLines = if (multiline) TextInputRules.multilineLines.last else 1,
            keyboardOptions = KeyboardOptions(imeAction = if (multiline) ImeAction.Default else ImeAction.Next),
            isError = form.isFlagged(field),
            shape = fieldShape,
        )
    }
}

/** `textEditor`: 複数行の自由記述。minLines(既定 4・最大 30)行から伸びる。 */
@Composable
fun TextEditorLeaf(c: Component, ctx: RenderContext, modifier: Modifier = Modifier) = FormInput(c, ctx, modifier) { form, field ->
    val lines = TextInputRules.editorLines(c.minLines)
    LabeledInput(c.label) {
        OutlinedTextField(
            value = form.text(field),
            onValueChange = { form.setText(field, it) },
            modifier = Modifier.fillMaxWidth(),
            placeholder = c.placeholder?.let { { Text(it) } },
            textStyle = RenderTheme.Fonts.inputValue,
            minLines = lines.first,
            maxLines = lines.last,
            isError = form.isFlagged(field),
            shape = fieldShape,
        )
    }
}

/** `secureField`: 伏せ字(PIN・パスワード)。 */
@Composable
fun SecureFieldLeaf(c: Component, ctx: RenderContext, modifier: Modifier = Modifier) = FormInput(c, ctx, modifier) { form, field ->
    LabeledInput(c.label) {
        OutlinedTextField(
            value = form.text(field),
            onValueChange = { form.setText(field, it) },
            modifier = Modifier.fillMaxWidth(),
            placeholder = c.placeholder?.let { { Text(it) } },
            textStyle = RenderTheme.Fonts.inputValue,
            singleLine = true,
            visualTransformation = PasswordVisualTransformation(),
            keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Password, imeAction = ImeAction.Next),
            isError = form.isFlagged(field),
            shape = fieldShape,
        )
    }
}

/** `numberField`: 数字キーボード。currency は前に ¥(unit が「円」等なら出さない・#150)、それ以外は後ろに unit。 */
@Composable
fun NumberFieldLeaf(c: Component, ctx: RenderContext, modifier: Modifier = Modifier) = FormInput(c, ctx, modifier) { form, field ->
    val def = form.entity.field(field)
    val type = def?.type ?: FieldType.NUMBER
    val isCurrency = type == FieldType.CURRENCY
    val unit = numberUnitText(c.unit ?: def?.unit, isCurrency)
    LabeledInput(c.label) {
        OutlinedTextField(
            value = form.numberText(field),
            onValueChange = { form.setNumberText(field, it) },
            modifier = Modifier.fillMaxWidth(),
            placeholder = { Text(c.placeholder ?: "0") },
            prefix = if (isCurrency) ({ Text("¥") }) else null,
            suffix = unit?.let { { Text(it) } },
            textStyle = RenderTheme.Fonts.inputNumber,
            singleLine = true,
            keyboardOptions = KeyboardOptions(
                keyboardType = if (type == FieldType.NUMBER) KeyboardType.Decimal else KeyboardType.Number,
                imeAction = ImeAction.Next,
            ),
            isError = form.isFlagged(field),
            shape = fieldShape,
        )
    }
}

/** 末尾の単位。currency は ¥ を前に出すので「円」等は落とし、「円/時」は「/時」だけ残す(#150)。 */
fun numberUnitText(unit: String?, isCurrency: Boolean): String? = if (isCurrency) FieldCoding.currencySuffix(unit) else unit

internal fun FormDraft.fieldName(field: String): String = entity.field(field)?.name ?: field
