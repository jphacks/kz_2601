package jp.kz2601.metamon.feedback

import android.content.Context
import android.content.Intent
import androidx.compose.ui.test.assertAll
import androidx.compose.ui.test.assertIsDisplayed
import androidx.compose.ui.test.assertIsEnabled
import androidx.compose.ui.test.assertIsNotEnabled
import androidx.compose.ui.test.assertIsOff
import androidx.compose.ui.test.isOff
import androidx.compose.ui.test.junit4.createEmptyComposeRule
import androidx.compose.ui.test.onAllNodesWithTag
import androidx.compose.ui.test.onAllNodesWithText
import androidx.compose.ui.test.onNodeWithTag
import androidx.compose.ui.test.onNodeWithText
import androidx.compose.ui.test.performClick
import androidx.compose.ui.test.performScrollTo
import androidx.compose.ui.test.performTextInput
import androidx.test.core.app.ActivityScenario
import androidx.test.core.app.ApplicationProvider
import androidx.test.ext.junit.runners.AndroidJUnit4
import jp.kz2601.metamon.MainActivity
import jp.kz2601.metamon.MetamonApplication
import jp.kz2601.metamon.R
import jp.kz2601.metamon.services.feedback.FeedbackConsent
import jp.kz2601.metamon.services.feedback.MockFeedbackClient
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Before
import org.junit.Rule
import org.junit.Test
import org.junit.runner.RunWith

/**
 * フィードバックの画面(#906)の画面のテスト(#1314)。モックの構成(`METAMON_MOCK=1`)で起動し、
 * `METAMON_AUTO_FEEDBACK=<秒>`(振った時と同じく開く口・debug だけ)で画面を開く。送り先はモック([MockFeedbackClient])。
 * - 既定ではどの添付もオフ
 * - 同意を入れるまで「送信」を押せない
 * - 送れたら「送信しました」が出る
 *
 * 端末かエミュレータで `./gradlew :app:connectedStgDebugAndroidTest`。
 */
@RunWith(AndroidJUnit4::class)
class FeedbackComposerUiTest {
    @get:Rule
    val compose = createEmptyComposeRule()

    private val context: Context = ApplicationProvider.getApplicationContext()
    private lateinit var scenario: ActivityScenario<MainActivity>

    @Before
    fun launch() {
        val intent = Intent(context, MainActivity::class.java)
            .putExtra("METAMON_MOCK", "1")
            .putExtra("METAMON_AUTO_FEEDBACK", "1")
        scenario = ActivityScenario.launch(intent)
    }

    @After
    fun close() {
        scenario.close()
    }

    @Test
    fun attachmentsAreOffConsentGatesSendAndSentIsShown() {
        compose.waitUntil(OPEN_TIMEOUT_MILLIS) { compose.onAllNodesWithTag("feedback.composer").fetchSemanticsNodes().isNotEmpty() }

        // 既定ではどの添付もオフ(スクショ・アプリは撮れた・開いている時だけ出る)
        for (tag in listOf("feedback.attachScreenshot", "feedback.attachApp")) {
            compose.onAllNodesWithTag(tag).assertAll(isOff())
        }
        compose.onNodeWithTag("feedback.attachDevice").performScrollTo().assertIsOff()
        compose.onNodeWithTag("feedback.agree").performScrollTo().assertIsOff()

        // 書いただけでは送れない。同意を入れたら送れる
        compose.onNodeWithTag("feedback.message").performScrollTo().performTextInput("ボタンが押せません")
        compose.onNodeWithTag("feedback.send").assertIsNotEnabled()
        compose.onNodeWithTag("feedback.agree").performScrollTo().performClick()
        compose.onNodeWithTag("feedback.send").assertIsEnabled().performClick()

        val sent = context.getString(R.string.feedback_composer_sent_title)
        compose.waitUntil(SEND_TIMEOUT_MILLIS) { compose.onAllNodesWithText(sent).fetchSemanticsNodes().isNotEmpty() }
        compose.onNodeWithText(sent).assertIsDisplayed()

        // モックの送り先に、添付なしで同意の版と一緒に届いた
        val client = (context as MetamonApplication).builtContainer?.feedback as MockFeedbackClient
        val submission = client.sent.single()
        assertEquals("ボタンが押せません", submission.message)
        assertEquals(FeedbackConsent.VERSION, submission.consentVersion)
        assertNull(submission.screenshotJpeg)
        assertNull(submission.app)
        assertNull(submission.device)
    }

    private companion object {
        /** 起動・モックの組み立て・つまみの 1 秒・スクショを撮るまで */
        const val OPEN_TIMEOUT_MILLIS = 15_000L
        const val SEND_TIMEOUT_MILLIS = 5_000L
    }
}
