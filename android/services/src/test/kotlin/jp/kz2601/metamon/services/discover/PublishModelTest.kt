package jp.kz2601.metamon.services.discover

import jp.kz2601.metamon.services.consent.InMemoryKeyValueStore
import jp.kz2601.metamon.services.http.errorBody
import jp.kz2601.metamon.services.http.json
import jp.kz2601.metamon.services.http.metamonHttp
import jp.kz2601.metamon.services.http.take
import jp.kz2601.metamon.services.live.HttpDiscoverClient
import jp.kz2601.metamon.services.social.AuthorProfile
import kotlinx.coroutines.runBlocking
import okhttp3.mockwebserver.MockResponse
import okhttp3.mockwebserver.MockWebServer
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Before
import org.junit.Test

/** 共有シートの「みんなに公開」: 自分の掲載の読み込み・公開の確認・公開・非公開(#902 / #1239)。 */
class PublishModelTest {
    private lateinit var server: MockWebServer
    private lateinit var profile: AuthorProfile
    private lateinit var model: PublishModel

    @Before
    fun setUp() {
        server = MockWebServer().apply { start() }
        profile = AuthorProfile(InMemoryKeyValueStore())
        model = PublishModel("app-1", HttpDiscoverClient(server.metamonHttp()), profile)
    }

    @After
    fun tearDown() {
        server.shutdown()
    }

    @Test
    fun loadFindsMyListingAcrossPages() = runBlocking {
        server.enqueue(json(200, """{"items":[{"id":"other","name":"x","installs":0}],"nextPageToken":"t2"}"""))
        server.enqueue(json(200, """{"items":[{"id":"app-1","name":"家計簿","installs":4,"mine":true}]}"""))
        model.load()

        assertEquals("""{"sort":"new","pageSize":50}""", server.take().body.readUtf8())
        assertEquals("""{"sort":"new","pageSize":50,"pageToken":"t2"}""", server.take().body.readUtf8())
        val s = model.state.value
        assertTrue(s.loaded)
        assertEquals(4, s.listing?.installs)
    }

    @Test
    fun someoneElsesListingOfTheSameIdIsNotMine() = runBlocking {
        server.enqueue(json(200, """{"items":[{"id":"app-1","name":"家計簿","installs":4,"mine":false}]}"""))
        model.load()
        assertNull(model.state.value.listing)
        assertTrue(model.state.value.loaded)
    }

    @Test
    fun publishAsksFirstWithTheSavedName() = runBlocking {
        profile.name = "たろう"
        model.requestPublish()
        assertEquals(PublishConfirm("たろう"), model.state.value.confirm)
        assertEquals(0, server.requestCount) // 確認するまで送らない

        model.cancelPublish()
        assertNull(model.state.value.confirm)
        assertEquals(0, server.requestCount)
    }

    @Test
    fun confirmingPublishesTheDescriptionTagsAndName() = runBlocking {
        server.enqueue(json(200, """{"id":"app-1","name":"家計簿","installs":0,"mine":true,"tags":["健康","習慣"]}"""))
        model.setDescription("  毎日の出費をつける  ")
        model.setTags("#健康　習慣、健康")
        model.requestPublish()
        model.confirmPublish(" はなこ ")

        val req = server.take()
        assertEquals("POST", req.method)
        assertEquals("/v1/apps/app-1/publish", req.path)
        assertEquals("""{"description":"毎日の出費をつける","tags":["健康","習慣"],"authorName":"はなこ"}""", req.body.readUtf8())
        val s = model.state.value
        assertNotNull(s.listing)
        assertNull(s.confirm)
        assertEquals(PublishNotice.PUBLISHED, s.notice)
        assertEquals("はなこ", profile.name) // 次からの初期値
    }

    @Test
    fun aBlankNameIsLeftOutSoTheServerSaysAnonymous() = runBlocking {
        server.enqueue(json(200, """{"id":"app-1","name":"家計簿","installs":0,"mine":true}"""))
        model.requestPublish()
        model.confirmPublish("  ")
        assertEquals("{}", server.take().body.readUtf8())
        assertNull(profile.name)
    }

    @Test
    fun textNotAllowedMarksTheField() = runBlocking {
        server.enqueue(json(422, errorBody("text_not_allowed", details = """{"field":"tags"}""")))
        model.setTags("だめ")
        model.requestPublish()
        model.confirmPublish("")
        val s = model.state.value
        assertEquals(PublishFailure.TEXT_NOT_ALLOWED, s.failure)
        assertEquals("tags", s.flaggedField)
        assertNull(s.listing)

        model.setTags("いい") // 直したら印を外す
        assertNull(model.state.value.flaggedField)
    }

    @Test
    fun onlyTheCreatorCanPublish() = runBlocking {
        server.enqueue(json(403, errorBody("forbidden")))
        server.enqueue(json(400, errorBody("bad_request")))
        model.requestPublish()
        model.confirmPublish("")
        assertEquals(PublishFailure.NOT_OWNER, model.state.value.failure)
        model.requestPublish()
        model.confirmPublish("")
        assertEquals(PublishFailure.BLANK_APP, model.state.value.failure)
    }

    @Test
    fun unpublishDeletesTheListing() = runBlocking {
        server.enqueue(json(200, """{"items":[{"id":"app-1","name":"家計簿","installs":4,"mine":true}]}"""))
        server.enqueue(MockResponse().setResponseCode(204))
        model.load()
        server.take()
        model.unpublish()

        val req = server.take()
        assertEquals("DELETE", req.method)
        assertEquals("/v1/apps/app-1/publish", req.path)
        assertNull(model.state.value.listing)
        assertEquals(PublishNotice.UNPUBLISHED, model.state.value.notice)
    }

    @Test
    fun aFailedUnpublishKeepsTheListing() = runBlocking {
        server.enqueue(json(200, """{"items":[{"id":"app-1","name":"家計簿","installs":4,"mine":true}]}"""))
        server.enqueue(json(403, errorBody("forbidden")))
        model.load()
        model.unpublish()
        assertNotNull(model.state.value.listing)
        assertEquals(PublishNotice.UNPUBLISH_FAILED, model.state.value.notice)
    }

    @Test
    fun withoutAClientNothingIsSent() = runBlocking {
        val off = PublishModel("app-1", null, profile)
        off.load()
        off.requestPublish()
        assertTrue(off.state.value.loaded)
        assertNull(off.state.value.confirm)
    }

    @Test
    fun splitTagsFollowsTheLimits() {
        assertEquals(listOf("a", "b"), PublishModel.splitTags(" a,b 、 a "))
        assertEquals(8, PublishModel.splitTags((1..10).joinToString(" ")).size)
        assertEquals(20, PublishModel.splitTags("x".repeat(30)).single().length)
    }
}
