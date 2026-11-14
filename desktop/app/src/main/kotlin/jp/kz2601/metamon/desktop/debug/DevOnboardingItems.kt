package jp.kz2601.metamon.desktop.debug

import jp.kz2601.metamon.desktop.i18n.Strings
import jp.kz2601.metamon.services.consent.KeyValueStore

/** 初回の流れの「もう見た」を消す口(#3213)。初回の流れの規則(#3201・onboarding/)が入ったら、その規則の印を消す物に差し替える。 */
fun interface OnboardingReplay {
    fun replay()
}

/**
 * 「もう見た」の印を端末の設定から消す。鍵は iOS の `FirstRunTutorialStore` と同じ(デスクトップの規則もこの鍵を使う)。
 * AI に送る同意は消さない(iOS の開発用メニューでも別の項目)。
 */
class SeenFlagsOnboardingReplay(private val settings: KeyValueStore) : OnboardingReplay {
    override fun replay() = KEYS.forEach(settings::remove)

    companion object {
        val KEYS = listOf("metamon.tutorial.seen", "metamon.tutorial.editHintShown")
    }
}

/** 「初回の流れをやり直す」(iOS の #2081 と同じ項目)。印を消し、次の起動で出し直す。 */
object DevOnboardingItems {
    /** Main.kt が端末の設定につなぐ。null なら項目を出さない。 */
    @Volatile
    var hook: OnboardingReplay? = null

    fun items(host: DevMenuHost): List<DevMenuItem> = hook?.let { replay ->
        listOf(
            DevMenuItem("onboarding:replay", Strings["desktop_dev_onboarding_replay"], listOf("onboarding", "tutorial", "first run")) {
                replay.replay()
                host.message(Strings["desktop_dev_onboarding_replay_done"])
            },
        )
    }.orEmpty()
}
