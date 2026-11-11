package jp.kz2601.metamon.renderer.components.scan

import android.content.Context
import androidx.compose.runtime.staticCompositionLocalOf
import com.google.android.gms.common.ConnectionResult
import com.google.android.gms.common.GoogleApiAvailabilityLight
import com.google.mlkit.vision.barcode.common.Barcode
import com.google.mlkit.vision.codescanner.GmsBarcodeScannerOptions
import com.google.mlkit.vision.codescanner.GmsBarcodeScanning

/** QR を読む仕組み。差し替え口(プレビュー・テスト・別の読み取り)。 */
interface QrScanEngine {
    /** この端末でカメラから読めるか。false なら手入力の欄だけ出す。 */
    fun isAvailable(context: Context): Boolean

    /** 読み取りの画面を出し、1 件読んだら(閉じたら)[onResult] を 1 回呼ぶ。 */
    fun scan(context: Context, onResult: (QrScanResult) -> Unit)
}

/**
 * Google Code Scanner(Play 開発者サービスの画面で読む)。カメラはサービスが使うのでアプリは CAMERA の権限を聞かない。
 * 読み取りのモジュールは初回に落とす(落とせなければ [QrScanResult.Failed])。
 */
object GmsQrScanEngine : QrScanEngine {
    // iOS の metadataObjectTypes と同じ種類
    private val options = GmsBarcodeScannerOptions.Builder()
        .setBarcodeFormats(
            Barcode.FORMAT_QR_CODE,
            Barcode.FORMAT_AZTEC,
            Barcode.FORMAT_CODE_39,
            Barcode.FORMAT_CODE_128,
            Barcode.FORMAT_DATA_MATRIX,
            Barcode.FORMAT_EAN_8,
            Barcode.FORMAT_EAN_13,
            Barcode.FORMAT_PDF417,
        )
        .enableAutoZoom()
        .build()

    override fun isAvailable(context: Context): Boolean =
        runCatching { GoogleApiAvailabilityLight.getInstance().isGooglePlayServicesAvailable(context) == ConnectionResult.SUCCESS }
            .getOrDefault(false)

    override fun scan(context: Context, onResult: (QrScanResult) -> Unit) {
        runCatching {
            GmsBarcodeScanning.getClient(context, options).startScan()
                .addOnSuccessListener { onResult(QrScanResult.Found(it.rawValue)) }
                .addOnCanceledListener { onResult(QrScanResult.Canceled) }
                .addOnFailureListener { onResult(QrScanResult.Failed) }
        }.onFailure { onResult(QrScanResult.Failed) }
    }
}

/** 描画に使う読み取り。既定は [GmsQrScanEngine]。 */
val LocalQrScanEngine = staticCompositionLocalOf<QrScanEngine> { GmsQrScanEngine }
