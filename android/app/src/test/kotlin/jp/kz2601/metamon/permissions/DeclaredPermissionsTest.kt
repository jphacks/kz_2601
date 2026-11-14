package jp.kz2601.metamon.permissions

import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test

/** マニフェストの権限の読み方が版で分かれること(12 / 12L で PackageInfoFlags を呼ばない)。 */
class DeclaredPermissionsTest {
    private val declared = arrayOf("android.permission.CAMERA")

    @Test
    fun android12UsesTheIntFlagsApi() {
        for (sdk in listOf(31, 32)) {
            val got = PermissionCenter.requestedPermissions(sdk, modern = { error("API 33 の呼び出し") }, legacy = { declared })
            assertEquals(setOf("android.permission.CAMERA"), got)
        }
    }

    @Test
    fun android13AndLaterUsePackageInfoFlags() {
        val got = PermissionCenter.requestedPermissions(33, modern = { declared }, legacy = { error("古い呼び出し") })
        assertEquals(setOf("android.permission.CAMERA"), got)
    }

    @Test
    fun noRequestedPermissionsIsEmpty() {
        assertTrue(PermissionCenter.requestedPermissions(31, modern = { null }, legacy = { null }).isEmpty())
    }
}
