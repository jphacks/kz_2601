import CoreGraphics
import Foundation
import Photos

/// 写真ライブラリを日付・アルバムで読む口(#2219・親 #1806)。部品・アクションはまだ無い(決定待ち)。
/// `DeviceCalendarSource` と同じ形: protocol + 実体(`LiveDevicePhotos`)+ 偽物(`FakeDevicePhotos`)。
///
/// 肝は 2 つ:
/// - **一部だけ許可(`.limited`)**: ユーザーが選んだ写真だけが見える。PhotoKit 側で既に絞られて返るが、
///   偽物と同じ道を通すために `DevicePhotosQuery.visible(_:access:selectedIds:)` で明示的に絞る。
///   アルバムは `.limited` では見えない(空を返す)。
/// - **許可が無い時に落ちない**: 読み取りは `DevicePhotosError.notAuthorized` を投げるだけ。
///   説明文が無いビルドでは許可を求めない(求めると落ちるため)。
///
/// 画像そのものはサムネイルだけを読み、サーバーには送らない(iCloud からの取得も許さない)。
public protocol DevicePhotosSource: Sendable {
    /// 許可の状態。`requesting: true` で未決定なら OS の許可ダイアログを出す(読み取り = `.readWrite`)。
    func authorization(requesting: Bool) async -> DevicePhotoAccess
    /// `from..<to` に撮られた写真・動画(新しい順)。期間は `DevicePhotosLimits` で切り、件数も上限で切る。
    /// `albumId` が nil なら全体。許可が無ければ `DevicePhotosError.notAuthorized`。
    func assets(from: Date, to: Date, albumId: String?) async throws -> [DevicePhotoAsset]
    /// 読む先を選ぶ画面のため: アルバム(名前順)。`.limited` では空(アルバムは見えない)。
    func albums() async throws -> [DevicePhotoAlbum]
    /// サムネイルだけを読む(元画像は読まない・iCloud からも取らない)。取れなければ nil。
    func thumbnail(for assetId: String, maxPixel: CGFloat) async -> PlatformImage?
}

/// PhotoKit の許可状態を自前の値にしたもの。
public enum DevicePhotoAccess: String, Sendable, Equatable, CaseIterable {
    case notDetermined
    case denied
    case restricted
    /// 一部だけ許可。ユーザーが選んだ写真だけが見える
    case limited
    case authorized

    /// 一部だけ許可でも読める(見える範囲が狭いだけ)
    public var canRead: Bool { self == .authorized || self == .limited }

    /// 選ばれた物だけが見えるか
    public var isLimited: Bool { self == .limited }

    /// 将来増えた値は「制限」(読まない)。
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

public enum DevicePhotosError: LocalizedError, Equatable {
    /// 読み取りが許可されていない(未決定・拒否・制限)
    case notAuthorized(DevicePhotoAccess)

    public var errorDescription: String? {
        switch self {
        case .notAuthorized: L("写真へのアクセスが許可されていません")
        }
    }
}

public enum DevicePhotoKind: String, Sendable, Equatable, CaseIterable {
    case image
    case video
    /// 写真でも動画でもない物(オーディオなど)は読まない
    case other

    init(_ type: PHAssetMediaType) {
        switch type {
        case .image: self = .image
        case .video: self = .video
        default: self = .other
        }
    }
}

/// 読む期間・件数・サムネイルの上限。超えた分は切る(エラーにしない)。
public enum DevicePhotosLimits {
    /// 今日から過去に読む日数
    public static let daysBack = 365
    /// 今日から先に読む日数(端末の時計が進んでいる写真のため)
    public static let daysAhead = 1
    /// 1 回で返す件数
    public static let maxItems = 200
    /// サムネイルの長辺(px)
    public static let thumbnailMaxPixel: CGFloat = 256

    /// `from..<to` を「今日 0:00 - daysBack 日」〜「今日 0:00 + daysAhead + 1 日」に収める。空なら nil。
    public static func clamp(from: Date, to: Date, now: Date = Date(), calendar: Calendar = .current) -> DateInterval? {
        let today = calendar.startOfDay(for: now)
        guard let lower = calendar.date(byAdding: .day, value: -daysBack, to: today),
              let upper = calendar.date(byAdding: .day, value: daysAhead + 1, to: today) else { return nil }
        let start = max(from, lower)
        let end = min(to, upper)
        guard start < end else { return nil }
        return DateInterval(start: start, end: end)
    }

    /// 「先週の写真」のような「今日を含む直近 n 日」。`days: 7` で今日 0:00 の 7 日前から今の直後まで。
    public static func recentDays(_ days: Int, now: Date = Date(), calendar: Calendar = .current) -> DateInterval? {
        let today = calendar.startOfDay(for: now)
        guard days > 0, let start = calendar.date(byAdding: .day, value: -days, to: today),
              let end = calendar.date(byAdding: .day, value: 1, to: today) else { return nil }
        return clamp(from: start, to: end, now: now, calendar: calendar)
    }
}

public struct DevicePhotoAlbum: Sendable, Equatable, Identifiable {
    public var id: String
    public var name: String
    public var count: Int
    public init(id: String, name: String, count: Int) {
        self.id = id
        self.name = name
        self.count = count
    }
}

/// 1 枚の写真・動画。画像そのものは入らない(サムネイルは `thumbnail(for:maxPixel:)` で別に読む)。
public struct DevicePhotoAsset: Sendable, Equatable, Identifiable, CustomStringConvertible, CustomDebugStringConvertible {
    public var id: String
    public var created: Date
    public var kind: DevicePhotoKind
    /// 場所が付いているか(座標そのものは渡さない)
    public var hasLocation: Bool
    public var pixelWidth: Int
    public var pixelHeight: Int
    /// 動画の長さ(秒)。写真は nil
    public var duration: TimeInterval?
    public var isFavorite: Bool

    public init(id: String, created: Date, kind: DevicePhotoKind, hasLocation: Bool,
                pixelWidth: Int = 0, pixelHeight: Int = 0, duration: TimeInterval? = nil, isFavorite: Bool = false) {
        self.id = id
        self.created = created
        self.kind = kind
        self.hasLocation = hasLocation
        self.pixelWidth = pixelWidth
        self.pixelHeight = pixelHeight
        self.duration = duration
        self.isFavorite = isFavorite
    }

    /// 撮った日・場所の有無は出さない(誤ってログに入れても漏れない)
    public var description: String { "DevicePhotoAsset(kind: \(kind.rawValue))" }
    public var debugDescription: String { description }
}

@MainActor
public enum DevicePhotos {
    /// 読む口の実体。アプリ起動時に 1 回だけ差し替える(`METAMON_MOCK=1` は `FakeDevicePhotos.sample`)。
    public static var source: any DevicePhotosSource = LiveDevicePhotos()
}

// MARK: - 実体と偽物で共有する絞り込み(単体テストの対象)

enum DevicePhotosQuery {
    static let creationDateKey = "creationDate"

    /// 期間・件数・並び順を `PHFetchOptions` に組み立てる(単体テストの対象)。
    /// `creationDate >= start AND creationDate < end` の半開区間。新しい順。
    static func fetchOptions(in range: DateInterval, limit: Int = DevicePhotosLimits.maxItems) -> PHFetchOptions {
        let options = PHFetchOptions()
        options.predicate = predicate(in: range)
        options.sortDescriptors = [NSSortDescriptor(key: creationDateKey, ascending: false)]
        options.fetchLimit = max(0, limit)
        options.includeHiddenAssets = false
        return options
    }

    /// 期間の絞り込み(半開区間)。終わりの日時そのものは含まない。
    static func predicate(in range: DateInterval) -> NSPredicate {
        NSPredicate(format: "%K >= %@ AND %K < %@",
                    creationDateKey, range.start as NSDate,
                    creationDateKey, range.end as NSDate)
    }

    /// 一部だけ許可の時は、ユーザーが選んだ物だけを残す(単体テストの対象)。
    /// 実機では PhotoKit 側で既に絞られて返るが、偽物と同じ道を通すために明示的に絞る。
    static func visible(_ all: [DevicePhotoAsset], access: DevicePhotoAccess, selectedIds: Set<String>) -> [DevicePhotoAsset] {
        guard access.canRead else { return [] }
        guard access.isLimited else { return all }
        return all.filter { selectedIds.contains($0.id) }
    }

    /// 期間で絞り、新しい順(同時なら id 順)に並べて上限で切る。写真・動画以外は落とす。
    static func assets(_ all: [DevicePhotoAsset], in range: DateInterval, limit: Int = DevicePhotosLimits.maxItems) -> [DevicePhotoAsset] {
        let hits = all.filter { $0.kind != .other && $0.created >= range.start && $0.created < range.end }
        let sorted = hits.sorted { a, b in
            a.created != b.created ? a.created > b.created : a.id < b.id
        }
        return Array(sorted.prefix(max(0, limit)))
    }

    /// 一部だけ許可ではアルバムが見えないので空。それ以外は名前順。
    static func albums(_ all: [DevicePhotoAlbum], access: DevicePhotoAccess) -> [DevicePhotoAlbum] {
        guard access.canRead, !access.isLimited else { return [] }
        return all.sorted { $0.name.localizedStandardCompare($1.name) == .orderedAscending }
    }
}

// MARK: - 偽物

/// `METAMON_MOCK=1`・テスト・プレビュー用。中身と許可の状態・選ばれた写真を外から決められる。
public struct FakeDevicePhotos: DevicePhotosSource {
    public var access: DevicePhotoAccess
    /// `requesting: true` で未決定の時に、ダイアログで選ばれたことにする状態
    public var grantOnRequest: DevicePhotoAccess
    public var allAssets: [DevicePhotoAsset]
    /// 一部だけ許可の時にユーザーが選んだ写真の id
    public var selectedIds: Set<String>
    public var albumList: [DevicePhotoAlbum]
    /// アルバムの id → 入っている写真の id
    public var albumMembers: [String: Set<String>]
    public var now: @Sendable () -> Date

    public init(access: DevicePhotoAccess = .authorized,
                grantOnRequest: DevicePhotoAccess = .authorized,
                assets: [DevicePhotoAsset] = [],
                selectedIds: Set<String> = [],
                albums: [DevicePhotoAlbum] = [],
                albumMembers: [String: Set<String>] = [:],
                now: @escaping @Sendable () -> Date = { Date() }) {
        self.access = access
        self.grantOnRequest = grantOnRequest
        self.allAssets = assets
        self.selectedIds = selectedIds
        self.albumList = albums
        self.albumMembers = albumMembers
        self.now = now
    }

    public func authorization(requesting: Bool) async -> DevicePhotoAccess {
        requesting && access == .notDetermined ? grantOnRequest : access
    }

    public func assets(from: Date, to: Date, albumId: String?) async throws -> [DevicePhotoAsset] {
        try check()
        guard let range = DevicePhotosLimits.clamp(from: from, to: to, now: now()) else { return [] }
        var pool = DevicePhotosQuery.visible(allAssets, access: access, selectedIds: selectedIds)
        if let albumId {
            // 一部だけ許可ではアルバムが見えない(= 指定しても空)
            guard !access.isLimited, let members = albumMembers[albumId] else { return [] }
            pool = pool.filter { members.contains($0.id) }
        }
        return DevicePhotosQuery.assets(pool, in: range)
    }

    public func albums() async throws -> [DevicePhotoAlbum] {
        try check()
        return DevicePhotosQuery.albums(albumList, access: access)
    }

    public func thumbnail(for assetId: String, maxPixel: CGFloat) async -> PlatformImage? {
        guard access.canRead,
              DevicePhotosQuery.visible(allAssets, access: access, selectedIds: selectedIds).contains(where: { $0.id == assetId })
        else { return nil }
        return PlatformImage()
    }

    private func check() throws {
        guard access.canRead else { throw DevicePhotosError.notAuthorized(access) }
    }

    /// `METAMON_MOCK=1` の固定の中身(今日を基準に作る)。
    public static var sample: FakeDevicePhotos {
        let cal = Calendar.current
        let today = cal.startOfDay(for: Date())
        func at(_ day: Int, _ hour: Int) -> Date { cal.date(byAdding: .hour, value: day * 24 + hour, to: today) ?? today }
        let assets = [
            DevicePhotoAsset(id: "fake-1", created: at(0, 8), kind: .image, hasLocation: true, pixelWidth: 4032, pixelHeight: 3024),
            DevicePhotoAsset(id: "fake-2", created: at(-2, 19), kind: .image, hasLocation: false, pixelWidth: 3024, pixelHeight: 4032, isFavorite: true),
            DevicePhotoAsset(id: "fake-3", created: at(-5, 12), kind: .video, hasLocation: true, pixelWidth: 1920, pixelHeight: 1080, duration: 12.5),
        ]
        let trip = DevicePhotoAlbum(id: "fake-trip", name: "旅行", count: 2)
        let food = DevicePhotoAlbum(id: "fake-food", name: "ごはん", count: 1)
        return FakeDevicePhotos(assets: assets,
                                albums: [trip, food],
                                albumMembers: ["fake-trip": ["fake-1", "fake-3"], "fake-food": ["fake-2"]])
    }
}

// MARK: - 実体

/// PhotoKit から読む。`.limited` でも落ちず、選ばれた写真だけが返る(PhotoKit 側で絞られる)。
/// Info.plist に説明文が無いビルド(審査用の版など)では許可を求めずに `.restricted` を返す(求めると落ちるため)。
public final class LiveDevicePhotos: DevicePhotosSource, @unchecked Sendable {
    public init() {}

    static let usageKey = "NSPhotoLibraryUsageDescription"

    /// 説明文の有無は生の Info.plist で見る(`object(forInfoDictionaryKey:)` は訳を返すので、審査用ビルドで
    /// Info.plist から消しても `.lproj` に訳が残っていると「ある」と判定してしまう)。
    static func hasUsageDescription(in info: [String: Any]?) -> Bool {
        guard let text = info?[usageKey] as? String else { return false }
        return !text.isEmpty
    }

    static func currentAccess() -> DevicePhotoAccess {
        DevicePhotoAccess(PHPhotoLibrary.authorizationStatus(for: .readWrite))
    }

    public func authorization(requesting: Bool) async -> DevicePhotoAccess {
        let current = Self.currentAccess()
        guard requesting, current == .notDetermined else { return current }
        guard Self.hasUsageDescription(in: Bundle.main.infoDictionary) else { return .restricted }
        return DevicePhotoAccess(await PHPhotoLibrary.requestAuthorization(for: .readWrite))
    }

    public func assets(from: Date, to: Date, albumId: String?) async throws -> [DevicePhotoAsset] {
        let access = Self.currentAccess()
        guard access.canRead else { throw DevicePhotosError.notAuthorized(access) }
        guard let range = DevicePhotosLimits.clamp(from: from, to: to) else { return [] }
        let options = DevicePhotosQuery.fetchOptions(in: range)
        let result: PHFetchResult<PHAsset>
        if let albumId {
            // 一部だけ許可ではアルバムが見えない(指定しても空)
            guard !access.isLimited, let collection = Self.collection(albumId) else { return [] }
            result = PHAsset.fetchAssets(in: collection, options: options)
        } else {
            result = PHAsset.fetchAssets(with: options)
        }
        var mapped: [DevicePhotoAsset] = []
        result.enumerateObjects { asset, _, _ in mapped.append(Self.asset(asset)) }
        // `.limited` は PhotoKit 側で既に選ばれた物だけに絞られて返る(ここは念のための二重の絞り)
        let visible = DevicePhotosQuery.visible(mapped, access: access,
                                                selectedIds: access.isLimited ? Set(mapped.map(\.id)) : [])
        return DevicePhotosQuery.assets(visible, in: range)
    }

    public func albums() async throws -> [DevicePhotoAlbum] {
        let access = Self.currentAccess()
        guard access.canRead else { throw DevicePhotosError.notAuthorized(access) }
        guard !access.isLimited else { return [] } // 一部だけ許可ではアルバムが見えない
        var found: [DevicePhotoAlbum] = []
        for (type, subtype) in [(PHAssetCollectionType.album, PHAssetCollectionSubtype.any),
                                (.smartAlbum, .smartAlbumFavorites)] {
            PHAssetCollection.fetchAssetCollections(with: type, subtype: subtype, options: nil)
                .enumerateObjects { collection, _, _ in found.append(Self.album(collection)) }
        }
        return DevicePhotosQuery.albums(found, access: access)
    }

    public func thumbnail(for assetId: String, maxPixel: CGFloat = DevicePhotosLimits.thumbnailMaxPixel) async -> PlatformImage? {
        guard Self.currentAccess().canRead else { return nil }
        guard let asset = PHAsset.fetchAssets(withLocalIdentifiers: [assetId], options: nil).firstObject else { return nil }
        let options = PHImageRequestOptions()
        options.deliveryMode = .fastFormat
        options.resizeMode = .fast
        options.isSynchronous = false
        options.isNetworkAccessAllowed = false // iCloud から取らない(端末にある分だけ)
        let side = max(1, maxPixel)
        let box = OnceBox()
        return await withCheckedContinuation { (cont: CheckedContinuation<PlatformImage?, Never>) in
            PHImageManager.default().requestImage(for: asset, targetSize: CGSize(width: side, height: side),
                                                  contentMode: .aspectFit, options: options) { image, _ in
                box.resumeOnce { cont.resume(returning: image) }
            }
        }
    }

    static func collection(_ id: String) -> PHAssetCollection? {
        PHAssetCollection.fetchAssetCollections(withLocalIdentifiers: [id], options: nil).firstObject
    }

    static func album(_ collection: PHAssetCollection) -> DevicePhotoAlbum {
        let count = collection.estimatedAssetCount == NSNotFound
            ? PHAsset.fetchAssets(in: collection, options: nil).count
            : collection.estimatedAssetCount
        return DevicePhotoAlbum(id: collection.localIdentifier, name: collection.localizedTitle ?? "", count: count)
    }

    static func asset(_ asset: PHAsset) -> DevicePhotoAsset {
        let kind = DevicePhotoKind(asset.mediaType)
        return DevicePhotoAsset(
            id: asset.localIdentifier,
            created: asset.creationDate ?? asset.modificationDate ?? Date.distantPast,
            kind: kind,
            hasLocation: asset.location != nil,
            pixelWidth: asset.pixelWidth,
            pixelHeight: asset.pixelHeight,
            duration: kind == .video ? asset.duration : nil,
            isFavorite: asset.isFavorite
        )
    }
}

/// `requestImage` は粗い画像 → きれいな画像の順で 2 回呼ばれることがあるので、再開は 1 回だけにする。
private final class OnceBox: @unchecked Sendable {
    private let lock = NSLock()
    private var done = false

    func resumeOnce(_ body: () -> Void) {
        lock.lock()
        let first = !done
        done = true
        lock.unlock()
        if first { body() }
    }
}

#if os(iOS)
import UIKit

public extension LiveDevicePhotos {
    /// 一部だけ許可の時に「写真を追加で選ぶ」画面を出す。許可が一部だけでない時は何もしない。
    @MainActor
    func presentLimitedLibraryPicker(from viewController: UIViewController) {
        guard Self.currentAccess().isLimited else { return }
        PHPhotoLibrary.shared().presentLimitedLibraryPicker(from: viewController)
    }
}
#endif
