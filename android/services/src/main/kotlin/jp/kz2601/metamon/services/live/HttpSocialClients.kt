package jp.kz2601.metamon.services.live

import jp.kz2601.metamon.dsl.records.MatchBox
import jp.kz2601.metamon.dsl.records.MatchInterest
import jp.kz2601.metamon.services.http.ApiJson
import jp.kz2601.metamon.services.http.HttpRequest
import jp.kz2601.metamon.services.http.MetamonHttp
import jp.kz2601.metamon.services.social.BlockedUser
import jp.kz2601.metamon.services.social.MatchingClient
import jp.kz2601.metamon.services.social.ModerationClient
import jp.kz2601.metamon.services.social.ReportReason
import jp.kz2601.metamon.services.social.ReportTarget
import kotlinx.serialization.Serializable
import kotlinx.serialization.json.buildJsonObject
import kotlinx.serialization.json.put

/** 仲間探しの API(docs/backend/openapi-appconfig.yaml `matching` タグ・#507)。iOS の `HTTPMatchingClient`。 */
class HttpMatchingClient(private val http: MetamonHttp) : MatchingClient {
    override suspend fun sendInterest(appId: String, recordId: String, fromRecordId: String, ageConfirmed: Boolean): MatchInterest {
        val body = buildJsonObject {
            put("recordId", recordId)
            put("fromRecordId", fromRecordId)
            put("ageConfirmed", ageConfirmed)
        }
        return decode(http.send(HttpRequest("POST", listOf("v1", "apps", appId, "interests"), body = body.toString(), timeoutMillis = TIMEOUT)))
    }

    override suspend fun list(appId: String, box: MatchBox): List<MatchInterest> {
        val req = when (box) {
            MatchBox.INCOMING, MatchBox.OUTGOING ->
                HttpRequest("GET", listOf("v1", "apps", appId, "interests"), query = listOf("box" to box.wire), timeoutMillis = TIMEOUT)
            MatchBox.MATCHES -> HttpRequest("GET", listOf("v1", "apps", appId, "matches"), timeoutMillis = TIMEOUT)
        }
        return ApiJson.decodeFromString(Items.serializer(), http.send(req)).items
    }

    override suspend fun accept(appId: String, uid: String, ageConfirmed: Boolean): MatchInterest {
        val body = buildJsonObject { put("ageConfirmed", ageConfirmed) }
        return decode(http.send(HttpRequest("POST", listOf("v1", "apps", appId, "interests", uid, "accept"), body = body.toString(), timeoutMillis = TIMEOUT)))
    }

    override suspend fun decline(appId: String, uid: String) {
        http.send(HttpRequest("DELETE", listOf("v1", "apps", appId, "interests", uid), timeoutMillis = TIMEOUT))
    }

    override suspend fun unmatch(appId: String, uid: String) {
        http.send(HttpRequest("DELETE", listOf("v1", "apps", appId, "matches", uid), timeoutMillis = TIMEOUT))
    }

    private fun decode(body: String): MatchInterest = ApiJson.decodeFromString(MatchInterest.serializer(), body)

    @Serializable
    private data class Items(val items: List<MatchInterest> = emptyList())

    private companion object {
        const val TIMEOUT = 20_000L
    }
}

/** 通報・ブロックの API(docs/backend/openapi-appconfig.yaml `moderation` タグ・#321)。iOS の `HTTPSharingClient+Moderation`。 */
class HttpModerationClient(private val http: MetamonHttp) : ModerationClient {
    override suspend fun report(target: ReportTarget, reason: ReportReason) {
        val body = buildJsonObject {
            when (target) {
                is ReportTarget.Listing -> {
                    put("kind", "listing")
                    put("id", target.id)
                }
                is ReportTarget.RecordTarget -> {
                    put("kind", "record")
                    put("appId", target.appId)
                    put("entityId", target.entityId)
                    put("recordId", target.recordId)
                }
            }
            put("reason", reason.wire)
        }
        http.send(HttpRequest("POST", listOf("v1", "reports"), body = body.toString(), timeoutMillis = TIMEOUT))
    }

    override suspend fun blockedUsers(): List<BlockedUser> =
        ApiJson.decodeFromString(Items.serializer(), http.send(HttpRequest("GET", listOf("v1", "me", "blocks"), timeoutMillis = TIMEOUT))).items

    override suspend fun block(uid: String, name: String?): BlockedUser = postBlock(
        buildJsonObject {
            put("uid", uid)
            // backend の上限は 64 文字
            name?.takeIf { it.isNotEmpty() }?.let { put("name", it.take(NAME_MAX)) }
        }.toString(),
    )

    override suspend fun blockListing(listingId: String): BlockedUser = postBlock(buildJsonObject { put("listingId", listingId) }.toString())

    override suspend fun unblock(uid: String) {
        http.send(HttpRequest("DELETE", listOf("v1", "me", "blocks", uid), timeoutMillis = TIMEOUT))
    }

    private suspend fun postBlock(body: String): BlockedUser =
        ApiJson.decodeFromString(BlockedUser.serializer(), http.send(HttpRequest("POST", listOf("v1", "me", "blocks"), body = body, timeoutMillis = TIMEOUT)))

    @Serializable
    private data class Items(val items: List<BlockedUser> = emptyList())

    private companion object {
        const val TIMEOUT = 20_000L
        const val NAME_MAX = 64
    }
}
