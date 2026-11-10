package jp.kz2601.metamon.renderer.components.actions

import jp.kz2601.metamon.dsl.records.FieldCoding
import jp.kz2601.metamon.dsl.records.FieldType
import jp.kz2601.metamon.dsl.records.JSONValue
import jp.kz2601.metamon.renderer.form.FormDraft

/** `{{fieldId}}` をフォームの下書きの値で置き換える(shareButton の text・openUrlButton の to・qrCode の text)。 */
object FormTemplate {
    /**
     * 保存する項目(computed 以外)の `{{id}}` を表示用の文字にする(iOS の `displayText`)。フォームの外ではそのまま。
     * [raw] = 日時・数値を整形せず保存値のまま(qrCode)。
     */
    fun fill(text: String, form: FormDraft?, raw: Boolean = false): String {
        if (form == null || !text.contains("{{")) return text
        var s = text
        for (f in form.entity.storedFields) {
            val token = "{{${f.id}}}"
            if (!s.contains(token)) continue
            val v = form.values[f.id] ?: JSONValue.Null
            s = s.replace(token, if (raw) v.displayString else FieldCoding.display(v, f))
        }
        return s
    }

    /** shareButton の文面: text を埋め、`field` が写真・動画・音声以外なら値を次の行に足す(iOS の `ShareButtonLeaf.rawText`)。 */
    fun shareText(text: String?, field: String?, form: FormDraft?): String {
        var s = fill(text.orEmpty(), form)
        val f = field?.let { form?.entity?.field(it) } ?: return s
        if (f.type == FieldType.PHOTO || f.type == FieldType.VIDEO || f.type == FieldType.AUDIO) return s
        val v = FieldCoding.display(form!!.values[f.id] ?: JSONValue.Null, f)
        if (v.isNotEmpty()) s += (if (s.isEmpty()) "" else "\n") + v
        return s
    }
}
