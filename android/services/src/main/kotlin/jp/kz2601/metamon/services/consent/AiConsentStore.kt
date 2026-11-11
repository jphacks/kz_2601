package jp.kz2601.metamon.services.consent

import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import java.time.Instant

/** 同意の状態(#888)。取り消した人は「あとで」と同じ扱い(生成しようとした時にまた出す)。 */
enum class AiConsentStatus {
    /** まだ説明を出していない(初回起動で出す) */
    NOT_PROMPTED,

    /** 説明は出したが同意していない(「あとで」・取り消し) */
    DEFERRED,

    /** 古い版の説明に同意したまま(送る中身を変えたので、もう一度同意が要る) */
    NEEDS_RECONSENT,

    /** 今の版に同意済み */
    GRANTED,
}

data class AiConsentState(val status: AiConsentStatus, val grantedAt: Instant?) {
    val isGranted: Boolean get() = status == AiConsentStatus.GRANTED
    val needsReconsent: Boolean get() = status == AiConsentStatus.NEEDS_RECONSENT
}

/**
 * 第三者 AI(Anthropic の Claude)へ送ることへの同意(iOS の `AIConsentStore`・#888)。
 * 端末に「同意した説明の版」と日時を保存する。説明の中身を変えたら [CURRENT_VERSION] を上げると全員に出し直す。
 * 版と鍵は iOS と同じ(説明の文言の正は iOS の Localizable.xcstrings)。
 */
class AiConsentStore(
    private val store: KeyValueStore,
    private val clock: () -> Instant = Instant::now,
) {
    private val _state = MutableStateFlow(read())
    val state: StateFlow<AiConsentState> = _state.asStateFlow()

    val isGranted: Boolean get() = _state.value.isGranted

    fun grant() {
        val now = clock()
        store.putInt(VERSION_KEY, CURRENT_VERSION)
        store.putLong(DATE_KEY, now.toEpochMilli())
        markPrompted()
        _state.value = read()
    }

    /** 取り消し。以後の生成・対話は、もう一度同意するまで送らない(送信済みのものは取り消せない旨を画面に書く)。 */
    fun revoke() {
        store.remove(VERSION_KEY)
        store.remove(DATE_KEY)
        _state.value = read()
    }

    /** 説明を出した(「あとで」でも)。「あとで」の人には起動のたびには出さない。 */
    fun markPrompted() {
        store.putBoolean(PROMPTED_KEY, true)
        store.putInt(PROMPTED_VERSION_KEY, CURRENT_VERSION)
        _state.value = read()
    }

    /**
     * サインアウト・アカウント削除・別の人のサインイン(#841)。同意は人ごとなので、説明を出した印ごと忘れる
     * (iOS は `LocalWipe.clearDefaults` が同じ鍵を消す)。次の人には起動時に説明を出し直す。
     */
    fun forget() {
        store.remove(VERSION_KEY)
        store.remove(DATE_KEY)
        store.remove(PROMPTED_KEY)
        store.remove(PROMPTED_VERSION_KEY)
        _state.value = read()
    }

    /** 保存先を直接消した後(アカウント削除など)に、状態を保存先に合わせ直す。 */
    fun reload() {
        _state.value = read()
    }

    /** 起動時に同意の画面を出すか。初回と、版が上がって出し直す時(その版で 1 回だけ)。 */
    val shouldPromptOnLaunch: Boolean get() = when (_state.value.status) {
        AiConsentStatus.GRANTED, AiConsentStatus.DEFERRED -> false
        AiConsentStatus.NOT_PROMPTED -> true
        AiConsentStatus.NEEDS_RECONSENT -> (store.getInt(PROMPTED_VERSION_KEY) ?: 0) < CURRENT_VERSION
    }

    private fun read(): AiConsentState {
        val version = store.getInt(VERSION_KEY) ?: 0
        val status = when {
            version >= CURRENT_VERSION -> AiConsentStatus.GRANTED
            version > 0 -> AiConsentStatus.NEEDS_RECONSENT
            store.getBoolean(PROMPTED_KEY) == true -> AiConsentStatus.DEFERRED
            else -> AiConsentStatus.NOT_PROMPTED
        }
        val at = if (status == AiConsentStatus.GRANTED) store.getLong(DATE_KEY)?.let(Instant::ofEpochMilli) else null
        return AiConsentState(status, at)
    }

    companion object {
        /**
         * 説明文の版(iOS の `AIConsentStore.currentVersion` と同じ)。送る中身・送り先を変えたら上げる(ポリシー 6 章)。
         * 3: 長い答えは最大 16KB / aiChat 直近 20 発言 / 写真のボタン 1 枚 / 共有は自分の記録だけ(ポリシー 2026-11-08 改定)。
         */
        const val CURRENT_VERSION = 3

        const val VERSION_KEY = "metamon.aiConsent.version"
        const val DATE_KEY = "metamon.aiConsent.date"
        const val PROMPTED_KEY = "metamon.aiConsent.prompted"
        const val PROMPTED_VERSION_KEY = "metamon.aiConsent.promptedVersion"
    }
}
