package jp.kz2601.metamon

import android.content.ComponentCallbacks2
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

@Suppress("DEPRECATION")
class TrimMemoryTest {
    @Test
    fun releasesOnMemoryPressure() {
        assertTrue(MetamonApplication.releasesOnTrim(ComponentCallbacks2.TRIM_MEMORY_RUNNING_LOW))
        assertTrue(MetamonApplication.releasesOnTrim(ComponentCallbacks2.TRIM_MEMORY_RUNNING_CRITICAL))
        assertTrue(MetamonApplication.releasesOnTrim(ComponentCallbacks2.TRIM_MEMORY_BACKGROUND))
        assertTrue(MetamonApplication.releasesOnTrim(ComponentCallbacks2.TRIM_MEMORY_COMPLETE))
    }

    @Test
    fun keepsTheModelWhenOnlyHiddenOrModerate() {
        assertFalse(MetamonApplication.releasesOnTrim(ComponentCallbacks2.TRIM_MEMORY_UI_HIDDEN))
        assertFalse(MetamonApplication.releasesOnTrim(ComponentCallbacks2.TRIM_MEMORY_RUNNING_MODERATE))
    }
}
