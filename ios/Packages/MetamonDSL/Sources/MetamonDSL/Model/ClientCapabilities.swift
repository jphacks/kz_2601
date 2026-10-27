import Foundation

/// AI(generate / chat)に「この iOS が描ける部品」を伝える(#245)。
/// 審査用ビルド(APP_STORE_REVIEW)はネイティブ API を使う部品をバイナリから外すので、AI にも使わせない。
/// 通常のビルドは nil(送らない)= backend は今まで通り全部使う。契約: docs/backend/openapi-ai.yaml
public enum ClientCapabilities {
    /// 審査用ビルドでバイナリから外す部品(Renderer の登録もしない)。
    public static let reviewExcludedComponents: Set<ComponentType> = [.healthStat, .locationTracker, .nearbyShare]
    /// 審査用ビルドで無効にする機能。fieldHealth = Field.health(HealthKit への書き込み)、cameraCapture = photoField / videoField の撮影。
    public static let reviewDisabledFeatures = ["fieldHealth", "cameraCapture"]

    /// 審査用ビルドのときの部品の allowlist(`unknown` を除く全 type − 外した部品)。
    public static var reviewAllowedComponents: [String] {
        ComponentType.allCases.filter { $0 != .unknown && !reviewExcludedComponents.contains($0) }.map(\.rawValue)
    }

    /// このビルドの値。通常 = nil(フィールドを送らない)。
    public static var allowedComponents: [String]? {
        #if APP_STORE_REVIEW
        reviewAllowedComponents
        #else
        nil
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
