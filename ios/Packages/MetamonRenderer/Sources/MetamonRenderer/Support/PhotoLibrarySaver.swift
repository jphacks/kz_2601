import Photos

/// 画像をカメラロールに保存する共通の口(#2220・親 #1807)。部品・アクションはまだ無い(決定待ち)。
/// `DeviceCalendarSource` と同じ形: protocol + 実体(`LivePhotoLibrary`)+ 偽物(`FakePhotoLibrary`)。
///
/// 肝は 2 つ:
/// - **追加だけの許可(`.addOnly`)**: 既にある写真は読まない。`NSPhotoLibraryAddUsageDescription` だけで足りる。
/// - **結果が 3 つに分かれる**: 保存した(`saved`)/ 許可なし(`notAuthorized`)/ 失敗(`failed`)。
///   前は許可なしも失敗も同じ扱いだったので、呼ぶ側が文言を出し分けられなかった。
///
/// 保存する画像そのものはログ・クラッシュ報告・サーバーに出さない。
public protocol PhotoLibraryAdding: Sendable {
    /// 追加だけの許可の状態。`requesting: true` で未決定なら OS の許可ダイアログを出す。
    func authorization(requesting: Bool) async -> PhotoAddAccess
    /// 画像を 1 枚カメラロールに足す。失敗は throw(許可の判定は呼ぶ側)。
    func add(_ image: PlatformImage) async throws
}

/// PhotoKit の「追加だけ」の許可状態を自前の値にしたもの。
public enum PhotoAddAccess: String, Sendable, Equatable, CaseIterable {
    case notDetermined
    case denied
    case restricted
    /// 一部だけ許可。追加はできる(見える範囲が狭いだけ)
    case limited
    case authorized

    /// 追加だけならこの 2 つで足りる
    public var canAdd: Bool { self == .authorized || self == .limited }

    /// 将来増えた値は「制限」(保存しない)。
    public init(_ status: PHAuthorizationStatus) {
        switch status {
        case .notDetermined: self = .notDetermined
        case .denied: self = .denied
        case .restricted: self = .restricted
        case .limited: self = .limited
        case .authorized: self = .authorized
        @unknown default: self = .restricted
        }
    }
}

/// 保存の結果。この 3 つだけ(単体テストの対象)。
public enum PhotoSaveResult: Sendable, Equatable {
    /// カメラロールに入った
    case saved
    /// 追加の許可が無い(未決定のまま拒否・拒否・制限)。設定へ案内できる
    case notAuthorized(PhotoAddAccess)
    /// 許可はあるのに保存できなかった(容量・PhotoKit の失敗)
    case failed

    public var isSaved: Bool { self == .saved }
}

@MainActor
public enum PhotoLibrarySaver {
    /// 保存する口の実体。テスト・プレビューでは差し替える。
    public static var library: any PhotoLibraryAdding = LivePhotoLibrary()

    /// 追加だけの許可を求めて画像を保存する。
    public static func save(_ image: PlatformImage) async -> PhotoSaveResult {
        await save(image, to: library)
    }

    /// 保存する所を差し替えられる形(単体テストはこちらを呼ぶ)。
    public static func save(_ image: PlatformImage, to library: any PhotoLibraryAdding) async -> PhotoSaveResult {
        let access = await library.authorization(requesting: true)
        guard access.canAdd else { return .notAuthorized(access) }
        do {
            try await library.add(image)
            return .saved
        } catch {
            return .failed
        }
    }
}

/// PhotoKit に本当に保存する実体。
public struct LivePhotoLibrary: PhotoLibraryAdding {
    public init() {}

    public func authorization(requesting: Bool) async -> PhotoAddAccess {
        guard requesting else { return PhotoAddAccess(PHPhotoLibrary.authorizationStatus(for: .addOnly)) }
        return PhotoAddAccess(await PHPhotoLibrary.requestAuthorization(for: .addOnly))
    }

    public func add(_ image: PlatformImage) async throws {
        try await PHPhotoLibrary.shared().performChanges {
            PHAssetChangeRequest.creationRequestForAsset(from: image)
        }
    }
}

/// テスト・プレビュー用。許可の状態と失敗を外から決められる。
public final class FakePhotoLibrary: PhotoLibraryAdding, @unchecked Sendable {
    public enum Failure: Error, Equatable {
        /// PhotoKit の `performChanges` が失敗した
        case changesFailed
    }

    public var access: PhotoAddAccess
    public var addError: Failure?
    /// 許可を聞いた回数
    public private(set) var authorizationCalls = 0
    /// 保存しようとした回数(許可なしなら増えない)
    public private(set) var addCalls = 0

    public init(access: PhotoAddAccess = .authorized, addError: Failure? = nil) {
        self.access = access
        self.addError = addError
    }

    public func authorization(requesting: Bool) async -> PhotoAddAccess {
        authorizationCalls += 1
        return access
    }

    public func add(_ image: PlatformImage) async throws {
        addCalls += 1
        if let addError { throw addError }
    }
}
