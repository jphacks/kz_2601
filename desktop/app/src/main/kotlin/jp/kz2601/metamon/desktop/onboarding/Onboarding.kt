package jp.kz2601.metamon.desktop.onboarding

import jp.kz2601.metamon.services.consent.AiConsentStatus
import jp.kz2601.metamon.services.consent.AiConsentStore
import jp.kz2601.metamon.services.consent.KeyValueStore

/**
 * 初回の案内の段(iOS の `FirstRunTutorial`・Mac の `MacOnboarding` と同じ順)。
 * AI に送る同意の説明 → ようこそ(チュートリアル 1 枚)→ 最初のアプリの後の「こう話すと直せます」。
 */
enum class OnboardingStep { CONSENT, WELCOME, EDIT_HINT }

/**
 * ようこそ・「こう話すと直せます」を見た印(iOS の `FirstRunTutorialStore`・鍵も同じ)。
 * 端末の設定(DesktopStore.settings)に置くので、サインアウトと「この端末のデータを消す」(settings.clear)で消える。
 * 毎回 [store] を読む(消した後に覚え直さなくてよい)。
 */
class OnboardingSeenStore(private val store: KeyValueStore) {
    val hasSeenWelcome: Boolean get() = store.getBoolean(SEEN_KEY) == true
    val hasShownEditHint: Boolean get() = store.getBoolean(EDIT_HINT_KEY) == true

    fun markWelcomeSeen() = store.putBoolean(SEEN_KEY, true)

    fun markEditHintShown() = store.putBoolean(EDIT_HINT_KEY, true)

    /** 人を替える時(設定ごと消さずに印だけ消す)。 */
    fun forget() {
        store.remove(SEEN_KEY)
        store.remove(EDIT_HINT_KEY)
    }

    companion object {
        const val SEEN_KEY = "metamon.tutorial.seen"
        const val EDIT_HINT_KEY = "metamon.tutorial.editHintShown"
    }
}

/** 次に出す段を決める(画面なし)。出した後は呼ぶ側が同意の markPrompted・[OnboardingSeenStore] の mark を呼ぶ。 */
class Onboarding(private val consent: AiConsentStore, private val seen: OnboardingSeenStore) {
    /** 同意の説明を出し終えたか(同意した・「あとで」・版が上がって出し直し待ちのどれでも)。 */
    private val consentPrompted: Boolean get() = consent.state.value.status != AiConsentStatus.NOT_PROMPTED

    /**
     * 今出す段。無ければ null。
     * - 同意: 初回と、版が上がった時に 1 回(`shouldPromptOnLaunch`)
     * - ようこそ: 同意の説明を出し終えてから、まだ見ていなければ
     * - 「こう話すと直せます」: 最初のアプリを作った直後([firstAppMade])に 1 回だけ。ようこその後
     */
    fun next(firstAppMade: Boolean = false): OnboardingStep? = when {
        consent.shouldPromptOnLaunch -> OnboardingStep.CONSENT
        consentPrompted && !seen.hasSeenWelcome -> OnboardingStep.WELCOME
        firstAppMade && seen.hasSeenWelcome && !seen.hasShownEditHint -> OnboardingStep.EDIT_HINT
        else -> null
    }

    /** 段を閉じた(同意・「あとで」・スキップのどれでも見た扱い)。同意の段は同意の画面が grant / markPrompted を書く。 */
    fun finish(step: OnboardingStep) {
        when (step) {
            OnboardingStep.CONSENT -> if (!consentPrompted) consent.markPrompted()
            OnboardingStep.WELCOME -> seen.markWelcomeSeen()
            OnboardingStep.EDIT_HINT -> seen.markEditHintShown()
        }
    }
}
