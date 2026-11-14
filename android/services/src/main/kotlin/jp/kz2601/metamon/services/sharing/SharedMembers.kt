package jp.kz2601.metamon.services.sharing

import jp.kz2601.metamon.dsl.AppDSL
import jp.kz2601.metamon.dsl.Component
import jp.kz2601.metamon.dsl.EditableBy
import jp.kz2601.metamon.dsl.FieldType
import jp.kz2601.metamon.services.http.ApiJson
import jp.kz2601.metamon.services.http.HttpRequest
import jp.kz2601.metamon.services.http.MetamonHttp
import kotlinx.serialization.Serializable
import kotlinx.serialization.json.buildJsonObject
import kotlinx.serialization.json.put

/** 参加者 1 人(openapi-appconfig `Member`・#254 / #272)。 */
@Serializable
data class SharedMember(
    val uid: String,
    /** 参加した時の表示名。作成者は常に空 */
    val displayName: String = "",
    /** owner | member */
    val role: String = ROLE_MEMBER,
    val admin: Boolean = false,
    /** share | legacy。作成者には無い */
    val via: String? = null,
    val joinedAt: String? = null,
    /** 呼び出した本人 */
    val you: Boolean = false,
) {
    val isOwner: Boolean get() = role == ROLE_OWNER

    /** 作成者は常に管理者 */
    val isAdmin: Boolean get() = admin || isOwner
}

/** `GET /v1/apps/{appId}/members` の応答。[role] / [admin] は呼び出した人の立場。 */
@Serializable
data class SharedMemberList(
    val items: List<SharedMember> = emptyList(),
    val role: String = ROLE_MEMBER,
    val admin: Boolean? = null,
) {
    val isOwner: Boolean get() = role == ROLE_OWNER
    val isAdmin: Boolean get() = admin ?: isOwner

    /** 上限に達していて、これ以上は参加できない。backend は作成者を除いた参加者で数える(一覧には作成者も 1 行ある) */
    val isFull: Boolean get() = items.count { !it.isOwner } >= MEMBER_LIMIT
}

const val ROLE_OWNER = "owner"
const val ROLE_MEMBER = "member"

/** 参加者の上限(backend の `store.MaxMembers`・members_full と同じ)。作成者はこの数に入らない */
const val MEMBER_LIMIT = 100

/** 役割ごとに押せる操作(iOS の SharedMembersSection と同じ判定)。 */
object MemberActions {
    /** 外す: 作成者だけ・自分と作成者は外せない */
    fun canRemove(list: SharedMemberList, member: SharedMember): Boolean = list.isOwner && !member.you && !member.isOwner

    /** 管理者にする / 外す: 管理者だけ・自分と作成者は変えられない */
    fun canSetAdmin(list: SharedMemberList, member: SharedMember): Boolean = list.isAdmin && !member.you && !member.isOwner

    /** 抜ける: 作成者以外(作成者は抜けられない・アプリを消す) */
    fun canLeave(list: SharedMemberList): Boolean = !list.isOwner
}

/** members API(docs/backend/openapi-appconfig.yaml `members` タグ)。iOS の `MembershipClient`。 */
interface MembersClient {
    suspend fun members(appId: String): SharedMemberList

    /** `PATCH /members/{uid}` `{admin}`。管理者だけ */
    suspend fun setAdmin(appId: String, uid: String, admin: Boolean)

    /** `DELETE /members/{uid}`。作成者だけ */
    suspend fun removeMember(appId: String, uid: String)

    /** `DELETE /members/me`(冪等) */
    suspend fun leave(appId: String)
}

class HttpMembersClient(private val http: MetamonHttp) : MembersClient {
    override suspend fun members(appId: String): SharedMemberList =
        ApiJson.decodeFromString(SharedMemberList.serializer(), http.send(request("GET", appId)))

    override suspend fun setAdmin(appId: String, uid: String, admin: Boolean) {
        http.send(request("PATCH", appId, uid, buildJsonObject { put("admin", admin) }.toString()))
    }

    override suspend fun removeMember(appId: String, uid: String) {
        http.send(request("DELETE", appId, uid))
    }

    override suspend fun leave(appId: String) {
        http.send(request("DELETE", appId, ME))
    }

    private fun request(method: String, appId: String, uid: String? = null, body: String? = null) =
        HttpRequest(method, listOfNotNull("v1", "apps", appId, "members", uid), body = body, timeoutMillis = TIMEOUT)

    private companion object {
        const val ME = "me"
        const val TIMEOUT = 20_000L
    }
}

/** 参加者を使う DSL か(member 型・editableBy・deleteBy・visibleTo)。使わないアプリでは members API を呼ばない(iOS の `usesMembers`)。 */
fun AppDSL.usesMembers(): Boolean {
    val fields = entities.any { e ->
        e.deleteBy != null || e.fields.any { it.type == FieldType.MEMBER || it.type == FieldType.MEMBERS || it.editableBy == EditableBy.ADMINS }
    }
    return fields || screens.any { s -> s.components.any(::hasVisibleTo) }
}

private fun hasVisibleTo(c: Component): Boolean = c.visibleTo != null || c.children.any(::hasVisibleTo)
