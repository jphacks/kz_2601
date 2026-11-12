package jp.kz2601.metamon.release

import jp.kz2601.metamon.services.release.MockReleaseRequestClient
import jp.kz2601.metamon.services.release.ReleaseRequest
import jp.kz2601.metamon.services.release.ReleaseRequestClient
import jp.kz2601.metamon.services.release.ReleaseStatus
import kotlinx.coroutines.runBlocking
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test
import java.io.IOException

/** リリース申請の状態(#1254): 送った申請がカードのバッジに出る・見送りの後はまた送れる。 */
class ReleaseRequestsModelTest {
    @Test
    fun sentRequestShowsOnTheCard() = runBlocking {
        val model = ReleaseRequestsModel(MockReleaseRequestClient())
        assertNull(model.send("a", "   ", ""))
        val r = model.send("a", " 毎朝の記録 ", " ")!!
        assertEquals("毎朝の記録", r.purpose)
        assertEquals(ReleaseStatus.RECEIVED, model.byApp.value["a"]?.kind)
        assertEquals(r, model.open("a"))
        model.reset()
        assertTrue(model.byApp.value.isEmpty())
        model.refresh()
        assertEquals(r, model.byApp.value["a"])
    }

    @Test
    fun declinedIsNotOpen() = runBlocking {
        val client = object : ReleaseRequestClient {
            override suspend fun request(appId: String, purpose: String, contact: String?) = error("unused")
            override suspend fun myRequests() = listOf(ReleaseRequest("1", "a", status = "declined"))
        }
        val model = ReleaseRequestsModel(client)
        model.refresh()
        assertEquals(ReleaseStatus.DECLINED, model.byApp.value["a"]?.kind)
        assertNull("見送りの後は新しく申請できる", model.open("a"))
    }

    @Test
    fun failuresKeepThePreviousState() = runBlocking {
        val client = object : ReleaseRequestClient {
            override suspend fun request(appId: String, purpose: String, contact: String?): ReleaseRequest = throw IOException("offline")
            override suspend fun myRequests(): List<ReleaseRequest> = throw IOException("offline")
        }
        val model = ReleaseRequestsModel(client)
        model.refresh()
        assertNull(model.send("a", "p", ""))
        assertTrue(model.byApp.value.isEmpty())
    }
}
