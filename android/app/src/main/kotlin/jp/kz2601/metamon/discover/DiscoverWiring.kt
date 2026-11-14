package jp.kz2601.metamon.discover

import androidx.compose.runtime.staticCompositionLocalOf
import jp.kz2601.metamon.services.discover.DiscoverClient
import jp.kz2601.metamon.services.social.AuthorProfile
import jp.kz2601.metamon.services.social.ModerationClient

/** 「探す」の API と、掲載の通報・ブロック(#902)。モック(サーバーにつながない構成)は null。 */
class DiscoverServices(val client: DiscoverClient, val moderation: ModerationClient?)

/** 共有シートの「みんなに公開」が使うもの。null = 公開の欄を出さない(シェアが使えない構成)。 */
class PublishEnv(val client: DiscoverClient, val profile: AuthorProfile?)

/** 共有シートの呼び出し側が渡す(共有シートのファイルは入口の 1 行だけにするため)。 */
val LocalPublishEnv = staticCompositionLocalOf<PublishEnv?> { null }
