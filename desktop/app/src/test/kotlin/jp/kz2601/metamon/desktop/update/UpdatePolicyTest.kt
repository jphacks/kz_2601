package jp.kz2601.metamon.desktop.update

import jp.kz2601.metamon.desktop.net.ClientPlatform
import org.junit.Assert.assertEquals
import org.junit.Test

class UpdatePolicyTest {
    private val windows = """{"minWindowsBuild":10,"latestWindowsBuild":20,"minLinuxBuild":30,"latestLinuxBuild":40}"""

    @Test
    fun windowsTable() {
        val cases = listOf(
            "9" to UpdateDecision.Required(10),
            "10" to UpdateDecision.Recommended(20),
            "19" to UpdateDecision.Recommended(20),
            "20" to UpdateDecision.None,
            "21" to UpdateDecision.None,
        )
        for ((build, expected) in cases) {
            assertEquals("build $build", expected, UpdatePolicy.decide(windows, ClientPlatform.WINDOWS, build))
        }
    }

    @Test
    fun linuxUsesItsOwnKeys() {
        assertEquals(UpdateDecision.Required(30), UpdatePolicy.decide(windows, ClientPlatform.LINUX, "25"))
        assertEquals(UpdateDecision.Recommended(40), UpdatePolicy.decide(windows, ClientPlatform.LINUX, "30"))
        assertEquals(UpdateDecision.None, UpdatePolicy.decide(windows, ClientPlatform.LINUX, "40"))
    }

    @Test
    fun noValuesMeansNone() {
        assertEquals(UpdateDecision.None, UpdatePolicy.decide("{}", ClientPlatform.WINDOWS, "1"))
        assertEquals(UpdateDecision.None, UpdatePolicy.decide(null as String?, ClientPlatform.WINDOWS, "1"))
        // Android の値だけ来ても見ない
        assertEquals(UpdateDecision.None, UpdatePolicy.decide("""{"minAndroidBuild":99}""", ClientPlatform.LINUX, "1"))
    }

    @Test
    fun malformedMeansNone() {
        for (body in listOf("not json", "[]", """{"minWindowsBuild":"10"}""", """{"minWindowsBuild":-1}""", """{"minWindowsBuild":0}""", """{"minWindowsBuild":1.5}""")) {
            assertEquals(body, UpdateDecision.None, UpdatePolicy.decide(body, ClientPlatform.WINDOWS, "1"))
        }
        // 自分の版が読めない
        for (build in listOf("", "1..2", "1.a", "-1", "dev")) {
            assertEquals(build, UpdateDecision.None, UpdatePolicy.decide(windows, ClientPlatform.WINDOWS, build))
        }
    }

    @Test
    fun otherOsMeansNone() {
        assertEquals(UpdateDecision.None, UpdatePolicy.decide(windows, ClientPlatform.MACOS, "1"))
    }

    @Test
    fun dottedBuildsCompareLeftToRight() {
        assertEquals(UpdateDecision.None, UpdatePolicy.decide("""{"minWindowsBuild":10}""", ClientPlatform.WINDOWS, "10.0.1"))
        assertEquals(UpdateDecision.Required(10), UpdatePolicy.decide("""{"minWindowsBuild":10}""", ClientPlatform.WINDOWS, "9.9.9"))
        assertEquals(UpdateDecision.None, UpdatePolicy.decide("""{"minWindowsBuild":10}""", ClientPlatform.WINDOWS, "10.0"))
    }
}
