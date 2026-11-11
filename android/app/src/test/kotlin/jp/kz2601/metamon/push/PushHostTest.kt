package jp.kz2601.metamon.push

import jp.kz2601.metamon.dsl.AppDSL
import jp.kz2601.metamon.dsl.DslJson
import jp.kz2601.metamon.services.push.InMemoryPushPrefs
import jp.kz2601.metamon.services.push.NotifyConsentStore
import jp.kz2601.metamon.services.push.NotifyConsentToggle
import jp.kz2601.metamon.services.push.PushDeviceClient
import jp.kz2601.metamon.services.push.PushDeviceRegistrar
import jp.kz2601.metamon.services.push.PushDeviceRegistration
import jp.kz2601.metamon.services.sharing.AppMembersStore
import jp.kz2601.metamon.services.sharing.MembersClient
import jp.kz2601.metamon.services.sharing.ROLE_MEMBER
import jp.kz2601.metamon.services.sharing.ROLE_OWNER
import jp.kz2601.metamon.services.sharing.SharedMember
import jp.kz2601.metamon.services.sharing.SharedMemberList
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.test.runTest
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

class PushHostTest {
    private val calls = mutableListOf<String>()
    private val client = object : PushDeviceClient {
        override suspend fun putDevice(installationId: String, registration: PushDeviceRegistration) { calls += "put ${registration.token}" }
        override suspend fun deleteDevice(installationId: String) { calls += "delete" }
        override suspend fun setNotifyConsent(appId: String, allowed: Boolean) { calls += "consent $appId $allowed" }
    }
    private val prefs = InMemoryPushPrefs()
    private var uid: String? = "uid-1"
    private var canPost = true
    private var token: String? = "fcm-a"

    private fun host(registerDevices: Boolean = true): PushHost {
        val store = NotifyConsentStore(prefs)
        return PushHost(
            registrar = PushDeviceRegistrar(client, prefs, appVersion = "0.1.0", os = "Android 15", newId = { "install-1" }),
            consentStore = store,
            consent = NotifyConsentToggle(client, store),
            uid = { uid },
            canPost = { canPost },
            fetchToken = { token },
            registerDevices = registerDevices,
        )
    }

    @Test
    fun refreshRegistersOnlyWhenSignedInAndAllowed() = runTest {
        val h = host()
        uid = null
        h.refresh()
        assertTrue(calls.isEmpty())
        uid = "uid-1"
        h.refresh()
        h.refresh()
        assertEquals(listOf("put fcm-a"), calls)
    }

    @Test
    fun refreshAfterNotificationsWereTurnedOffDeletesTheRegistration() = runTest {
        val h = host()
        h.refresh()
        canPost = false
        h.refresh()
        assertEquals(listOf("put fcm-a", "delete"), calls)
    }

    @Test
    fun newTokenIsSentAndMissingTokenIsIgnored() = runTest {
        val h = host()
        token = null
        h.refresh()
        assertTrue(calls.isEmpty())
        h.receive("fcm-b")
        assertEquals(listOf("put fcm-b"), calls)
    }

    @Test
    fun nothingIsSentWhileTheBackendDoesNotAcceptAndroid() = runTest {
        val h = host(registerDevices = false)
        h.refresh()
        h.receive("fcm-b")
        assertTrue(calls.isEmpty())
    }

    @Test
    fun signOutDeletesTheRegistration() = runTest {
        val h = host()
        h.refresh()
        h.unregister()
        assertEquals(listOf("put fcm-a", "delete"), calls)
    }

    @Test
    fun openOnlyWhenSignedIn() = runTest {
        val opened = mutableListOf<String>()
        val h = host()
        uid = null
        h.open("app-1") { opened += it }
        uid = "uid-1"
        h.open("app-1") { opened += it }
        assertEquals(listOf("app-1"), opened)
    }

    @Test
    fun consentIsReadPerAccount() = runTest {
        val h = host()
        assertFalse(h.consentAllowed("app-1"))
        h.consent.change(true, h.currentUid(), "app-1") {}
        assertTrue(h.consentAllowed("app-1"))
        uid = "uid-2"
        assertFalse(h.consentAllowed("app-1"))
    }

    @Test
    fun consentIsOfferedOnlyForSharedOrPublicRecords() {
        assertFalse(PushHost.canSetConsent(dsl(null)))
        assertFalse(PushHost.canSetConsent(dsl("private")))
        assertTrue(PushHost.canSetConsent(dsl("shared")))
        assertTrue(PushHost.canSetConsent(dsl("public")))
    }

    private fun dsl(scope: String?) = DslJson.decodeFromString(
        AppDSL.serializer(),
        """{"schemaVersion":1,"app":{"name":"x"},"initialScreen":"home","screens":[{"id":"home","title":"h","components":[]}],
           "entities":[{"id":"e","name":"e"${scope?.let { ",\"scope\":\"$it\"" } ?: ""},"fields":[]}]}""",
    )

    /** 参加者の口。[list] が null なら読めない(通信の失敗) */
    private class FakeMembers(var list: SharedMemberList?) : MembersClient {
        var calls = 0
        override suspend fun members(appId: String): SharedMemberList {
            calls++
            return list ?: throw IllegalStateException("offline")
        }
        override suspend fun setAdmin(appId: String, uid: String, admin: Boolean) = Unit
        override suspend fun removeMember(appId: String, uid: String) = Unit
        override suspend fun leave(appId: String) = Unit
    }

    private suspend fun offered(list: SharedMemberList?, scope: String? = "shared"): Boolean {
        val client = FakeMembers(list)
        val store = AppMembersStore(client, CoroutineScope(Dispatchers.Unconfined))
        return PushHost.canSetConsent(dsl(scope), "app-1", store)
    }

    @Test
    fun consentIsHiddenForOwnerOnly() = runTest {
        val owner = SharedMember(uid = "o", role = ROLE_OWNER, you = true)
        // 作成者には同意なしで届くので出さない
        assertFalse(offered(SharedMemberList(listOf(owner), role = ROLE_OWNER)))
        // 管理者・参加者には出す
        val admin = SharedMember(uid = "a", role = ROLE_MEMBER, admin = true, you = true)
        assertTrue(offered(SharedMemberList(listOf(SharedMember(uid = "o", role = ROLE_OWNER), admin), role = ROLE_MEMBER, admin = true)))
        val member = SharedMember(uid = "m", role = ROLE_MEMBER, you = true)
        assertTrue(offered(SharedMemberList(listOf(SharedMember(uid = "o", role = ROLE_OWNER), member), role = ROLE_MEMBER)))
        // 役割が読めない時は出す(iOS と同じ)
        assertTrue(offered(null))
        // 自分だけのアプリは作成者でなくても出さない(参加者は読まない)
        assertFalse(offered(SharedMemberList(role = ROLE_MEMBER), scope = "private"))
    }

    @Test
    fun consentWithoutMembersClientIsOffered() = runTest {
        assertTrue(PushHost.canSetConsent(dsl("public"), "app-1", null))
        assertTrue(PushHost.canSetConsent(dsl("public"), "app-1", AppMembersStore(null, CoroutineScope(Dispatchers.Unconfined))))
        // 読んだ役割がそのまま入る
        val store = AppMembersStore(FakeMembers(SharedMemberList(role = ROLE_OWNER)), CoroutineScope(Dispatchers.Unconfined))
        store.reload("app-1")
        assertEquals(true, store["app-1"]?.isOwner)
        assertFalse(PushHost.canSetConsent(dsl("shared"), store["app-1"]))
    }

    @Test
    fun pendingOpenIsConsumedOnce() {
        PushOpen.consume()
        PushOpen.offer(null)
        assertNull(PushOpen.pending.value)
        PushOpen.offer("app-1")
        assertEquals("app-1", PushOpen.consume())
        assertNull(PushOpen.consume())
    }
}
