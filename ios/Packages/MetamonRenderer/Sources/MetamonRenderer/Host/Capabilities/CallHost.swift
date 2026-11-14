// 審査用ビルド(APP_STORE_REVIEW)は通話(#447)を外す(共有と一緒・マイクとカメラで他の人とつながる)。
#if !APP_STORE_REVIEW
import MetamonDSL

/// 通話(#447)。callButton を押すとホストが通話の画面を開く(トークン・LiveKit・コインはホストが持つ)。
/// 持たないホスト(モック・プレビュー)ではボタンを押せない。
@MainActor
public protocol CallHost: AnyObject {
    /// このアプリで通話できるか(共有のアプリで、live で、サインインしている)。false ならボタンを押せない。
    var canCall: Bool { get }
    /// 通話の画面を開く。`room` は callButton の room(nil = アプリの部屋)、`video` は部品の video。
    func startCall(room: String?, video: Bool, title: String)
}

public extension DSLRuntime {
    var calls: (any CallHost)? { self as? any CallHost }
}
#endif
