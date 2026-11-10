package jp.kz2601.metamon.services.discover

import jp.kz2601.metamon.services.http.errorBody
import jp.kz2601.metamon.services.http.json
import jp.kz2601.metamon.services.http.metamonHttp
import jp.kz2601.metamon.services.http.take
import jp.kz2601.metamon.services.live.HttpDiscoverClient
import jp.kz2601.metamon.services.live.HttpModerationClient
import jp.kz2601.metamon.services.social.ReportReason
import kotlinx.coroutines.runBlocking
import okhttp3.mockwebserver.MockWebServer
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Before
import org.junit.Test

/** 「探す」の検索・カテゴリ・インストール・通報 / ブロック(#902)。口は docs/backend/openapi-appconfig.yaml。 */
class DiscoverModelTest {
    private lateinit var server: MockWebServer
    private lateinit var model: DiscoverModel

    @Before
    fun setUp() {
        server = MockWebServer().apply { start() }
        val http = server.metamonHttp()
        model = DiscoverModel(HttpDiscoverClient(http), HttpModerationClient(http))
    }

    @After
    fun tearDown() {
        server.shutdown()
    }

    @Test
    fun searchSendsTheQueryInTheBodyNotTheUrl() = runBlocking {
        server.enqueue(json(200, """{"items":[$KAKEIBO,$KINTORE],"nextPageToken":"p2"}"""))
        model.setQuery(" 家計簿 ")
        model.setSort(DiscoverSort.NEW)
        model.search()

        val req = server.take()
        assertEquals("POST", req.method)
        assertEquals("/v1/public-apps/search", req.path) // 検索語を URL に載せない
        assertEquals("""{"q":"家計簿","sort":"new","pageSize":50}""", req.body.readUtf8())
        val s = model.state.value
        assertFalse(s.loading)
        assertNull(s.failure)
        assertEquals(listOf("a1", "a2"), s.items.map { it.id })
        assertEquals(12, s.items[0].installs)
        assertEquals(listOf("stat", "list"), s.items[0].components)
    }

    @Test
    fun emptyQueryLeavesOutQAndDefaultsToPopular() = runBlocking {
        server.enqueue(json(200, """{"items":[]}"""))
        model.search()
        assertEquals("""{"sort":"popular","pageSize":50}""", server.take().body.readUtf8())
        assertTrue(model.state.value.items.isEmpty())
    }

    @Test
    fun categoriesComeFromTagsAndFilterTheList() = runBlocking {
        server.enqueue(json(200, """{"items":[$KAKEIBO,$KINTORE,$SANPO]}"""))
        model.search()
        // 健康 が 2 件で先頭・同数は先に出た順
        assertEquals(listOf("健康", "お金", "運動", "散歩"), model.state.value.categories)

        model.toggleCategory("健康")
        assertEquals(listOf("a2", "a3"), model.state.value.visibleItems.map { it.id })
        model.toggleCategory("健康") // もう一度で外す
        assertEquals(3, model.state.value.visibleItems.size)
    }

    @Test
    fun aCategoryMissingFromNewResultsIsCleared() = runBlocking {
        server.enqueue(json(200, """{"items":[$KAKEIBO,$KINTORE]}"""))
        server.enqueue(json(200, """{"items":[$KAKEIBO]}"""))
        model.search()
        model.toggleCategory("運動")
        model.setQuery("家計")
        model.search()
        assertNull(model.state.value.category)
    }

    @Test
    fun searchFailureKeepsWhatWasLoaded() = runBlocking {
        server.enqueue(json(200, """{"items":[$KAKEIBO]}"""))
        server.enqueue(json(400, errorBody("bad_request")))
        model.search()
        model.search()
        assertEquals(DiscoverFailure.OTHER, model.state.value.failure)
        assertEquals(listOf("a1"), model.state.value.items.map { it.id })
    }

    @Test
    fun withoutAClientDiscoverIsUnavailable() = runBlocking {
        val off = DiscoverModel(null)
        off.search()
        assertEquals(DiscoverFailure.UNAVAILABLE, off.state.value.failure)
        assertFalse(off.state.value.loading)
        assertEquals(DiscoverInstallResult.Failed(DiscoverFailure.UNAVAILABLE), off.install(PublicApp("a1")))
        assertFalse(off.canModerate)
    }

    @Test
    fun installPostsToTheListingAndReturnsTheCopy() = runBlocking {
        server.enqueue(json(201, """{"id":"new-1","name":"家計簿","currentVersion":3,"dsl":{},"created":true,"joined":false}"""))
        val result = model.install(PublicApp("a 1", name = "家計簿"))

        val req = server.take()
        assertEquals("POST", req.method)
        assertEquals("/v1/public-apps/a%201/install", req.path)
        result as DiscoverInstallResult.Installed
        assertEquals("new-1", result.app.id)
        assertTrue(result.app.created)
        assertNull(model.state.value.installing)
    }

    @Test
    fun installOfAnUnpublishedOrMissingListingSaysWhy() = runBlocking {
        server.enqueue(json(403, errorBody("unpublished")))
        server.enqueue(json(404, errorBody("not_found")))
        assertEquals(DiscoverInstallResult.Failed(DiscoverFailure.UNPUBLISHED), model.install(PublicApp("a1")))
        assertEquals(DiscoverInstallResult.Failed(DiscoverFailure.NOT_FOUND), model.install(PublicApp("a1")))
    }

    @Test
    fun reportSendsTheListingAndTheReason() = runBlocking {
        server.enqueue(json(200, """{"id":"r1","created":true}"""))
        assertEquals(DiscoverModerationResult.REPORTED, model.report(PublicApp("a1"), ReportReason.SPAM))
        val req = server.take()
        assertEquals("/v1/reports", req.path)
        assertEquals("""{"kind":"listing","id":"a1","reason":"spam"}""", req.body.readUtf8())
    }

    @Test
    fun reportOverTheDailyLimitSaysSo() = runBlocking {
        // 429 は 1 回だけ再送される
        server.enqueue(json(429, errorBody("rate_limited"), "Retry-After" to "1"))
        server.enqueue(json(429, errorBody("rate_limited"), "Retry-After" to "1"))
        assertEquals(DiscoverModerationResult.REPORT_LIMITED, model.report(PublicApp("a1"), ReportReason.SPAM))
    }

    @Test
    fun blockingThePublisherHidesTheListingAndReloads() = runBlocking {
        server.enqueue(json(200, """{"items":[$KAKEIBO,$KINTORE]}"""))
        server.enqueue(json(200, """{"uid":"u9","name":"たろう","createdAt":"2026-11-11T00:00:00Z"}"""))
        server.enqueue(json(200, """{"items":[$KINTORE]}"""))
        model.search()
        server.take()

        assertEquals(DiscoverModerationResult.BLOCKED, model.blockPublisher(model.state.value.items[0]))
        val block = server.take()
        assertEquals("/v1/me/blocks", block.path)
        assertEquals("""{"listingId":"a1"}""", block.body.readUtf8())
        assertEquals("/v1/public-apps/search", server.take().path)
        assertEquals(listOf("a2"), model.state.value.items.map { it.id })
    }

    @Test
    fun aFailedBlockPutsTheListingBack() = runBlocking {
        server.enqueue(json(200, """{"items":[$KAKEIBO,$KINTORE]}"""))
        server.enqueue(json(500, errorBody("internal")))
        model.search()
        assertEquals(DiscoverModerationResult.BLOCK_FAILED, model.blockPublisher(model.state.value.items[0]))
        assertEquals(listOf("a1", "a2"), model.state.value.items.map { it.id })
    }

    private companion object {
        const val KAKEIBO =
            """{"id":"a1","name":"家計簿","description":"毎日の出費","tags":["お金"],"authorName":"たろう","components":["stat","list"],"installs":12,"mine":false}"""
        const val KINTORE = """{"id":"a2","name":"筋トレ","tags":["健康","運動"],"installs":3}"""
        const val SANPO = """{"id":"a3","name":"散歩","tags":["#健康","散歩"],"installs":1}"""
    }
}
