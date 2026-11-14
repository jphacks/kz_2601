package jp.kz2601.metamon.onboarding

import androidx.annotation.StringRes
import jp.kz2601.metamon.R
import jp.kz2601.metamon.services.consent.KeyValueStore
import jp.kz2601.metamon.ui.CranePose
import jp.kz2601.metamon.voice.stage.EditHintStore
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow

/**
 * 初回のチュートリアル(#900・iOS の `FirstRunTutorial`)。カードの中身。
 * 初回の順序は iOS と同じ 同意(#888)→ チュートリアル(1 枚)。
 * 「直せる・戻せる」は最初のアプリの直後の「こう話すと直せます」(`BuildStageHost`)に任せる。
 */
object FirstRunTutorial {
    data class Card(
        val id: String,
        val pose: CranePose,
        @param:StringRes val title: Int,
        @param:StringRes val body: Int,
        /** 補足(小さい字)。声の準備中も文字で作れること */
        @param:StringRes val note: Int? = null,
    )

    /** 初回の関門を減らすため 1 枚だけ(iOS の UX 点検 H1)。 */
    val cards: List<Card> = listOf(
        Card(
            id = "create",
            pose = CranePose.HAPPY,
            title = R.string.onboarding_create_title,
            body = R.string.onboarding_create_body,
            note = R.string.onboarding_create_note,
        ),
    )
}

/**
 * チュートリアルと「こう話すと直せます」を見たかを端末に残す(iOS の `FirstRunTutorialStore`)。
 * 「こう話すと直せます」を出す時点と順番は `BuildStageState` / `PostBuildNotice` が持ち、ここは見た印だけ([editHints])。
 */
class FirstRunTutorialStore(private val store: KeyValueStore) {
    private val _hasSeenTutorial = MutableStateFlow(store.getBoolean(SEEN_KEY) == true)
    val hasSeenTutorial: StateFlow<Boolean> = _hasSeenTutorial.asStateFlow()

    val hasShownEditHint: Boolean get() = store.getBoolean(EDIT_HINT_KEY) == true

    fun markTutorialSeen() {
        store.putBoolean(SEEN_KEY, true)
        _hasSeenTutorial.value = true
    }

    fun markEditHintShown() = store.putBoolean(EDIT_HINT_KEY, true)

    /** 自動で出すか。同意の説明を出し終えてから、まだ見ていなければ。 */
    fun shouldAutoPresent(consentPrompted: Boolean): Boolean = consentPrompted && !_hasSeenTutorial.value

    /** 完成の後のお知らせ(`BuildStageState`)に渡す「こう話すと直せます」の印。 */
    val editHints: EditHintStore = EditHintStore(read = { hasShownEditHint }, write = ::markEditHintShown)

    /**
     * 後片付け(#841)。iOS は `LocalWipe.clearDefaults` がこの 2 つの鍵も消す(残す鍵に入っていない)ので、
     * 次の人には同意の後にチュートリアルを、最初のアプリの後に「こう話すと直せます」を出し直す。
     */
    fun forget() {
        store.remove(SEEN_KEY)
        store.remove(EDIT_HINT_KEY)
        _hasSeenTutorial.value = false
    }

    /** 自動のテスト・モック用: 両方見た扱い(iOS の `METAMON_AI_CONSENT=granted` の時の markAllSeen)。 */
    fun markAllSeen() {
        markTutorialSeen()
        markEditHintShown()
    }

    companion object {
        /** SharedPreferences の名前。#1157 の「こう話すと直せます」の印と同じ置き場 */
        const val PREFS = "metamon.tutorial"
        const val SEEN_KEY = "seen"
        const val EDIT_HINT_KEY = "editHintShown"
    }
}

/**
 * チュートリアルを出す時点(iOS の `AppRuntime+Tutorial`)。
 * - 起動時・同意の画面が閉じた時: 同意の説明を出し終えていて、まだ見ていなければ([presentIfNeeded])
 * - 設定の「使い方を見る」から何度でも([replay])
 * - スキップ・「はじめる」・戻るのどれでも見た扱い([finish])
 */
class TutorialPrompt(private val store: FirstRunTutorialStore) {
    private val _visible = MutableStateFlow(false)
    val visible: StateFlow<Boolean> = _visible.asStateFlow()

    /** @param consentShowing 同意の画面が出ている間は出さない(重ねない) */
    fun presentIfNeeded(consentPrompted: Boolean, consentShowing: Boolean) {
        if (_visible.value || consentShowing || !store.shouldAutoPresent(consentPrompted)) return
        _visible.value = true
    }

    fun replay() {
        _visible.value = true
    }

    fun finish() {
        store.markTutorialSeen()
        _visible.value = false
    }

    /** 後片付け(#841)。前の人の画面を閉じる(見た印は [FirstRunTutorialStore.forget] が消す) */
    fun reset() {
        _visible.value = false
    }
}
