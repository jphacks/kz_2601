import Contacts
import Foundation

/// 端末の連絡先の一覧を読む口(#2218・親 #1805)。部品・アクションはまだ無い(決定待ち)。
/// `DeviceCalendarSource` と同じ形: protocol + 実体(`LiveDeviceContacts`)+ 偽物(`FakeDeviceContacts`)。
/// 1 人だけ選ぶ `ContactPick`(許可不要)と違い、こちらは `CNContactStore` で読むので許可が要る。
/// 取るのは名前・誕生日・グループだけ(電話・メール・住所は `keysToFetch` に入れない)。
/// 値は端末の中で返すだけ。ログ・クラッシュ報告・サーバーに出さない(`description` も伏せる)。
public protocol DeviceContactsSource: Sendable {
    /// 許可の状態。`requesting: true` で未決定なら OS の許可ダイアログを出す。
    func authorization(requesting: Bool) async -> DeviceContactsAccess
    /// 連絡先(名前順・`DeviceContactsLimits.maxItems` で切る)。許可が無ければ `DeviceContactsError.notAuthorized`。
    /// limited(iOS 18 の一部だけ許可)では許可された人だけが返る。
    func contacts() async throws -> [DeviceContact]
    /// グループ(名前順)。
    func groups() async throws -> [DeviceContactGroup]
}

/// `CNAuthorizationStatus` を自前の値にしたもの。
public enum DeviceContactsAccess: String, Sendable, Equatable {
    case notDetermined
    case denied
    case restricted
    /// iOS 18 の「一部の連絡先だけ許可」。許可された人だけ読める
    case limited
    case authorized

    public var canRead: Bool { self == .authorized || self == .limited }

    /// 将来増えた値は「制限」(読まない)。
    public init(_ status: CNAuthorizationStatus) {
        if #available(iOS 18.0, macOS 15.0, *), status == .limited {
            self = .limited
            return
        }
        switch status {
        case .notDetermined: self = .notDetermined
        case .denied: self = .denied
        case .restricted: self = .restricted
        case .authorized: self = .authorized
        default: self = .restricted
        }
    }
}

public enum DeviceContactsError: LocalizedError, Equatable {
    case notAuthorized(DeviceContactsAccess)

    public var errorDescription: String? {
        L("連絡先へのアクセスが許可されていません")
    }
}

public enum DeviceContactsLimits {
    /// 1 回で返す人数
    public static let maxItems = 2000
}

/// 誕生日。年の無い誕生日(連絡先で年を入れていない)は `year == nil`。
public struct DeviceContactBirthday: Sendable, Equatable, Hashable {
    public var year: Int?
    public var month: Int
    public var day: Int

    public init(year: Int?, month: Int, day: Int) {
        self.year = year
        self.month = month
        self.day = day
    }

    /// 年があれば `YYYY-MM-DD`(date の field と同じ)、無ければ `--MM-DD`(ISO 8601 の年なし)。
    public var isoString: String {
        if let year { return String(format: "%04d-%02d-%02d", year, month, day) }
        return String(format: "--%02d-%02d", month, day)
    }
}

public struct DeviceContactGroup: Sendable, Equatable, Identifiable {
    public var id: String
    public var name: String
    public init(id: String, name: String) {
        self.id = id
        self.name = name
    }
}

public struct DeviceContact: Sendable, Equatable, Identifiable, CustomStringConvertible, CustomDebugStringConvertible {
    public var id: String
    public var name: String
    public var birthday: DeviceContactBirthday?
    /// 入っているグループの id(名前順に並べ替えない・`groups()` で引く)
    public var groupIds: [String]

    public init(id: String, name: String, birthday: DeviceContactBirthday?, groupIds: [String] = []) {
        self.id = id
        self.name = name
        self.birthday = birthday
        self.groupIds = groupIds
    }

    /// 名前・誕生日は出さない
    public var description: String { "DeviceContact(id: \(id), birthday: \(birthday != nil), groups: \(groupIds.count))" }
    public var debugDescription: String { description }
}

@MainActor
public enum DeviceContacts {
    /// 読む口の実体。アプリ起動時に 1 回だけ差し替える(`METAMON_MOCK=1` は `FakeDeviceContacts.sample`)。
    public static var source: any DeviceContactsSource = LiveDeviceContacts()
}

// MARK: - 実体と偽物で共有する変換(単体テストの対象)

enum DeviceContactsQuery {
    /// `CNContact.birthday` の日付の部品から。月・日が無ければ nil。年が無い / 未定義なら `year == nil`。
    static func birthday(_ c: DateComponents?) -> DeviceContactBirthday? {
        guard let c, let month = c.month, let day = c.day,
              month != NSDateComponentUndefined, day != NSDateComponentUndefined,
              (1...12).contains(month), (1...31).contains(day) else { return nil }
        let year = c.year.flatMap { $0 == NSDateComponentUndefined ? nil : $0 }
        return DeviceContactBirthday(year: year, month: month, day: day)
    }

    /// 名前の空白を詰め、名前の無い人は後ろへ。名前順(同名は id 順)に並べて上限で切る。
    static func sorted(_ all: [DeviceContact]) -> [DeviceContact] {
        let trimmed = all.map { c -> DeviceContact in
            var c = c
            c.name = c.name.trimmingCharacters(in: .whitespacesAndNewlines)
            return c
        }
        let sorted = trimmed.sorted { a, b in
            if a.name.isEmpty != b.name.isEmpty { return !a.name.isEmpty }
            let order = a.name.localizedStandardCompare(b.name)
            if order != .orderedSame { return order == .orderedAscending }
            return a.id < b.id
        }
        return Array(sorted.prefix(DeviceContactsLimits.maxItems))
    }

    static func sortedGroups(_ groups: [DeviceContactGroup]) -> [DeviceContactGroup] {
        groups.sorted { $0.name.localizedStandardCompare($1.name) == .orderedAscending }
    }

    /// グループ id → 人の id の一覧を、人ごとのグループ id に引き直す。
    static func groupIds(byContact members: [String: [String]]) -> [String: [String]] {
        var result: [String: [String]] = [:]
        for groupId in members.keys.sorted() {
            for contactId in members[groupId] ?? [] { result[contactId, default: []].append(groupId) }
        }
        return result
    }
}

// MARK: - 偽物

/// `METAMON_MOCK=1`・テスト・プレビュー用。
public struct FakeDeviceContacts: DeviceContactsSource {
    public var access: DeviceContactsAccess
    public var grantOnRequest: DeviceContactsAccess
    public var allContacts: [DeviceContact]
    public var groupList: [DeviceContactGroup]
    /// limited の時に読める人の id(nil なら全員)
    public var limitedIds: Set<String>?

    public init(access: DeviceContactsAccess = .authorized, grantOnRequest: DeviceContactsAccess = .authorized,
                contacts: [DeviceContact] = [], groups: [DeviceContactGroup] = [], limitedIds: Set<String>? = nil) {
        self.access = access
        self.grantOnRequest = grantOnRequest
        self.allContacts = contacts
        self.groupList = groups
        self.limitedIds = limitedIds
    }

    public func authorization(requesting: Bool) async -> DeviceContactsAccess {
        requesting && access == .notDetermined ? grantOnRequest : access
    }

    public func contacts() async throws -> [DeviceContact] {
        guard access.canRead else { throw DeviceContactsError.notAuthorized(access) }
        let visible = access == .limited ? allContacts.filter { limitedIds?.contains($0.id) ?? true } : allContacts
        return DeviceContactsQuery.sorted(visible)
    }

    public func groups() async throws -> [DeviceContactGroup] {
        guard access.canRead else { throw DeviceContactsError.notAuthorized(access) }
        return DeviceContactsQuery.sortedGroups(groupList)
    }

    public static var sample: FakeDeviceContacts {
        let family = DeviceContactGroup(id: "fake-family", name: "家族")
        let friends = DeviceContactGroup(id: "fake-friends", name: "友だち")
        return FakeDeviceContacts(
            contacts: [
                DeviceContact(id: "c1", name: "山田 花子", birthday: DeviceContactBirthday(year: 1990, month: 4, day: 12), groupIds: [family.id]),
                DeviceContact(id: "c2", name: "佐藤 太郎", birthday: DeviceContactBirthday(year: nil, month: 11, day: 3), groupIds: [friends.id]),
                DeviceContact(id: "c3", name: "鈴木 一郎", birthday: nil),
            ],
            groups: [family, friends]
        )
    }
}

// MARK: - 実体

/// Contacts から読む。Info.plist に説明文が無いビルド(審査用の版)では許可を求めずに `.restricted` を返す(求めると落ちるため)。
public final class LiveDeviceContacts: DeviceContactsSource, @unchecked Sendable {
    private let store = CNContactStore()

    public init() {}

    static let usageKey = "NSContactsUsageDescription"

    /// 生の Info.plist で見る(訳が `.lproj` に残っていても、審査用ビルドで消した版は「無い」とする)。
    static func hasUsageDescription(in info: [String: Any]?) -> Bool {
        guard let text = info?[usageKey] as? String else { return false }
        return !text.isEmpty
    }

    /// 名前(表示用の並び)と誕生日だけ。id は常に取れる。
    static var keysToFetch: [CNKeyDescriptor] {
        [CNContactFormatter.descriptorForRequiredKeys(for: .fullName), CNContactBirthdayKey as CNKeyDescriptor]
    }

    public func authorization(requesting: Bool) async -> DeviceContactsAccess {
        let current = DeviceContactsAccess(CNContactStore.authorizationStatus(for: .contacts))
        guard requesting, current == .notDetermined else { return current }
        guard Self.hasUsageDescription(in: Bundle.main.infoDictionary) else { return .restricted }
        _ = try? await store.requestAccess(for: .contacts)
        return DeviceContactsAccess(CNContactStore.authorizationStatus(for: .contacts))
    }

    public func contacts() async throws -> [DeviceContact] {
        try check()
        let store = self.store
        return try await Task.detached(priority: .userInitiated) {
            // enumerateContacts は同期で重いので主スレッドの外で回す
            let groups = try store.groups(matching: nil)
            var members: [String: [String]] = [:]
            for g in groups {
                let request = CNContactFetchRequest(keysToFetch: [CNContactIdentifierKey as CNKeyDescriptor])
                request.predicate = CNContact.predicateForContactsInGroup(withIdentifier: g.identifier)
                var ids: [String] = []
                try store.enumerateContacts(with: request) { c, _ in ids.append(c.identifier) }
                members[g.identifier] = ids
            }
            let byContact = DeviceContactsQuery.groupIds(byContact: members)
            var result: [DeviceContact] = []
            let request = CNContactFetchRequest(keysToFetch: Self.keysToFetch)
            request.unifyResults = true
            try store.enumerateContacts(with: request) { c, _ in
                result.append(DeviceContact(
                    id: c.identifier,
                    name: CNContactFormatter.string(from: c, style: .fullName) ?? "",
                    birthday: DeviceContactsQuery.birthday(c.birthday),
                    groupIds: byContact[c.identifier] ?? []
                ))
            }
            return DeviceContactsQuery.sorted(result)
        }.value
    }

    public func groups() async throws -> [DeviceContactGroup] {
        try check()
        let groups = try store.groups(matching: nil).map { DeviceContactGroup(id: $0.identifier, name: $0.name) }
        return DeviceContactsQuery.sortedGroups(groups)
    }

    private func check() throws {
        let a = DeviceContactsAccess(CNContactStore.authorizationStatus(for: .contacts))
        guard a.canRead else { throw DeviceContactsError.notAuthorized(a) }
    }
}
