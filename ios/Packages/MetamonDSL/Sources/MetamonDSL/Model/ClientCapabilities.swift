import Foundation

/// AI(generate / chat)に「この iOS が描ける部品」を伝える(#245・#302)。
/// 審査用ビルド(APP_STORE_REVIEW)はネイティブ API を使う部品をバイナリから外すので、AI にも使わせない。
/// 通常のビルドも描ける全部品を毎回送る(#302): 後の版で backend に足された部品は、この版には出さない。
/// 契約: docs/backend/openapi-ai.yaml・版の規則: docs/dsl/README.md §5
public enum ClientCapabilities {
    /// この版が描画できる DSL の最大 `schemaVersion`(ヘッダ `X-Client-DSL-Version`)。
    public static let supportedSchemaVersion = 1

    /// この版が描画できる全部品(`unknown` を除く全 type)。
    public static var allComponents: [String] {
        ComponentType.allCases.filter { $0 != .unknown }.map(\.rawValue)
    }

    /// 審査用ビルドでバイナリから外す部品(Renderer の登録もしない)。
    public static let reviewExcludedComponents: Set<ComponentType> = [.healthStat, .locationTracker, .nearbyShare, .dashcam, .sensorValue, .audioRecorder, .webView, .chatThread]
    /// 審査用ビルドで無効にする機能。fieldHealth = Field.health(HealthKit への書き込み)、cameraCapture = photoField / videoField の撮影、
    /// sharedData = entity の scope shared / public(他の人と同じ記録を見る作り・#451 G2。backend が private に直す)。
    public static let reviewDisabledFeatures = ["fieldHealth", "cameraCapture", "sharedData"]

    /// 審査用ビルドのときの部品の allowlist(`unknown` を除く全 type − 外した部品)。
    public static var reviewAllowedComponents: [String] {
        ComponentType.allCases.filter { $0 != .unknown && !reviewExcludedComponents.contains($0) }.map(\.rawValue)
    }

    /// このビルドの値。通常 = 全部品(#302)、審査用 = 外した部品を除く。
    public static var allowedComponents: [String]? {
        #if APP_STORE_REVIEW
        reviewAllowedComponents
        #else
        allComponents
        #endif
    }

    public static var disabledFeatures: [String]? {
        #if APP_STORE_REVIEW
        reviewDisabledFeatures
        #else
        nil
        #endif
    }
}
