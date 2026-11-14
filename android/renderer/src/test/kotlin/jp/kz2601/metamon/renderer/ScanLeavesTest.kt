package jp.kz2601.metamon.renderer

import jp.kz2601.metamon.dsl.AppDSL
import jp.kz2601.metamon.dsl.Component
import jp.kz2601.metamon.dsl.ComponentType
import jp.kz2601.metamon.dsl.decode
import jp.kz2601.metamon.dsl.records.JSONValue
import jp.kz2601.metamon.dsl.records.entityModels
import jp.kz2601.metamon.renderer.components.inputs.InputRules
import jp.kz2601.metamon.renderer.components.scan.QrScanOutcome
import jp.kz2601.metamon.renderer.components.scan.QrScanResult
import jp.kz2601.metamon.renderer.components.scan.QrScanRules
import jp.kz2601.metamon.renderer.conformance.InputNodes
import jp.kz2601.metamon.renderer.form.FormDraft
import jp.kz2601.metamon.renderer.registry.ComponentRegistry
import kotlinx.serialization.json.JsonNull
import kotlinx.serialization.json.JsonPrimitive
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

/** qrScanner(#866): 読んだ値の形・読み取り結果の扱い・フォームとの結び付き(iOS の QrScannerLeaf と RenderTree.formBound)。 */
class ScanLeavesTest {
    private val dsl = AppDSL.decode(
        """
        {"schemaVersion":1,"app":{"name":"t","voiceEntry":"none"},"initialScreen":"s",
         "entities":[{"id":"ticket","name":"チケット","fields":[{"id":"code","name":"コード","type":"string","required":true}]}],
         "screens":[{"id":"s","title":"読む","entity":"ticket","components":[{"type":"qrScanner","field":"code","label":"QRを読み取る"}]}]}
        """.trimIndent(),
    )
    private val entity = dsl.entityModels().first()
    private val scanner: Component = dsl.screens.first().components.first()

    @Test
    fun registered() {
        assertTrue(ComponentType.QR_SCANNER.wire in ComponentRegistry.standard.registeredTypes)
        assertTrue(ComponentType.QR_SCANNER in InputRules.formBound)
        // 型で描き替える入力ではない(iOS の FormFieldOverride.inputTypes に入っていない)
        assertTrue(ComponentType.QR_SCANNER !in InputRules.overridable)
    }

    @Test
    fun foundWritesRawStringAsIs() {
        val o = QrScanRules.outcome(QrScanResult.Found(" https://example.com/a?b=1 "))
        assertEquals(QrScanOutcome.Write(JSONValue.Str(" https://example.com/a?b=1 ")), o)
        // 共有リンクも加工せずに文字列で入れる(iOS と同じ)
        assertEquals(QrScanOutcome.Write(JSONValue.Str("tailorit://import/abc")), QrScanRules.outcome(QrScanResult.Found("tailorit://import/abc")))
    }

    @Test
    fun emptyOrBinaryCodeKeepsValue() {
        assertEquals(QrScanOutcome.Keep, QrScanRules.outcome(QrScanResult.Found("")))
        assertEquals(QrScanOutcome.Keep, QrScanRules.outcome(QrScanResult.Found(null)))
    }

    @Test
    fun canceledKeepsAndFailedFallsBackToManual() {
        assertEquals(QrScanOutcome.Keep, QrScanRules.outcome(QrScanResult.Canceled))
        assertEquals(QrScanOutcome.ManualOnly, QrScanRules.outcome(QrScanResult.Failed))
    }

    @Test
    fun scannedValueGoesToDraftAndPayload() {
        val form = FormDraft(entity)
        assertEquals(listOf("コード"), form.flagMissingRequired())
        val o = QrScanRules.outcome(QrScanResult.Found("ABC-123")) as QrScanOutcome.Write
        form.values["code"] = o.value
        assertEquals(JSONValue.Str("ABC-123"), form.payload["code"])
        assertTrue(form.flagMissingRequired().isEmpty())
        assertEquals("ABC-123", QrScanRules.preview(form.values["code"]))
    }

    @Test
    fun previewHidesEmpty() {
        assertNull(QrScanRules.preview(null))
        assertNull(QrScanRules.preview(JSONValue.Null))
        assertNull(QrScanRules.preview(JSONValue.Str("")))
    }

    @Test
    fun conformanceProps() {
        val form = FormDraft(entity)
        val empty = InputNodes.props(scanner, form, NoopDSLRuntime)
        assertEquals(JsonPrimitive("code"), empty["field"])
        assertEquals(JsonPrimitive("QRを読み取る"), empty["label"])
        assertEquals(JsonNull, empty["value"])
        form.values["code"] = JSONValue.Str("X")
        assertEquals(JsonPrimitive("X"), InputNodes.props(scanner, form, NoopDSLRuntime)["value"])
        // フォームの外では値を出さない
        assertTrue(InputNodes.props(scanner, null, NoopDSLRuntime).isEmpty())
        assertEquals("needsForm", InputRules.state(scanner, null, null))
    }
}
