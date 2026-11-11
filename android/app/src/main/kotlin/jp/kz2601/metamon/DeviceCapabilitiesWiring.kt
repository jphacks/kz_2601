package jp.kz2601.metamon

import jp.kz2601.metamon.renderer.registry.ComponentRegistry
import jp.kz2601.metamon.renderer.registry.DeviceCapabilities
import jp.kz2601.metamon.services.ClientCapabilities

/** 描画器の登録表から作った申告を ai の送る形にする(#845)。 */
internal fun deviceCapabilities(registry: ComponentRegistry = ComponentRegistry.standard): ClientCapabilities =
    DeviceCapabilities.of(registry).let {
        ClientCapabilities(
            allowedComponents = it.allowedComponents,
            disabledFeatures = it.disabledFeatures,
            supportedFeatures = it.supportedFeatures,
        )
    }
