package jp.kz2601.metamon.renderer.components.location

import android.Manifest
import android.content.Context
import android.content.pm.PackageManager
import android.location.Address
import android.location.Geocoder
import android.location.Location
import android.location.LocationManager
import android.os.Build
import android.os.CancellationSignal
import android.os.SystemClock
import androidx.activity.compose.rememberLauncherForActivityResult
import androidx.activity.result.contract.ActivityResultContracts
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.rememberUpdatedState
import androidx.compose.runtime.staticCompositionLocalOf
import androidx.compose.ui.platform.LocalContext
import jp.kz2601.metamon.renderer.components.media.PermissionPrompt
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.suspendCancellableCoroutine
import kotlinx.coroutines.withContext
import kotlinx.coroutines.withTimeoutOrNull
import kotlin.coroutines.resume

/**
 * 位置の権限を聞く窓口。:app が権限の窓口(理由 → OS のダイアログ → 設定へ)で差し替える。
 * 既定は OS のダイアログだけ(細かい / おおよそのどちらかが許されれば使える)。
 */
val LocalLocationPermission = staticCompositionLocalOf<PermissionPrompt> { { onResult -> rememberPlainLocationPrompt(onResult) } }

private val LOCATION_NAMES = arrayOf(Manifest.permission.ACCESS_FINE_LOCATION, Manifest.permission.ACCESS_COARSE_LOCATION)

@Composable
private fun rememberPlainLocationPrompt(onResult: (Boolean) -> Unit): () -> Unit {
    val context = LocalContext.current
    val callback by rememberUpdatedState(onResult)
    val launcher = rememberLauncherForActivityResult(ActivityResultContracts.RequestMultiplePermissions()) { r -> callback(r.values.any { it }) }
    return { if (context.hasLocationPermission()) callback(true) else launcher.launch(LOCATION_NAMES) }
}

/** 細かい / おおよそのどちらかの位置の許可があるか。 */
fun Context.hasLocationPermission(): Boolean = LOCATION_NAMES.any { checkSelfPermission(it) == PackageManager.PERMISSION_GRANTED }

/** 取れた位置か、取れなかった理由。 */
sealed interface LocationFix {
    data class Got(val location: Location) : LocationFix
    data class Problem(val problem: LocationProblem) : LocationFix
}

/** 現在地を 1 回だけ取る(iOS の `OneShotLocation`)。権限は呼ぶ前に聞いておく。使用中だけ・裏では取らない。 */
object OneShotLocation {
    suspend fun request(context: Context): LocationFix {
        if (!context.hasLocationPermission()) return LocationFix.Problem(LocationProblem.DENIED)
        val lm = context.getSystemService(Context.LOCATION_SERVICE) as? LocationManager
            ?: return LocationFix.Problem(LocationProblem.FAILED)
        if (!lm.isLocationEnabled) return LocationFix.Problem(LocationProblem.OFF)
        val fine = context.checkSelfPermission(Manifest.permission.ACCESS_FINE_LOCATION) == PackageManager.PERMISSION_GRANTED
        val providers = LocationButtonRules.providers(lm.getProviders(true), fine)
        if (providers.isEmpty()) return LocationFix.Problem(LocationProblem.OFF)
        return try {
            for (p in providers) {
                val loc = withTimeoutOrNull(LocationButtonRules.PROVIDER_TIMEOUT_MS) { current(context, lm, p) }
                if (loc != null) return LocationFix.Got(loc)
            }
            // どれも時間内に返らなければ、少し前に取れた位置で代える
            val last = providers.mapNotNull { lm.getLastKnownLocation(it) }.maxByOrNull { it.elapsedRealtimeNanos }
            val ageMs = last?.let { (SystemClock.elapsedRealtimeNanos() - it.elapsedRealtimeNanos) / 1_000_000 }
            if (last != null && ageMs != null && LocationButtonRules.freshEnough(ageMs)) LocationFix.Got(last)
            else LocationFix.Problem(LocationProblem.FAILED)
        } catch (_: SecurityException) {
            LocationFix.Problem(LocationProblem.DENIED)
        }
    }

    private suspend fun current(context: Context, lm: LocationManager, provider: String): Location? =
        suspendCancellableCoroutine { cont ->
            val signal = CancellationSignal()
            cont.invokeOnCancellation { signal.cancel() }
            try {
                lm.getCurrentLocation(provider, signal, context.mainExecutor) { if (cont.isActive) cont.resume(it) }
            } catch (_: IllegalArgumentException) {
                if (cont.isActive) cont.resume(null)
            }
        }

    /** 座標を住所にする(端末の Geocoder)。使えない・見つからない時は null(打った住所は消さない)。 */
    suspend fun address(context: Context, lat: Double, lng: Double): String? {
        if (!Geocoder.isPresent()) return null
        val geocoder = Geocoder(context)
        val line = try {
            if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.TIRAMISU) {
                withTimeoutOrNull(LocationButtonRules.PROVIDER_TIMEOUT_MS) {
                    suspendCancellableCoroutine { cont ->
                        geocoder.getFromLocation(
                            lat,
                            lng,
                            1,
                            object : Geocoder.GeocodeListener {
                                override fun onGeocode(addresses: MutableList<Address>) {
                                    if (cont.isActive) cont.resume(addresses.firstOrNull()?.getAddressLine(0))
                                }

                                override fun onError(errorMessage: String?) {
                                    if (cont.isActive) cont.resume(null)
                                }
                            },
                        )
                    }
                }
            } else {
                withContext(Dispatchers.IO) {
                    @Suppress("DEPRECATION")
                    geocoder.getFromLocation(lat, lng, 1)?.firstOrNull()?.getAddressLine(0)
                }
            }
        } catch (_: Exception) {
            null
        }
        return line?.takeIf { it.isNotBlank() }
    }
}
