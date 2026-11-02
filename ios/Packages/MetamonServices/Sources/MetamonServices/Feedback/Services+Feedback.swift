#if !APP_STORE_REVIEW // 審査用ビルドはフィードバックを外す(#319)
import Foundation

/// アプリ内のフィードバック(#319)。live では `HTTPFeedbackClient`、モックでは `MockFeedbackClient`。
public enum FeedbackServiceKey: ServiceKey {
    public static let defaultValue: any FeedbackClient = MockFeedbackClient()
}

extension Services {
    public var feedback: any FeedbackClient {
        get { self[FeedbackServiceKey.self] }
        set { self[FeedbackServiceKey.self] = newValue }
    }
}
#endif
