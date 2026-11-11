package jp.kz2601.metamon.settings

import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

/** #898 / #1240 「詳細(サポート用)」: 問い合わせに要る値(ユーザー ID を含む)だけを出し、トークン・メールアドレスなどの秘密は入れない。 */
class SupportInfoTest {
    private val info = SupportInfo(
        uid = "uid-123",
        voiceModel = "AndroidSpeechTranscriber",
        aiEndpoint = SupportInfo.host("https://ai-stg-1.asia-northeast1.run.app"),
        appConfigEndpoint = SupportInfo.host("https://appconfig-stg-1.asia-northeast1.run.app/v1"),
        appVersion = "0.1.0-stg",
        build = "1",
        appRevision = 5,
    )

    @Test
    fun rowsHoldEveryValueInOrder() {
        assertEquals(
            listOf("uid-123", "AndroidSpeechTranscriber", "ai-stg-1.asia-northeast1.run.app", "appconfig-stg-1.asia-northeast1.run.app", "0.1.0-stg (1)", "v5"),
            info.rows.map { it.value },
        )
    }

    @Test
    fun clipboardHasOneLinePerRow() {
        val lines = info.clipboardText { it.name }.split("\n")
        assertEquals(info.rows.size, lines.size)
        assertEquals("USER_ID: uid-123", lines[0])
        assertEquals("VERSION: 0.1.0-stg (1)", lines[4])
    }

    @Test
    fun endpointKeepsOnlyTheHost() {
        // 接続先にユーザー情報・パス・クエリ(鍵など)が混じっていても、ホスト名しか出さない
        assertEquals("example.com", SupportInfo.host("https://alice%40example.org:token123@example.com/v1/apps?key=secret#frag"))
        assertEquals("live", SupportInfo.host("not a url"))
        assertEquals("live", SupportInfo.host(""))
    }

    @Test
    fun noSecretsInRowsOrClipboard() {
        val leaky = SupportInfo(
            uid = null,
            voiceModel = "m",
            aiEndpoint = SupportInfo.host("https://uid-abc:Bearer.eyJtoken@ai.example.com/path?uid=abc123&token=eyJ"),
            appConfigEndpoint = SupportInfo.host("https://appconfig.example.com/me?email=alice%40example.org"),
            appVersion = "1.0",
            build = "2",
            appRevision = 0,
        )
        val text = leaky.clipboardText { it.name } + leaky.rows.joinToString { it.value }
        for (secret in listOf("abc123", "eyJ", "token", "Bearer", "@", "alice", "uid")) {
            assertFalse("$secret が出ている: $text", text.contains(secret))
        }
        // メールアドレス・トークンの欄は無い
        assertTrue(SupportInfo.Field.entries.none { f -> listOf("EMAIL", "TOKEN").any { f.name.contains(it) } })
    }

    @Test
    fun userIdRowOnlyWhenSignedIn() {
        // iOS と同じく先頭に出す(#1240)。サインインしていない・空なら行ごと出さない
        assertEquals(SupportInfo.Field.USER_ID, info.rows.first().field)
        assertFalse(info.copy(uid = null).rows.any { it.field == SupportInfo.Field.USER_ID })
        assertFalse(info.copy(uid = "").rows.any { it.field == SupportInfo.Field.USER_ID })
    }
}
