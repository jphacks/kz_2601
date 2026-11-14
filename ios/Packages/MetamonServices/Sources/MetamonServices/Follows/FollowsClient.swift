import Foundation

/// フォロー(#270)。アプリごとに、サインイン中の人が誰をフォローしているか(appconfig の `/v1/apps/{appId}/follows`)。
/// フォローは本人だけが見られ、相手に知らせない。
public protocol FollowsClient: Sendable {
    /// `GET /v1/apps/{appId}/follows`: フォローしている人の uid。
    func list(appId: String) async throws -> [String]
    /// `PUT` / `DELETE /v1/apps/{appId}/follows/{uid}`(冪等)。操作の後にフォローしているかを返す。
    func setFollow(appId: String, uid: String, on: Bool) async throws -> Bool
}

/// API の `details.reason`。
public enum FollowsReason {
    /// 1 アプリでフォローできる人数(500)を超えた
    public static let full = "follows_full"
    /// 参加者だけのアプリで、相手が参加者でない
    public static let notInApp = "not_in_app"
}

/// フォロー(#270)。appconfig が live の時だけ(モックでは nil = フォローのボタンを出さず、フォロー中の一覧は空)。
public enum FollowsServiceKey: ServiceKey {
    public static var defaultValue: (any FollowsClient)? { nil }
}

extension Services {
    public var follows: (any FollowsClient)? {
        get { self[FollowsServiceKey.self] }
        set { self[FollowsServiceKey.self] = newValue }
    }
}
