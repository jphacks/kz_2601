import Foundation
import MetamonServices

/// live 構成で 1 機能ぶんのサービスを `Services` に差し込む(#516)。
/// 1 機能 = `Features/<機能>/<機能>+Wiring.swift` の `static let` 1 つ + `BuildConfig.liveWirings` の 1 行。
/// 審査用ビルドで外す機能は、closure の中身を `#if !APP_STORE_REVIEW` で囲む(シンボルを参照しない)。
struct LiveWiring: Sendable {
    /// (services, appconfig の URL(nil = appconfig はインメモリ), Bearer の発行元)
    let apply: @Sendable (inout Services, URL?, any TokenProvider) -> Void
}
