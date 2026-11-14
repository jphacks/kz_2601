package jp.kz2601.metamon.renderer.components.map

import android.content.Context
import android.location.Address
import android.location.Geocoder
import android.os.Build
import jp.kz2601.metamon.renderer.components.location.LocationButtonRules
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.asExecutor
import kotlinx.coroutines.suspendCancellableCoroutine
import kotlinx.coroutines.withTimeoutOrNull
import kotlin.coroutines.resume

/** 検索で見つけた場所。名前はピン・現在地では null。 */
data class FoundPlace(val name: String?, val address: String?, val at: LatLng)

/**
 * locationPicker の検索(端末の Geocoder・API キー不要)。地名・住所はよく引けるが、店名(POI)は端末と Geocoder の実装次第で
 * 弱い(iOS の MKLocalSearch の代わりにはならない)。使えない端末では空。
 */
object PlaceGeocoder {
    private const val MAX_RESULTS = 8

    suspend fun search(context: Context, query: String): List<FoundPlace> {
        val q = query.trim()
        if (q.isEmpty() || !Geocoder.isPresent()) return emptyList()
        val geocoder = Geocoder(context)
        val found: List<Address>? = try {
            withTimeoutOrNull(LocationButtonRules.PROVIDER_TIMEOUT_MS) {
                suspendCancellableCoroutine<List<Address>?> { cont ->
                    if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.TIRAMISU) {
                        geocoder.getFromLocationName(
                            q,
                            MAX_RESULTS,
                            object : Geocoder.GeocodeListener {
                                override fun onGeocode(addresses: MutableList<Address>) {
                                    if (cont.isActive) cont.resume(addresses)
                                }

                                override fun onError(errorMessage: String?) {
                                    if (cont.isActive) cont.resume(null)
                                }
                            },
                        )
                    } else {
                        Dispatchers.IO.asExecutor().execute {
                            val r = try {
                                @Suppress("DEPRECATION")
                                geocoder.getFromLocationName(q, MAX_RESULTS).orEmpty()
                            } catch (_: Exception) {
                                null
                            }
                            if (cont.isActive) cont.resume(r)
                        }
                    }
                }
            }
        } catch (e: CancellationException) {
            throw e
        } catch (_: Exception) {
            null
        }
        return found.orEmpty().mapNotNull { a ->
            if (!a.hasLatitude() || !a.hasLongitude()) return@mapNotNull null
            val line = a.getAddressLine(0)?.takeIf { it.isNotBlank() }
            val name = a.featureName?.takeIf { it.isNotBlank() && line?.startsWith(it) != true }
            FoundPlace(name, line, LatLng(a.latitude, a.longitude))
        }.distinctBy { MapRules.value(it.at) }
    }
}
