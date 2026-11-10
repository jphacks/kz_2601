package jp.kz2601.metamon.services.social

import jp.kz2601.metamon.dsl.records.MatchBox
import jp.kz2601.metamon.dsl.records.MatchInterest

/** 仲間探しの API(appconfig の `matching` タグ・#507)。`ageConfirmed` は本人の「18 歳以上」の確認(年齢そのものは送らない)。 */
interface MatchingClient {
    /** `POST /v1/apps/{appId}/interests`。相手が送っていればその場で matched。 */
    suspend fun sendInterest(appId: String, recordId: String, fromRecordId: String, ageConfirmed: Boolean): MatchInterest

    /** `GET /v1/apps/{appId}/interests?box=` / `GET /v1/apps/{appId}/matches` */
    suspend fun list(appId: String, box: MatchBox): List<MatchInterest>

    /** `POST /v1/apps/{appId}/interests/{uid}/accept` */
    suspend fun accept(appId: String, uid: String, ageConfirmed: Boolean): MatchInterest

    /** `DELETE /v1/apps/{appId}/interests/{uid}`: 届いたものを見送る / 送ったものを取り下げる(冪等・相手に知らせない) */
    suspend fun decline(appId: String, uid: String)

    /** `DELETE /v1/apps/{appId}/matches/{uid}` */
    suspend fun unmatch(appId: String, uid: String)
}

/** API の `details.reason`(400)。 */
object MatchingReason {
    /** 18 歳以上の確認が無い */
    const val AGE_UNCONFIRMED = "age_unconfirmed"

    /** 自分がブロックしている相手には送れない */
    const val BLOCKED = "blocked"
}
