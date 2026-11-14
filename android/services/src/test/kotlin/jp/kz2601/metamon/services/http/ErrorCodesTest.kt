package jp.kz2601.metamon.services.http

import org.junit.Assert.assertEquals
import org.junit.Test
import java.io.File

/** [ErrorCodes] が backend/internal/httpx/codes.go と同じ集合か。 */
class ErrorCodesTest {
    @Test
    fun matchesBackendCodes() {
        val path = System.getProperty("metamon.backendCodesFile") ?: error("metamon.backendCodesFile が無い")
        val backend = Regex("""^const Code\w+ = "([a-z_]+)"$""", RegexOption.MULTILINE)
            .findAll(File(path).readText())
            .map { it.groupValues[1] }
            .toList()
        assertEquals(backend.sorted(), ErrorCodes.all.sorted())
        assertEquals("ErrorCodes.all に重複がある", ErrorCodes.all.size, ErrorCodes.all.toSet().size)
    }
}
