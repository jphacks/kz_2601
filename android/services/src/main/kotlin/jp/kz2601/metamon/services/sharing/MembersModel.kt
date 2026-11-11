package jp.kz2601.metamon.services.sharing

import jp.kz2601.metamon.dsl.AppDSL
import jp.kz2601.metamon.services.http.ApiError
import jp.kz2601.metamon.services.http.FailureKind
import jp.kz2601.metamon.services.http.SharedAccessDenial
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch

/**
 * アプリごとの参加者と自分が管理者か(iOS の `AppMembersStore`・#272)。
 * 部品の member の選択肢・管理者だけの削除 / ボタン(visibleTo・#1089)が読む受け口。
 */
class AppMembersStore(private val client: MembersClient?, private val scope: CoroutineScope) {
    data class Entry(
        val members: List<SharedMember>,
        /** null = 分からない(読めなかった) */
        val isAdmin: Boolean?,
        /** 自分が作成者か(応答の role)。null = 分からない */
        val isOwner: Boolean? = null,
    )

    private val _entries = MutableStateFlow<Map<String, Entry>>(emptyMap())
    val entries: StateFlow<Map<String, Entry>> = _entries.asStateFlow()

    operator fun get(appId: String): Entry? = _entries.value[appId]

    /** DSL を当てた時: 参加者を使うアプリなら読み直す。 */
    fun note(appId: String, dsl: AppDSL) {
        if (client == null || !dsl.usesMembers()) return
        scope.launch { reload(appId) }
    }

    /** 読み直す。参加していなければ空・管理者でない。それ以外の失敗は前の値のまま。 */
    suspend fun reload(appId: String) {
        val client = client ?: return
        try {
            put(appId, client.members(appId))
        } catch (e: CancellationException) {
            throw e
        } catch (e: Exception) {
            if (SharedAccessDenial.from(e) == SharedAccessDenial.NOT_MEMBER) _entries.update { it + (appId to Entry(emptyList(), isAdmin = false, isOwner = false)) }
        }
    }

    fun put(appId: String, list: SharedMemberList) = _entries.update { it + (appId to Entry(list.items, list.isAdmin, list.isOwner)) }

    fun forget(appId: String) = _entries.update { it - appId }

    fun clear() {
        _entries.value = emptyMap()
    }
}

/** 参加者の欄の段階(iOS の `SharedMembersModel.Phase`)。 */
enum class MembersPhase {
    /** 共有の記録があるか確かめ中(何も出さない) */
    PROBING,

    /** 読み込み中(スケルトン) */
    LOADING,

    /** 共有の記録が無いアプリ・使えない構成(欄ごと出さない) */
    HIDDEN,
    LOADED,

    /** 自分が参加者ではない(403 not_member) */
    NOT_MEMBER,
    FAILED,

    /** 自分で抜けた */
    LEFT,
}

/** 操作の失敗(画面はこれで文言を選ぶ・元に戻した後に出す)。 */
enum class MembersFailure { REMOVE, SET_ADMIN, LEAVE, OFFLINE }

data class MembersState(
    val phase: MembersPhase = MembersPhase.PROBING,
    val list: SharedMemberList? = null,
    /** 外した直後の人。リンクの取り消しへ誘導する(作成者のリンクは取り消すまで有効なので) */
    val justRemoved: SharedMember? = null,
    /** 抜ける途中(二度押しを防ぐ) */
    val leaving: Boolean = false,
) {
    val isVisible: Boolean get() = phase != MembersPhase.PROBING && phase != MembersPhase.HIDDEN
}

/**
 * 共有シート・設定の「参加している人」(iOS の `SharedMembersModel`)。外す・管理者・抜けるは楽観的 UI(先に反映し、失敗したら戻す)。
 * @param client null = 使えない構成(モック)
 * @param isShared scope:shared の entity があるか(無いアプリでは欄を出さない)
 * @param store 成功したら部品の受け口([AppMembersStore])にも入れる
 */
class MembersModel(
    private val appId: String,
    private val client: MembersClient?,
    private val isShared: suspend () -> Boolean,
    private val store: AppMembersStore? = null,
) {
    private val _state = MutableStateFlow(MembersState())
    val state: StateFlow<MembersState> = _state.asStateFlow()

    suspend fun load() {
        val client = client ?: return _state.update { it.copy(phase = MembersPhase.HIDDEN) }
        val probing = _state.value.phase == MembersPhase.PROBING
        try {
            if (probing) {
                if (!isShared()) return _state.update { it.copy(phase = MembersPhase.HIDDEN) }
                _state.update { it.copy(phase = MembersPhase.LOADING) }
            }
            val list = client.members(appId)
            store?.put(appId, list)
            _state.update { it.copy(phase = MembersPhase.LOADED, list = list) }
        } catch (e: CancellationException) {
            throw e
        } catch (e: Exception) {
            val phase = when {
                SharedAccessDenial.from(e) == SharedAccessDenial.NOT_MEMBER -> MembersPhase.NOT_MEMBER
                probing && _state.value.phase == MembersPhase.PROBING -> MembersPhase.HIDDEN // 共有か分からない時は出さない
                else -> MembersPhase.FAILED
            }
            _state.update { it.copy(phase = phase) }
        }
    }

    /** 作成者が他の人を外す(確認は画面で取る)。先に一覧から消し、失敗したら元の位置に戻す。 */
    suspend fun remove(member: SharedMember): MembersFailure? {
        val client = client ?: return null
        val current = _state.value.list ?: return null
        val index = current.items.indexOfFirst { it.uid == member.uid }
        if (index < 0 || !MemberActions.canRemove(current, member)) return null
        _state.update { it.copy(list = current.copy(items = current.items - current.items[index]), justRemoved = member) }
        return try {
            client.removeMember(appId, member.uid)
            _state.value.list?.let { store?.put(appId, it) }
            null
        } catch (e: CancellationException) {
            throw e
        } catch (e: Exception) {
            if ((e as? ApiError)?.status == 404) return null // もう参加していない = 目的は達成
            _state.update { s ->
                val back = s.list?.takeIf { l -> l.items.none { it.uid == member.uid } }
                    ?.let { l -> l.copy(items = l.items.toMutableList().apply { add(index.coerceAtMost(size), member) }) }
                s.copy(list = back ?: s.list, justRemoved = s.justRemoved?.takeIf { it.uid != member.uid })
            }
            failureOf(e, MembersFailure.REMOVE)
        }
    }

    /** 管理者にする / 外す。先に反映し、失敗したら戻す。 */
    suspend fun setAdmin(member: SharedMember, admin: Boolean): MembersFailure? {
        val client = client ?: return null
        val current = _state.value.list ?: return null
        if (current.items.none { it.uid == member.uid } || !MemberActions.canSetAdmin(current, member)) return null
        _state.update { s -> s.copy(list = s.list?.withAdmin(member.uid, admin)) }
        return try {
            client.setAdmin(appId, member.uid, admin)
            _state.value.list?.let { store?.put(appId, it) }
            null
        } catch (e: CancellationException) {
            throw e
        } catch (e: Exception) {
            _state.update { s -> s.copy(list = s.list?.withAdmin(member.uid, member.admin)) }
            failureOf(e, MembersFailure.SET_ADMIN)
        }
    }

    /** 自分が抜ける(確認は画面で取る)。先に「抜けました」にし、失敗したら戻す。作成者は抜けられない。 */
    suspend fun leave(): MembersFailure? {
        val client = client ?: return null
        val s = _state.value
        val list = s.list
        if (s.leaving || (list != null && !MemberActions.canLeave(list))) return null
        val before = s.phase
        _state.update { it.copy(phase = MembersPhase.LEFT, leaving = true) }
        return try {
            client.leave(appId)
            store?.forget(appId)
            _state.update { it.copy(leaving = false) }
            null
        } catch (e: CancellationException) {
            _state.update { it.copy(phase = before, leaving = false) }
            throw e
        } catch (e: Exception) {
            _state.update { it.copy(phase = before, leaving = false) }
            failureOf(e, MembersFailure.LEAVE)
        }
    }

    fun dismissRemoved() = _state.update { it.copy(justRemoved = null) }

    private fun failureOf(e: Throwable, fallback: MembersFailure): MembersFailure = when (FailureKind.classify(e)) {
        FailureKind.Offline, FailureKind.Timeout -> MembersFailure.OFFLINE
        else -> fallback
    }

    private fun SharedMemberList.withAdmin(uid: String, admin: Boolean) =
        copy(items = items.map { if (it.uid == uid) it.copy(admin = admin) else it })
}
