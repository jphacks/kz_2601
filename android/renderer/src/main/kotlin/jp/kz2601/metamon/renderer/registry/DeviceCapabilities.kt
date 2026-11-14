package jp.kz2601.metamon.renderer.registry

import jp.kz2601.metamon.dsl.ComponentType
import jp.kz2601.metamon.renderer.components.media.PhotoIllustrate

/**
 * generate / chat に送る「この端末が描ける部品・使える機能」(iOS の `ClientCapabilities`・#245・#302・#482)。
 * 部品は [ComponentRegistry] から導く(登録の無い部品 = 「この端末では使えません」の枠 = 申告しない)。
 * 契約: docs/backend/openapi-ai.yaml・backend/internal/ai/restrict.go / client_features.go。
 */
data class DeviceCapabilities(
    /** 描ける部品の type(container + leaf)。iOS と同じく毎回全部送る(後の版で増えた部品をこの版に出させない) */
    val allowedComponents: List<String>,
    /** 使えない機能(backend の disabledFeatures) */
    val disabledFeatures: List<String>,
    /** 部品以外で実装している DSL の動作(backend の supportedFeatures) */
    val supportedFeatures: List<String>,
) {
    /** supportedFeatures の 1 つ。[needs] の部品が描けない時は送らない。[implemented] は Android の実装の有無。 */
    internal class Feature(val key: String, val implemented: Boolean, val needs: ComponentType? = null)

    companion object {
        /** Field.health(健康のデータへの書き込み)。Health Connect で入れた(#870)ので通常の版では外さない。 */
        const val FIELD_HEALTH = "fieldHealth"

        /**
         * iOS の通常の版が送る全キー(並びも同じ)と、Android での有無。無いものは backend が AI に使わせない。
         * 有無を変えたら iOS の ClientCapabilities.swift のコメントと揃える。
         */
        internal val features: List<Feature> = listOf(
            Feature("recordRetention", implemented = true), // RecordRetentionRunner・確認の保存(#482)
            // 仲間探し(#877): matchList・list の「つながりたい」・通報 / ブロック。iOS の通常の版と同じく送る
            Feature("matching", implemented = true, needs = ComponentType.MATCH_LIST),
            Feature("notification.when", implemented = true), // ConditionNotifications
            Feature("singleEntity", implemented = true), // ScreenForm・{"setting"}
            Feature("dataRoles", implemented = true), // member の選択(ChoiceLeaves)・visibleTo のボタン(ButtonLeaf・#1244)
            Feature("addressedMessages", implemented = true), // members の選択(ChoiceLeaves)
            // 場所の通知: Geofencing で見張る(LocalNotifications・正確な位置と裏での位置の許可が要る)
            Feature("notification.near", implemented = true),
            Feature("aiButton.long", implemented = true, needs = ComponentType.AI_BUTTON),
            Feature("aiButton.inputPhoto", implemented = true, needs = ComponentType.AI_BUTTON),
            Feature("aiButton.saveTo", implemented = true, needs = ComponentType.AI_BUTTON),
            Feature("aiButton.inputFeeds", implemented = true, needs = ComponentType.AI_BUTTON), // AiFeedInput
            Feature("aiButton.outputs", implemented = true, needs = ComponentType.AI_BUTTON), // AiOutputs・AiButtonRun.fillOutputs
            Feature("action.lookupBook", implemented = true), // BookLookupPlan
            Feature("text.highlight", implemented = true, needs = ComponentType.TEXT), // TextLeaf・{{fieldId}}・field の語
            Feature("feed.v2", implemented = true, needs = ComponentType.FEED), // warnings / pressure
            Feature("expr.addMinutes", implemented = true), // ExprEvaluator
            Feature("aiChat.inputEntity", implemented = true, needs = ComponentType.AI_CHAT),
            Feature("component.bracket", implemented = true, needs = ComponentType.BRACKET),
            Feature("progressBar.settingTarget", implemented = true, needs = ComponentType.PROGRESS_BAR),
            Feature("photoField.illustrate", implemented = PhotoIllustrate.AVAILABLE, needs = ComponentType.PHOTO_FIELD),
            // 以下 2 つは iOS だけ(Android の部品・暗号化がまだ)
            Feature("component.callButton", implemented = false, needs = ComponentType.CALL_BUTTON),
            Feature("field.encrypted", implemented = false),
            Feature("openUrlButton.app", implemented = true, needs = ComponentType.OPEN_URL_BUTTON), // OpenUrlTarget・shareTo x / mail
        )

        /**
         * 描けるが、iOS と同じ体験がそろうまで申告しない部品(今は無い)。
         * nearbyShare は送受信(#1382)とすれ違い(mode:passby・#1383)がそろったので申告する。
         */
        internal val PENDING: Set<ComponentType> = emptySet()

        /**
         * 代わりの表示だけを登録した部品(#351 の C 区分・1 行 1 部品)。共有で届いた時は代わりを出すが、AI には作らせない。
         * musicPlayer: 再生はせず、曲名で音楽アプリの検索を開く(#879)。
         */
        internal val FALLBACK_ONLY: Set<ComponentType> = setOf(
            ComponentType.MUSIC_PLAYER,
        )

        /** [registry] で描ける部品(UNKNOWN・[PENDING]・[FALLBACK_ONLY] を除く・schema の並び)。 */
        fun allowedComponents(registry: ComponentRegistry = ComponentRegistry.standard): List<String> =
            ComponentType.entries
                .filter { it != ComponentType.UNKNOWN && it !in PENDING && it !in FALLBACK_ONLY && registry.builder(it) != null }
                .map { it.wire }

        fun supportedFeatures(registry: ComponentRegistry = ComponentRegistry.standard): List<String> =
            features.filter { f -> f.implemented && (f.needs == null || registry.builder(f.needs) != null) }.map { it.key }

        fun of(registry: ComponentRegistry = ComponentRegistry.standard): DeviceCapabilities = DeviceCapabilities(
            allowedComponents = allowedComponents(registry),
            disabledFeatures = emptyList(),
            supportedFeatures = supportedFeatures(registry),
        )
    }
}
