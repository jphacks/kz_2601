package jp.kz2601.metamon

/**
 * Play の審査に出す版(#1275・iOS の `APP_STORE_REVIEW`・docs/ios/review-build.md)。
 * `-Pmetamon.playReview=true` で組むと、共有(共有シート・シェアリンク・参加している人)と「探す」の入口を出さず、
 * 生成にも `sharedData` を使わせない(backend が scope を private に直す)。既定はオフで、通常の版は今まで通り。
 */
object ReviewBuild {
    val enabled: Boolean get() = BuildConfig.PLAY_REVIEW

    /** 審査用の版で backend に使わせない機能(iOS の `ClientCapabilities.reviewDisabledFeatures` の共有の分) */
    const val SHARED_DATA = "sharedData"

    /** 生成に送る disabledFeatures。審査用の版は [SHARED_DATA] を足す。 */
    fun disabledFeatures(base: List<String>, review: Boolean): List<String> =
        if (review && SHARED_DATA !in base) base + SHARED_DATA else base

    /** 共有の入口を出してよいか。審査用の版は出さない。 */
    fun canShare(review: Boolean, mock: Boolean, mockSharing: Boolean): Boolean = !review && (!mock || mockSharing)
}
