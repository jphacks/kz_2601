package jp.kz2601.metamon.renderer

import jp.kz2601.metamon.dsl.AppDSL
import jp.kz2601.metamon.dsl.Component
import jp.kz2601.metamon.dsl.ComponentType
import jp.kz2601.metamon.dsl.decode
import jp.kz2601.metamon.dsl.records.JSONValue
import jp.kz2601.metamon.dsl.records.entityModels
import jp.kz2601.metamon.renderer.components.inputs.InputRules
import jp.kz2601.metamon.renderer.components.location.LocationButtonRules
import jp.kz2601.metamon.renderer.conformance.InputNodes
import jp.kz2601.metamon.renderer.form.FormDraft
import jp.kz2601.metamon.renderer.registry.ComponentRegistry
import jp.kz2601.metamon.renderer.registry.DeviceCapabilities
import kotlinx.serialization.json.JsonNull
import kotlinx.serialization.json.JsonPrimitive
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test
import java.util.Locale

/** locationButton(#867): 保存する形・提供元の順・住所の field・フォームとの結び付き(iOS の LocationButtonLeaf)。 */
class LocationButtonTest {
    private val dsl = AppDSL.decode(
        """
        {"schemaVersion":1,"app":{"name":"t","voiceEntry":"none"},"initialScreen":"s",
         "entities":[{"id":"spot","name":"場所","fields":[
           {"id":"place","name":"位置","type":"location"},{"id":"addr","name":"住所","type":"string"}]}],
         "screens":[{"id":"s","title":"記録","entity":"spot","components":[
           {"type":"locationButton","field":"place","label":"現在地を記録","addressField":"addr"},
           {"type":"locationButton","field":"place","addressField":"place"},
           {"type":"locationButton","field":"place","addressField":""}]}]}
        """.trimIndent(),
    )
    private val entity = dsl.entityModels().first()
    private val buttons: List<Component> = dsl.screens.first().components

    @Test
    fun registeredAndDeclared() {
        assertTrue(ComponentType.LOCATION_BUTTON.wire in ComponentRegistry.standard.registeredTypes)
        assertTrue(ComponentType.LOCATION_BUTTON.wire in DeviceCapabilities.of().allowedComponents)
        assertTrue(ComponentType.LOCATION_BUTTON in InputRules.formBound)
        assertFalse(ComponentType.LOCATION_BUTTON in InputRules.overridable)
        // 地図の提供元が決まるまで描かない
        for (t in listOf(ComponentType.LOCATION_PICKER, ComponentType.PLACE_SEARCH, ComponentType.MAP_VIEW)) {
            assertFalse(t.wire, t.wire in ComponentRegistry.standard.registeredTypes)
        }
    }

    @Test
    fun coordinateIsSixDecimalsWithDotEvenInOtherLocales() {
        assertEquals("35.681236,139.767125", LocationButtonRules.coordinate(35.6812362, 139.7671248))
        assertEquals("-33.868800,151.209300", LocationButtonRules.coordinate(-33.8688, 151.2093))
        val saved = Locale.getDefault()
        try {
            Locale.setDefault(Locale.GERMANY)
            assertEquals("36.561300,136.656200", LocationButtonRules.coordinate(36.5613, 136.6562))
        } finally {
            Locale.setDefault(saved)
        }
    }

    @Test
    fun providerOrderFollowsPermissionLevel() {
        val all = listOf("passive", "gps", "network", "fused")
        assertEquals(listOf("fused", "network", "gps"), LocationButtonRules.providers(all, fineGranted = true))
        assertEquals("おおよその許可だけなら gps は使わない", listOf("fused", "network"), LocationButtonRules.providers(all, fineGranted = false))
        assertEquals(listOf("gps"), LocationButtonRules.providers(listOf("gps", "passive"), fineGranted = true))
        assertEquals(emptyList<String>(), LocationButtonRules.providers(listOf("gps", "passive"), fineGranted = false))
    }

    @Test
    fun addressTargetSkipsEmptyAndSameField() {
        assertEquals("addr", LocationButtonRules.addressTarget(buttons[0]))
        assertNull(LocationButtonRules.addressTarget(buttons[1]))
        assertNull(LocationButtonRules.addressTarget(buttons[2]))
    }

    @Test
    fun lastKnownOnlyWhenRecent() {
        assertTrue(LocationButtonRules.freshEnough(0))
        assertTrue(LocationButtonRules.freshEnough(LocationButtonRules.MAX_LAST_KNOWN_AGE_MS))
        assertFalse(LocationButtonRules.freshEnough(LocationButtonRules.MAX_LAST_KNOWN_AGE_MS + 1))
        assertFalse(LocationButtonRules.freshEnough(-1))
    }

    @Test
    fun previewOnlyForNonEmptyString() {
        assertEquals("35.0,135.0", LocationButtonRules.preview(JSONValue.Str("35.0,135.0")))
        assertNull(LocationButtonRules.preview(JSONValue.Str("")))
        assertNull(LocationButtonRules.preview(JSONValue.Null))
        assertNull(LocationButtonRules.preview(null))
    }

    @Test
    fun treeNodeCarriesFieldLabelAndValue() {
        val runtime = NoopDSLRuntime
        val form = FormDraft(entity)
        val p = InputNodes.props(buttons[0], form, runtime)
        assertEquals(JsonPrimitive("place"), p["field"])
        assertEquals(JsonPrimitive("現在地を記録"), p["label"])
        assertEquals(JsonNull, p["value"])
        form.values["place"] = JSONValue.Str(LocationButtonRules.coordinate(35.0, 135.0))
        assertEquals(JsonPrimitive("35.000000,135.000000"), InputNodes.props(buttons[0], form, runtime)["value"])
        assertEquals("フォームの外では needsForm", "needsForm", InputRules.state(buttons[0], null, null))
    }
}
