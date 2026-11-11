package jp.kz2601.metamon.renderer

import jp.kz2601.metamon.dsl.ComponentType
import jp.kz2601.metamon.renderer.registry.ComponentRegistry
import jp.kz2601.metamon.renderer.registry.DeviceCapabilities
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Assume.assumeTrue
import org.junit.Test
import java.io.File

class DeviceCapabilitiesTest {
    private val caps = DeviceCapabilities.of()

    @Test
    fun allowedComponentsAreExactlyTheRegisteredTypes() {
        val pending = DeviceCapabilities.PENDING.map { it.wire }.toSet()
        assertEquals(ComponentRegistry.standard.registeredTypes - pending, caps.allowedComponents.toSet())
        assertEquals(caps.allowedComponents.size, caps.allowedComponents.toSet().size)
        assertFalse(ComponentType.UNKNOWN.wire in caps.allowedComponents)
    }

    @Test
    fun unregisteredPartsAreNotDeclared() {
        val missing = ComponentType.entries.filter { it != ComponentType.UNKNOWN && ComponentRegistry.standard.builder(it) == null }
        assertTrue("登録の無い部品がある前提(場所の検索など)", missing.isNotEmpty())
        missing.forEach { assertFalse(it.wire, it.wire in caps.allowedComponents) }
        // 今 Android で描けない部品(登録を足したらここから消す)
        for (t in listOf(
            ComponentType.PLACE_SEARCH,
            ComponentType.LOCATION_TRACKER,
            ComponentType.HEALTH_STAT,
            // 描けるが、すれ違い(#1383)がそろうまで申告しない
            ComponentType.NEARBY_SHARE,
        )) {
            assertFalse(t.wire, t.wire in caps.allowedComponents)
        }
        // この統合で登録した部品(#871・#872・#877・#879・#1354・地図 #867)は申告する
        for (t in listOf(
            ComponentType.LOCATION_PICKER,
            ComponentType.MAP_VIEW,
            ComponentType.SENSOR_VALUE,
            ComponentType.DASHCAM,
            ComponentType.MATCH_LIST,
            ComponentType.MUSIC_PLAYER,
            ComponentType.ROUTE_GUIDE,
            ComponentType.LOCATION_BUTTON,
        )) {
            assertTrue(t.wire, t.wire in caps.allowedComponents)
        }
    }

    @Test
    fun featuresFollowTheRegistry() {
        val bare = ComponentRegistry.build { register(ComponentType.VSTACK) { _, _, _ -> } }
        val s = DeviceCapabilities.supportedFeatures(bare)
        assertFalse("component.bracket" in s)
        assertFalse("aiButton.long" in s)
        assertFalse("aiChat.inputEntity" in s)
        assertTrue("singleEntity" in s) // 部品に依らない動作は残る
        assertEquals(listOf("vstack"), DeviceCapabilities.allowedComponents(bare))
    }

    @Test
    fun supportedFeaturesOfTheStandardBuild() {
        assertEquals(
            listOf(
                "recordRetention", "matching", "notification.when", "singleEntity", "addressedMessages", "notification.near", "aiButton.long", "aiButton.inputPhoto", "aiButton.saveTo",
                "action.lookupBook", "feed.v2", "expr.addMinutes", "aiChat.inputEntity", "component.bracket", "progressBar.settingTarget",
                "openUrlButton.app",
            ),
            caps.supportedFeatures,
        )
        assertEquals(listOf("fieldHealth"), caps.disabledFeatures)
    }

    /** 機能のキーは iOS の通常の版と同じ集合を見ている(送るかどうかは Android の実装で決める)。 */
    @Test
    fun featureKeysCoverTheIosList() {
        val docs = System.getProperty("metamon.dslDocsDir")
        assumeTrue(docs != null)
        val swift = File(docs, "../../ios/Packages/MetamonDSL/Sources/MetamonDSL/Model/ClientCapabilities.swift")
        assumeTrue(swift.isFile)
        val body = swift.readText().substringAfter("public static var supportedFeatures").substringAfter("#else").substringBefore("#endif")
        val ios = Regex("\"([^\"]+)\"").findAll(body).map { it.groupValues[1] }.toList()
        val known = DeviceCapabilities.features.map { it.key }
        val notCovered = ios - known.toSet()
        assertTrue("Android に無いキー: $notCovered(DeviceCapabilities.features に有無を足す)", notCovered.isEmpty())
        assertTrue(caps.supportedFeatures.all { it in ios })
    }
}
