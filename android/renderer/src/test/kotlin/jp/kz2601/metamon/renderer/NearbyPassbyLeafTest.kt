package jp.kz2601.metamon.renderer

import jp.kz2601.metamon.dsl.AppDSL
import jp.kz2601.metamon.dsl.decode
import jp.kz2601.metamon.dsl.nearby.NearbyLimits
import jp.kz2601.metamon.dsl.nearby.NearbyPassby
import jp.kz2601.metamon.dsl.records.entityModels
import jp.kz2601.metamon.renderer.components.nearby.NearbyHost
import jp.kz2601.metamon.renderer.components.nearby.PassbyCopy
import jp.kz2601.metamon.renderer.components.nearby.PassbyState
import jp.kz2601.metamon.renderer.components.nearby.StaticNearbyPassbyController
import jp.kz2601.metamon.dsl.records.JSONValue
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

/** すれ違い通信(#1383・iOS の NearbyPassbyRenderTests): ホストの既定の振る舞いと、画面の文の組み立て。 */
class NearbyPassbyLeafTest {
    private val dsl = AppDSL.decode(
        """
        {"schemaVersion":1,"app":{"name":"すれ違い","voiceEntry":"none"},"initialScreen":"home",
         "entities":[{"id":"profile","name":"プロフィール","fields":[{"id":"name","name":"名前","type":"string"},{"id":"hobby","name":"趣味","type":"string"},
           {"id":"pic","name":"写真","type":"photo"},{"id":"memo","name":"メモ","type":"string","hidden":true}]}],
         "screens":[{"id":"home","title":"すれ違い","components":[
           {"type":"nearbyShare","id":"pb","label":"すれ違い通信","entity":"profile","mode":"passby","fields":["name","hobby","pic","memo"],"distance":true}]}]}
        """.trimIndent(),
    )
    private val config = NearbyPassby.Config.of(dsl.screens[0].components[0])!!

    @Test
    fun staticControllerNeedsOptInAndAttach() {
        val host = StaticNearbyPassbyController()
        host.attach(config)
        assertFalse("オンにするまで探さない", host.state.value.isAdvertising)
        host.setOptIn(true, config)
        assertTrue(host.state.value.isAdvertising)
        host.detach(NearbyPassby.Config(componentId = "other", entityId = "profile", label = "", fields = emptyList()))
        assertTrue("別の部品の detach では止めない", host.state.value.isAdvertising)
        host.detach(config)
        assertFalse(host.state.value.isAdvertising)
        assertNull(host.attached)
    }

    @Test
    fun agreeSetsConsentVersionAndAnonymous() {
        val host = StaticNearbyPassbyController()
        assertFalse(host.state.value.hasConsent)
        host.agree(anonymous = true)
        assertTrue(host.state.value.hasConsent)
        assertEquals(NearbyLimits.ANONYMOUS_NAME, host.state.value.publicName)
        host.setAnonymous(false)
        assertEquals("Tailor It", host.state.value.publicName)
        assertFalse("前の版の同意では足りない", PassbyState(consentedVersion = 1).hasConsent)
    }

    @Test
    fun fieldNamesDropHiddenAndMedia() {
        val entity = dsl.entityModels().first()
        assertEquals(listOf("名前", "趣味"), PassbyCopy.fieldNames(config, entity))
        assertEquals(emptyList<String>(), PassbyCopy.fieldNames(config, null))
        assertEquals("「名前」「趣味」", PassbyCopy.quoted(listOf("名前", "趣味")))
    }

    @Test
    fun consentTextMatchesDslInJapanese() {
        val names = listOf("名前", "趣味")
        val built = PassbyCopy.consent(
            intro = "オンの間、近くで同じアプリを開いている人と自動でつながり、次のものを渡します。",
            nameLine = "・表示名: 太郎",
            fieldsLine = "・項目: ${PassbyCopy.quoted(names)}",
            records = "相手の記録はこの端末に追加されます。写真・動画や、それ以外の項目は送りません。",
            stops = "この画面を閉じるか、アプリを裏に回すと止まります。",
        )
        assertEquals(NearbyPassby.consentText(names, "太郎"), built)
    }

    @Test
    fun hostWithoutPassbyIsDisplayOnly() {
        val host = object : NearbyHost {
            override fun shareNearby(entity: String, records: List<Map<String, JSONValue>>) = Unit
        }
        assertNull("既定は通信しない(表示だけ)", host.passby)
    }
}
