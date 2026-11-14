package jp.kz2601.metamon.consent

import android.content.Context
import android.content.Intent
import android.os.SystemClock
import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.assertIsOff
import androidx.compose.ui.test.assertIsOn
import androidx.compose.ui.test.assertTextEquals
import androidx.compose.ui.test.junit4.createEmptyComposeRule
import androidx.compose.ui.test.onAllNodesWithTag
import androidx.compose.ui.test.onNodeWithTag
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performClick
import androidx.compose.ui.test.performScrollTo
import androidx.compose.ui.test.performTextInput
import androidx.test.core.app.ActivityScenario
import androidx.test.core.app.ApplicationProvider
import androidx.test.ext.junit.runners.AndroidJUnit4
import jp.kz2601.metamon.AppContainer
import jp.kz2601.metamon.MainActivity
import jp.kz2601.metamon.MetamonApplication
import jp.kz2601.metamon.R
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith

/**
 * 同意の画面(`consent.sheet`)と設定のプライバシー(#888)の画面のテスト(#1172)。
 * モックの構成(`METAMON_MOCK=1`)で起動する。タグは iOS の accessibilityIdentifier と同じ名前。
 * - 初回起動(説明未表示)で同意の画面が出て、「あとで」で閉じると次の起動では出ない
 * - 設定の「説明を読んで同意する」→「同意して使う」で「同意済み」になる
 * - 「同意を取り消す」→ 確認 →「未同意」になる
 * - 取り消した後に話しかけると同意の画面が出て、生成は始まらない(送らない)
 * - クラッシュ報告・AI ボタンの利用状況の切り替えが起動し直しても残る
 *
 * つまみ(`METAMON_AI_CONSENT`)が効くのは構成を組み立てる最初の起動だけなので、同意の状態は毎回
 * [jp.kz2601.metamon.services.consent.AiConsentStore] に直に作ってから起動する(モックの構成は同意済みで始まる)。
 *
 * 端末かエミュレータで `./gradlew :app:connectedStgDebugAndroidTest`。
 */
@RunWith(AndroidJUnit4::class)
class AiConsentUiTest {
    @get:Rule
    val compose = createEmptyComposeRule()

    private val app = ApplicationProvider.getApplicationContext<MetamonApplication>()
    private val context: Context get() = app
    private var scenario: ActivityScenario<MainActivity>? = null

    @After
    fun restore() {
        close()
        app.builtContainer?.let {
            // 次のテスト・次のクラスにはモックの既定(同意済み・報告はオン)で渡す
            it.consentPrompt.reset()
            it.consent.grant()
            it.crashReporting.setEnabled(true)
            it.aiButtonMetrics.setEnabled(true)
        }
    }

    @Test
    fun firstLaunchPromptsAndLaterIsNotShownAgain() {
        container().consent.forget() // 初回(まだ説明を出していない)
        launch()

        waitFor("consent.sheet")
        compose.onNodeWithTag("consent.sheet").assertIsDisplayed()
        compose.onNodeWithText(string(R.string.consent_title)).assertIsDisplayed()
        compose.onNodeWithTag("consent.later").performClick()
        waitGone("consent.sheet")
        assertFalse(container().consent.isGranted)

        close()
        launch() // 「あとで」の人には起動のたびには出さない
        assertNoConsentOnLaunch()
    }

    @Test
    fun settingsGrantsConsent() {
        revoked()
        launch()
        openSettings()

        status().assertTextEquals(string(R.string.settings_ai_consent_not_granted))
        compose.onNodeWithTag("settings.aiConsent.grant").performScrollTo().performClick()
        waitFor("consent.sheet")
        compose.onNodeWithTag("consent.agree").performClick()
        waitGone("consent.sheet")

        assertTrue(container().consent.isGranted)
        status().assertTextEquals(string(R.string.settings_ai_consent_granted))
    }

    @Test
    fun settingsRevokesConsentAfterConfirm() {
        container().consent.grant()
        launch()
        openSettings()

        status().assertTextEquals(string(R.string.settings_ai_consent_granted))
        compose.onNodeWithTag("settings.aiConsent.revoke").performScrollTo().performClick()
        compose.onNodeWithText(string(R.string.settings_revoke_title)).assertIsDisplayed()
        compose.onNodeWithText(string(R.string.settings_revoke_confirm)).performClick()

        compose.waitUntil(TIMEOUT_MILLIS) { !container().consent.isGranted }
        status().assertTextEquals(string(R.string.settings_ai_consent_not_granted))
    }

    @Test
    fun talkingAfterRevokeShowsConsentAndSendsNothing() {
        revoked() // 取り消した後(説明は出した・未同意)。起動では出さず、話しかけた時に出す
        launch()
        assertNoConsentOnLaunch()
        val version = container().runtime.state.value.document.version

        openVoice()
        compose.onNodeWithTag("voice.composer").performTextInput(UTTERANCE)
        compose.onNodeWithTag("voice.send").performClick()

        waitFor("consent.sheet")
        // 送らない: 生成は始まらず、アプリの版も同意の状態も変わらない
        assertFalse(exists("generating.create"))
        assertEquals(version, container().runtime.state.value.document.version)
        assertFalse(container().consent.isGranted)
    }

    @Test
    fun crashReportingAndMetricsTogglesSurviveRelaunch() {
        val crashReporting = container().crashReporting
        val metrics = container().aiButtonMetrics
        crashReporting.setEnabled(true)
        metrics.setEnabled(true)

        launch()
        openSettings()
        compose.onNodeWithTag("settings.crashReporting.toggle").performScrollTo().assertIsOn().performClick()
        compose.onNodeWithTag("settings.aiButtonMetrics.toggle").performScrollTo().assertIsOn().performClick()
        compose.waitUntil(TIMEOUT_MILLIS) { !crashReporting.isEnabled && !metrics.isEnabled }

        close()
        launch()
        openSettings()
        compose.onNodeWithTag("settings.crashReporting.toggle").performScrollTo().assertIsOff()
        compose.onNodeWithTag("settings.aiButtonMetrics.toggle").performScrollTo().assertIsOff()
    }

    /** 組み立て済みの構成(無ければ 1 回起動して組み立てる。モックの構成は同意済みなので画面は塞がらない) */
    private fun container(): AppContainer {
        app.builtContainer?.let { return it }
        launch()
        close()
        return checkNotNull(app.builtContainer) { "構成が組み立てられていない" }
    }

    /** 取り消した後の状態。iOS と同じく「あとで」と同じ扱い(説明は出した・未同意) */
    private fun revoked() = container().consent.apply {
        grant()
        revoke()
    }

    private fun launch() {
        scenario = ActivityScenario.launch(
            Intent(context, MainActivity::class.java).putExtra("METAMON_MOCK", "1"),
        )
    }

    private fun close() {
        scenario?.close()
        scenario = null
    }

    private fun string(id: Int) = context.getString(id)

    private fun exists(tag: String) = compose.onAllNodesWithTag(tag).fetchSemanticsNodes().isNotEmpty()

    private fun waitFor(tag: String) = compose.waitUntil(TIMEOUT_MILLIS) { exists(tag) }

    private fun waitGone(tag: String) = compose.waitUntil(TIMEOUT_MILLIS) { !exists(tag) }

    /** 起動の画面が出てから、同意の画面を出す 0.5 秒より長く待っても出ないこと */
    private fun assertNoConsentOnLaunch() {
        waitFor("settings.open")
        SystemClock.sleep(CONSENT_SETTLE_MILLIS)
        compose.waitForIdle()
        assertFalse(exists("consent.sheet"))
    }

    private fun openSettings() {
        compose.onNodeWithTag("settings.open").performClick()
        waitFor("settings.sheet")
    }

    /** 話しかける画面。作ったアプリを見ている時はマイクの FAB から開く(初回の画面はそのまま出ている) */
    private fun openVoice() {
        if (exists("voice.fab")) compose.onNodeWithTag("voice.fab").performClick()
        waitFor("voice.composer")
    }

    private fun status() = compose.onNodeWithTag("settings.aiConsent.status").performScrollTo()

    private companion object {
        /** 起動・モックの組み立て・同意の画面の 0.5 秒・シートの開閉まで */
        const val TIMEOUT_MILLIS = 15_000L

        /** 同意の画面が出ないことを確かめる前に待つ長さ(出すのは起動から 0.5 秒後) */
        const val CONSENT_SETTLE_MILLIS = 1_500L

        /** 送る前に止まるので、文の中身は何でもよい(「別のアプリ」などの端末で拾う発話は避ける) */
        const val UTTERANCE = "家計簿アプリが欲しい"
    }
}
