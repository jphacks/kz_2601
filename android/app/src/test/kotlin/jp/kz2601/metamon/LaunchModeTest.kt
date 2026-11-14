package jp.kz2601.metamon

import org.junit.Assert.assertEquals
import org.junit.Test
import org.w3c.dom.Element
import java.io.File
import javax.xml.parsers.DocumentBuilderFactory

/** 共有リンク・通知は開いている MainActivity の onNewIntent で受ける(2 つ目の画面を積まない)。 */
class LaunchModeTest {
    @Test
    fun mainActivityIsSingleTask() {
        val factory = DocumentBuilderFactory.newInstance().apply { isNamespaceAware = true }
        val doc = factory.newDocumentBuilder().parse(File("src/main/AndroidManifest.xml"))
        val activities = doc.getElementsByTagName("activity")
        val main = (0 until activities.length).map { activities.item(it) as Element }
            .single { it.getAttributeNS(ANDROID_NS, "name") == ".MainActivity" }
        assertEquals("singleTask", main.getAttributeNS(ANDROID_NS, "launchMode"))
    }

    private companion object {
        const val ANDROID_NS = "http://schemas.android.com/apk/res/android"
    }
}
