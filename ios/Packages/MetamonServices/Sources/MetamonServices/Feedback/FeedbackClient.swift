// 審査用ビルド(APP_STORE_REVIEW)では外す: プライバシーの表示に無い種類(サポートへの問い合わせ・診断)を集めるため(#319)。
#if !APP_STORE_REVIEW
import Foundation

/// アプリ内のフィードバック(#319)。添付はどれも本人が選んだものだけ(既定はすべて無し)。記録の中身は送らない。
/// 契約: docs/backend/openapi-appconfig.yaml `POST /feedback`
public struct FeedbackSubmission: Sendable, Equatable {
    public enum Kind: String, Sendable, CaseIterable { case bug, idea, other }

    /// 今のアプリの DSL(構造だけ)。サーバーが本人の木の `versions/{dslVersion}` を写すので、送るのは参照だけ。
    public struct AppRef: Sendable, Equatable, Encodable {
        public var appId: String
        public var dslVersion: Int
        public init(appId: String, dslVersion: Int) {
            self.appId = appId
            self.dslVersion = dslVersion
        }
    }

    /// アプリの版・OS・機種。
    public struct Device: Sendable, Equatable, Encodable {
        public var appVersion: String
        public var build: String
        public var os: String
        public var model: String
        public var locale: String
        public init(appVersion: String, build: String, os: String, model: String, locale: String) {
            self.appVersion = appVersion
            self.build = build
            self.os = os
            self.model = model
            self.locale = locale
        }
    }

    public var kind: Kind
    public var message: String
    /// 送信前に見せた同意文の版(`FeedbackConsent.version`)
    public var consentVersion: String
    /// JPEG(`FeedbackScreenshot.jpeg` で 512 KiB 以下にしたもの)
    public var screenshotJPEG: Data?
    public var app: AppRef?
    public var device: Device?

    public init(kind: Kind = .other, message: String, consentVersion: String = FeedbackConsent.version,
                screenshotJPEG: Data? = nil, app: AppRef? = nil, device: Device? = nil) {
        self.kind = kind
        self.message = message
        self.consentVersion = consentVersion
        self.screenshotJPEG = screenshotJPEG
        self.app = app
        self.device = device
    }
}

/// 同意文の版。文言(FeedbackComposerView の同意の段落)を変えたら上げる。サーバーは `consentVersion` として保存する。
public enum FeedbackConsent {
    public static let version = "feedback-2026-11"
    /// サーバーの保存期間(日)。同意文にも出す。backend `store.FeedbackRetention` と揃える。
    public static let retentionDays = 180
    /// スクショの上限(デコード後のバイト)。backend `maxFeedbackScreenshot` と揃える。
    public static let maxScreenshotBytes = 512 << 10
    public static let maxMessageLength = 2000
}

/// サーバーが実際に保存した添付。
public struct FeedbackReceipt: Sendable, Equatable, Decodable {
    public struct Attachments: Sendable, Equatable, Decodable {
        public var screenshot: Bool
        public var dsl: Bool
        public var device: Bool
    }
    public var id: String
    public var status: String
    public var attachments: Attachments
}

public protocol FeedbackClient: Sendable {
    func send(_ feedback: FeedbackSubmission) async throws -> FeedbackReceipt
}

/// appconfig の `POST /v1/feedback`。
public struct HTTPFeedbackClient: FeedbackClient {
    public let baseURL: URL
    public let token: any TokenProvider

    public init(baseURL: URL, token: any TokenProvider) {
        self.baseURL = baseURL
        self.token = token
    }

    struct Body: Encodable {
        struct Attachments: Encodable {
            var screenshot: String?
            var app: FeedbackSubmission.AppRef?
            var device: FeedbackSubmission.Device?
        }
        var kind: String
        var message: String
        var consentVersion: String
        var attachments: Attachments?

        init(_ f: FeedbackSubmission) {
            kind = f.kind.rawValue
            message = f.message.trimmingCharacters(in: .whitespacesAndNewlines)
            consentVersion = f.consentVersion
            let a = Attachments(screenshot: f.screenshotJPEG?.base64EncodedString(), app: f.app, device: f.device)
            attachments = (a.screenshot == nil && a.app == nil && a.device == nil) ? nil : a
        }
    }

    public func send(_ feedback: FeedbackSubmission) async throws -> FeedbackReceipt {
        let data = try await HTTP.send("POST", baseURL: baseURL, path: "v1/feedback", body: Body(feedback), token: token, timeout: 30)
        return try JSONDecoder().decode(FeedbackReceipt.self, from: data)
    }
}

/// モック(サーバー無しの起動・テスト)。送った内容を覚えるだけ。
public actor MockFeedbackClient: FeedbackClient {
    public private(set) var sent: [FeedbackSubmission] = []
    public init() {}

    public func send(_ feedback: FeedbackSubmission) async throws -> FeedbackReceipt {
        sent.append(feedback)
        return FeedbackReceipt(id: "fb_mock\(sent.count)", status: "received",
                               attachments: .init(screenshot: feedback.screenshotJPEG != nil, dsl: feedback.app != nil, device: feedback.device != nil))
    }
}
#endif
