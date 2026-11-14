package jp.kz2601.metamon.renderer.components.media

import jp.kz2601.metamon.dsl.Source

/** タップした時に開くもの(iOS の `PhotoCapturePlan`)。 */
enum class PhotoCapturePlan { CAMERA, LIBRARY, CHOOSE }

/** `photoField.source`(camera | library | both・省略や知らない値は both)と端末のカメラの有無から開き方を決める。 */
object PhotoSourcePolicy {
    fun plan(source: Source?, cameraAvailable: Boolean): PhotoCapturePlan {
        if (!cameraAvailable) return PhotoCapturePlan.LIBRARY
        return when (source) {
            Source.CAMERA -> PhotoCapturePlan.CAMERA
            Source.LIBRARY -> PhotoCapturePlan.LIBRARY
            else -> PhotoCapturePlan.CHOOSE
        }
    }
}

/**
 * `photoField.illustrate`(写真を絵にする)。iOS は端末内の Image Playground で作り、使えない端末ではボタンを出さない。
 * Android には同じ仕組みが無いので、今は使えない端末と同じ扱い(ボタンを出さない・写真の入力として動く)。
 */
object PhotoIllustrate {
    const val AVAILABLE = false

    fun showsButton(requested: Boolean?, available: Boolean = AVAILABLE): Boolean = requested == true && available
}
