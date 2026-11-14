// 審査用ビルドでは外す(#319・docs/ios/review-build.md)。
#if !APP_STORE_REVIEW
import Foundation
import MetamonRenderer
import MetamonServices

/// フィードバック(#319)の iOS と Mac で同じ部分(#983)。同意文・失敗の文言・端末の情報の詰め方。
enum FeedbackCopy {
    /// 送る前に見せる同意文。変えたら `FeedbackConsent.version` を上げる。
    static var consent: String {
        L10n.tr("送った内容(本文と、オンにした添付)は、Tailor It の不具合を直し、改善するために運営者が読みます。Google Cloud(Firestore)に保存し、\(FeedbackConsent.retentionDays) 日後に消去します。アカウントを削除したときも消去します。第三者には渡しません。")
    }

    static func message(for error: Error) -> String {
        switch FailureKind.classify(error) {
        case .rateLimited: L10n.tr("今日はこれ以上送れません。明日また送ってください。")
        case .offline: L10n.tr("接続がありません。つながる場所でもう一度送ってください。")
        case .timeout, .busy: L10n.tr("サーバーが混み合っています。少し待ってからもう一度送ってください。")
        case .sessionExpired: L10n.tr("サインインし直してから送ってください。")
        default: L10n.tr("もう一度送ってください。続く場合は添付を減らしてください。")
        }
    }

    /// アプリの版・OS・機種・言語。送る項目は iOS と同じ(名前・端末名・シリアル番号などは入れない)。
    @MainActor
    static func device(os: String, model: String, bundle: Bundle = .main) -> FeedbackSubmission.Device {
        let info = bundle.infoDictionary ?? [:]
        return .init(appVersion: info["CFBundleShortVersionString"] as? String ?? "-",
                     build: info["CFBundleVersion"] as? String ?? "-",
                     os: os,
                     model: model,
                     locale: LanguagePreference.shared.code)
    }
}
#endif
