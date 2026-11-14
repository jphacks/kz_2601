package jp.kz2601.metamon.services.discover

import jp.kz2601.metamon.services.InstallResult
import kotlinx.serialization.Serializable

/** 「探す」の掲載(appconfig `PublicApp`)。[id] は掲載者の appId。 */
@Serializable
data class PublicApp(
    val id: String,
    val name: String = "",
    val description: String = "",
    val tags: List<String> = emptyList(),
    val authorName: String = "",
    /** 使っている部品の type(カードのアイコン用・最大 8) */
    val components: List<String> = emptyList(),
    val dslVersion: Int? = null,
    val installs: Int = 0,
    val mine: Boolean = false,
    val createdAt: String? = null,
    val updatedAt: String? = null,
    /** 運営が非公開にした(掲載者本人にだけ付く・#321) */
    val hidden: Boolean? = null,
    val hiddenReason: String? = null,
)

@Serializable
data class PublicAppPage(val items: List<PublicApp> = emptyList(), val nextPageToken: String? = null)

/** 並び順(`sort`)。popular = インストール数の多い順、new = 更新の新しい順。 */
enum class DiscoverSort(val wire: String) { POPULAR("popular"), NEW("new") }

/** `POST /apps/{appId}/publish` の本文。null のキーは送らない(サーバーが DSL の名前・説明・「匿名」を使う)。 */
data class PublishRequest(
    val name: String? = null,
    val description: String? = null,
    val tags: List<String> = emptyList(),
    val authorName: String? = null,
)

/**
 * 「探す」(appconfig の `sharing` タグ・#902)。iOS の `SharingClient` の検索・インストール・公開・非公開。
 * 契約: docs/backend/openapi-appconfig.yaml の `/public-apps/search`・`/public-apps/{id}/install`・`/apps/{appId}/publish`。
 */
interface DiscoverClient {
    /** `POST /v1/public-apps/search`(検索語を URL に載せない)。 */
    suspend fun search(query: String, sort: DiscoverSort, pageToken: String? = null): PublicAppPage

    /** `POST /v1/public-apps/{id}/install`。2 回目以降は最初に入れたアプリ(`created: false`)。 */
    suspend fun install(publicAppId: String): InstallResult

    /** `POST /v1/apps/{appId}/publish`(作成者だけ・再実行で更新)。 */
    suspend fun publish(appId: String, request: PublishRequest): PublicApp

    /** `DELETE /v1/apps/{appId}/publish`(掲載者だけ)。 */
    suspend fun unpublish(appId: String)
}
