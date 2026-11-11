package jp.kz2601.metamon.runtime

import jp.kz2601.metamon.runtime.RuntimeFixture.runtime
import kotlinx.coroutines.test.runTest
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

/** iOS の RecordRejectedToastTests: 同期で弾かれた時の toast は項目を DSL の名前で出し、サーバーの英語は出さない(H9)。 */
class RecordRejectedToastTest {
    private val dsl = RuntimeFixture.dsl("warikan")

    @Test
    fun rejectedToastNamesTheField() = runTest {
        val text = runtime(dsl).rejectedToast("expenses", "payer", "field \"payer\" is required")
        assertTrue(text, text.contains("払った人"))
        assertFalse("英語の field id は見せない: $text", text.contains("\"payer\""))
        assertFalse("サーバーの文は見せない: $text", text.contains("is required"))
    }

    @Test
    fun rejectedToastWithoutFieldHidesServerMessage() = runTest {
        val text = runtime(dsl).rejectedToast("expenses", null, "data is required")
        assertFalse(text, text.contains("data is required"))
        assertEquals("保存できませんでした。入力を確認して、もう一度お試しください", text)
    }

    @Test
    fun rejectedToastUnknownFieldFallsBackToId() = runTest {
        val text = runtime(dsl).rejectedToast("expenses", "gone", "field \"gone\" must be a string")
        assertTrue(text, text.contains("gone"))
    }
}
