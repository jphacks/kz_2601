package jp.kz2601.metamon.runtime

import jp.kz2601.metamon.runtime.RuntimeFixture.runtime
import kotlinx.coroutines.test.runTest
import org.junit.Assert.assertEquals
import org.junit.Test

/** iOS の RecordRejectedToastTests: 同期で弾かれた時の toast は項目を DSL の名前で出し、サーバーの英語は出さない(H9)。 */
class RecordRejectedToastTest {
    private val dsl = RuntimeFixture.dsl("warikan")

    @Test
    fun rejectedToastNamesTheField() = runTest {
        val text = runtime(dsl).rejectedToast("expenses", "payer", "field \"payer\" is required")
        // 項目は DSL の名前。英語の field id・サーバーの文は引数に入れない
        assertEquals(RuntimeCopy.rejected("払った人"), text)
    }

    @Test
    fun rejectedToastWithoutFieldHidesServerMessage() = runTest {
        val text = runtime(dsl).rejectedToast("expenses", null, "data is required")
        assertEquals(RuntimeCopy.REJECTED_GENERIC, text)
    }

    @Test
    fun rejectedToastUnknownFieldFallsBackToId() = runTest {
        val text = runtime(dsl).rejectedToast("expenses", "gone", "field \"gone\" must be a string")
        assertEquals(RuntimeCopy.rejected("gone"), text)
    }
}
