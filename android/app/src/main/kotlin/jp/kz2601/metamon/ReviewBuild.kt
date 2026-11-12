package jp.kz2601.metamon

import jp.kz2601.metamon.dsl.ComponentType

/**
 * Play の審査に出す版(#1275・iOS の `APP_STORE_REVIEW`・docs/ios/review-build.md)。
 * `-Pmetamon.playReview=true` で組むと、共有(共有シート・シェアリンク・参加している人)と「探す」の入口を出さず、
 * 生成に送る申告も iOS の審査用の版と同じだけ外す(`ClientCapabilities.reviewExcludedComponents` /
 * `reviewDisabledFeatures` / 審査用の `supportedFeatures`)。既定はオフで、通常の版は今まで通り。
 */
object ReviewBuild {
    val enabled: Boolean get() = BuildConfig.PLAY_REVIEW

    /** 審査用の版で backend に使わせない機能(iOS の `ClientCapabilities.reviewDisabledFeatures` の共有の分) */
    const val SHARED_DATA = "sharedData"

    /** 審査用の版で使わせない機能(iOS の `reviewDisabledFeatures` と同じ並び) */
    val DISABLED_FEATURES: List<String> = listOf("fieldHealth", "cameraCapture", SHARED_DATA)

    /** 審査用の版で申告しない部品(iOS の `reviewExcludedComponents`・1 行 1 部品) */
    val EXCLUDED_COMPONENTS: Set<String> = setOf(
        ComponentType.AI_CHAT,
        ComponentType.AUDIO_RECORDER,
        ComponentType.CALL_BUTTON,
        ComponentType.CHAT_THREAD,
        ComponentType.DASHCAM,
        ComponentType.FEED,
        ComponentType.HEALTH_STAT,
        ComponentType.LOCATION_TRACKER,
        ComponentType.MATCH_LIST,
        ComponentType.NEARBY_SHARE,
        ComponentType.SENSOR_VALUE,
        ComponentType.WEB_VIEW,
    ).map { it.wire }.toSet()

    /** 審査用の版で送らない supportedFeatures(iOS の審査用の `supportedFeatures` に無いもの・1 行 1 キー) */
    val WITHHELD_FEATURES: Set<String> = setOf(
        "addressedMessages",
        "component.callButton",
        "dataRoles",
        "field.encrypted",
        "matching",
        "photoField.illustrate",
    )

    /** 生成に送る allowedComponents。審査用の版は [EXCLUDED_COMPONENTS] を除く。 */
    fun allowedComponents(base: List<String>, review: Boolean): List<String> =
        if (review) base.filterNot { it in EXCLUDED_COMPONENTS } else base

    /** 生成に送る disabledFeatures。審査用の版は [DISABLED_FEATURES] の無い分を足す。 */
    fun disabledFeatures(base: List<String>, review: Boolean): List<String> =
        if (review) base + DISABLED_FEATURES.filterNot { it in base } else base

    /** 生成に送る supportedFeatures。審査用の版は [WITHHELD_FEATURES] を除く。 */
    fun supportedFeatures(base: List<String>, review: Boolean): List<String> =
        if (review) base.filterNot { it in WITHHELD_FEATURES } else base

    /** 共有の入口を出してよいか。審査用の版は出さない。 */
    fun canShare(review: Boolean, mock: Boolean, mockSharing: Boolean): Boolean = !review && (!mock || mockSharing)
}
