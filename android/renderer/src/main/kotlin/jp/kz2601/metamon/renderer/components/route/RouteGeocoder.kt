package jp.kz2601.metamon.renderer.components.route

import android.content.Context
import android.location.Address
import android.location.Geocoder
import android.os.Build
import jp.kz2601.metamon.renderer.components.location.LocationButtonRules
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.suspendCancellableCoroutine
import kotlinx.coroutines.withContext
import kotlinx.coroutines.withTimeoutOrNull
import kotlin.coroutines.resume

/** 住所を緯度経度にする(端末の Geocoder・#1350)。 */
object RouteGeocoder {
    suspend fun lookup(context: Context, address: String): AddressLookup {
        if (!Geocoder.isPresent()) return AddressLookup.Unavailable
        val geocoder = Geocoder(context)
        val found: List<Address>? = try {
            withTimeoutOrNull(LocationButtonRules.PROVIDER_TIMEOUT_MS) {
                if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.TIRAMISU) {
                    suspendCancellableCoroutine<List<Address>?> { cont ->
                        geocoder.getFromLocationName(
                            address,
                            1,
                            object : Geocoder.GeocodeListener {
                                override fun onGeocode(addresses: MutableList<Address>) {
                                    if (cont.isActive) cont.resume(addresses)
                                }

                                override fun onError(errorMessage: String?) {
                                    if (cont.isActive) cont.resume(null)
                                }
                            },
                        )
                    }
                } else {
                    withContext(Dispatchers.IO) {
                        @Suppress("DEPRECATION")
                        geocoder.getFromLocationName(address, 1).orEmpty()
                    }
                }
            }
        } catch (_: Exception) {
            null
        }
        return AddressLookup.of(found?.map { if (it.hasLatitude() && it.hasLongitude()) it.latitude to it.longitude else null })
    }
}
