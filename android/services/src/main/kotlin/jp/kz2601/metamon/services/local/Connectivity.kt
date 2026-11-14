package jp.kz2601.metamon.services.local

import android.content.Context
import android.net.ConnectivityManager
import android.net.Network
import android.net.NetworkCapabilities
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow

/** ネットワークの到達性(iOS の `ConnectivityMonitor`)。オンライン復帰で送信待ちを流すために使う。 */
interface ConnectivityMonitor {
    /** 今の値を持ち、変わるたびに流す。 */
    val online: StateFlow<Boolean>
    val isOnline: Boolean get() = online.value
}

/**
 * `ConnectivityManager` の既定のネットワークを見る(iOS の `NWPathMonitor` 実装)。
 * インターネットに出られる(`NET_CAPABILITY_VALIDATED`)時をオンラインとする。
 */
class AndroidConnectivity(context: Context) : ConnectivityMonitor {
    private val manager = context.getSystemService(ConnectivityManager::class.java)
    private val state = MutableStateFlow(currentlyOnline())
    override val online: StateFlow<Boolean> = state.asStateFlow()

    private val callback = object : ConnectivityManager.NetworkCallback() {
        override fun onCapabilitiesChanged(network: Network, caps: NetworkCapabilities) {
            state.value = caps.isUsable()
        }

        override fun onLost(network: Network) {
            state.value = false
        }

        override fun onUnavailable() {
            state.value = false
        }
    }

    init {
        manager.registerDefaultNetworkCallback(callback)
    }

    fun close() = runCatching { manager.unregisterNetworkCallback(callback) }

    private fun currentlyOnline(): Boolean =
        manager.activeNetwork?.let(manager::getNetworkCapabilities)?.isUsable() ?: false

    private fun NetworkCapabilities.isUsable(): Boolean =
        hasCapability(NetworkCapabilities.NET_CAPABILITY_INTERNET) && hasCapability(NetworkCapabilities.NET_CAPABILITY_VALIDATED)
}

/** テスト用: 手で切り替える到達性(iOS の `ManualConnectivity`)。 */
class ManualConnectivity(online: Boolean = true) : ConnectivityMonitor {
    private val state = MutableStateFlow(online)
    override val online: StateFlow<Boolean> = state.asStateFlow()

    fun set(online: Boolean) {
        state.value = online
    }
}
