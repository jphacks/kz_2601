package jp.kz2601.metamon.services.follows

/**
 * フォロー(#270・iOS の `FollowsClient`)。アプリごとに、サインイン中の人が誰をフォローしているか
 * (appconfig の `/v1/apps/{appId}/follows`)。フォローは本人だけが見られ、相手に知らせない。
 */
interface FollowsClient {
    /** `GET /v1/apps/{appId}/follows`: フォローしている人の uid。 */
    suspend fun list(appId: String): List<String>

    /** `PUT` / `DELETE /v1/apps/{appId}/follows/{uid}`(冪等)。操作の後にフォローしているかを返す。 */
    suspend fun setFollow(appId: String, uid: String, on: Boolean): Boolean
}

/** API の `details.reason`。 */
object FollowsReason {
    /** 1 アプリでフォローできる人数(500)を超えた */
    const val FULL = "follows_full"

    /** 参加者だけのアプリで、相手が参加者でない */
    const val NOT_IN_APP = "not_in_app"
}
