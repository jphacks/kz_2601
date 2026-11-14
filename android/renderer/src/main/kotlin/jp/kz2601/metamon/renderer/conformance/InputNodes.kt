package jp.kz2601.metamon.renderer.conformance

import jp.kz2601.metamon.dsl.Component
import jp.kz2601.metamon.dsl.ComponentType
import jp.kz2601.metamon.dsl.records.JSONValue
import jp.kz2601.metamon.renderer.DSLRuntime
import jp.kz2601.metamon.renderer.components.inputs.InputRules
import jp.kz2601.metamon.renderer.form.FormChoices
import jp.kz2601.metamon.renderer.form.FormDraft
import jp.kz2601.metamon.renderer.form.PickerRules
import jp.kz2601.metamon.renderer.form.text
import kotlinx.serialization.json.JsonArray
import kotlinx.serialization.json.JsonElement
import kotlinx.serialization.json.JsonNull
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive

/** 描画の木のうち、入力部品とフォームの部分(iOS の RenderTree.swift の form / formBound / FormFieldOverride)。 */
object InputNodes {
    /** 画面の `form`: `{entity, recordId, readOnly, values}`(values は保存する形に正規化した値)。 */
    fun form(form: FormDraft): JsonObject = JsonObject(
        mapOf(
            "entity" to JsonPrimitive(form.entity.id),
            "recordId" to (form.recordId?.let(::JsonPrimitive) ?: JsonNull),
            "readOnly" to JsonPrimitive(form.isReadOnly),
            "values" to JsonObject(form.payload.mapValues { it.value.toJsonElement() }),
        ),
    )

    /** 入力部品の state。入力でなければ null(呼び出し側の規則で決める)。 */
    fun state(c: Component, form: FormDraft?, runtime: DSLRuntime): String? {
        if (form != null && form.hides(c, runtime.currentUid())) return "hidden"
        if (InputRules.override(c, form) != null) return "rendered"
        return null
    }

    /** 入力部品の props(DSL に書いた値 + 束縛中の値)。入力でなければ空。 */
    fun props(c: Component, form: FormDraft?, runtime: DSLRuntime): Map<String, JsonElement> {
        if (c.type !in InputRules.formBound || form == null) return emptyMap()
        val fid = c.field ?: return emptyMap()
        val p = LinkedHashMap(declared(c))
        val value = (form.values[fid] ?: JSONValue.Null).toJsonElement()
        val override = InputRules.override(c, form)
        if (override != null) {
            p["label"] = JsonPrimitive(c.label ?: override.name)
            if (override.isComputed) {
                p["input"] = JsonPrimitive("computed")
                p["value"] = JsonPrimitive(FormChoices.computedText(override, form, runtime))
            } else {
                p["input"] = JsonPrimitive(override.type.wire)
                p["value"] = value
                if (override.refEntity != null) {
                    p["options"] = JsonArray(
                        FormChoices.refOptions(override, runtime).map {
                            JsonObject(mapOf("id" to JsonPrimitive(it.id), "title" to JsonPrimitive(it.title)))
                        },
                    )
                }
            }
            return p
        }
        p["value"] = value
        p["label"] = InputRules.label(c, form)?.let(::JsonPrimitive) ?: JsonNull
        if (c.type == ComponentType.PICKER) {
            val options = PickerRules.options(form.entity.field(fid)?.options.orEmpty(), form.text(fid))
            p["options"] = JsonArray(options.map(::JsonPrimitive))
            p["style"] = JsonPrimitive(PickerRules.style(c, options.size))
        }
        return p
    }

    /** 入力部品が DSL に書く値(見た目のキーは入れない)。 */
    private fun declared(c: Component): Map<String, JsonElement> = buildMap {
        c.label?.let { put("label", JsonPrimitive(it)) }
        c.placeholder?.let { put("placeholder", JsonPrimitive(it)) }
        c.field?.let { put("field", JsonPrimitive(it)) }
        c.unit?.let { put("unit", JsonPrimitive(it)) }
        c.min?.let { put("min", JsonPrimitive(it)) }
        c.max?.let { put("max", JsonPrimitive(it)) }
        c.step?.let { put("step", JsonPrimitive(it)) }
    }
}
