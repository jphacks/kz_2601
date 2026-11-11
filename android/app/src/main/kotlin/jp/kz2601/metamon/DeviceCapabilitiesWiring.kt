package jp.kz2601.metamon

import jp.kz2601.metamon.renderer.registry.ComponentRegistry
import jp.kz2601.metamon.renderer.registry.DeviceCapabilities
import jp.kz2601.metamon.services.ClientCapabilities

/** 描画器の登録表から作った申告を ai の送る形にする(#845)。審査用の版は共有の記録も使わせない(#1275)。 */
internal fun deviceCapabilities(
    registry: ComponentRegistry = ComponentRegistry.standard,
    review: Boolean = ReviewBuild.enabled,
): ClientCapabilities =
    DeviceCapabilities.of(registry).let {
        ClientCapabilities(
            allowedComponents = it.allowedComponents,
            disabledFeatures = ReviewBuild.disabledFeatures(it.disabledFeatures, review),
            supportedFeatures = it.supportedFeatures,
        )
    }
