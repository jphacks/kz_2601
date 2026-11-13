import Contacts
import Foundation
import MetamonDSL
#if os(iOS)
import ContactsUI
import SwiftUI
#endif

/// 連絡先から 1 人選ぶ口(#2217・親 #1804)。部品・アクションはまだ無い(決定待ち)。
///
/// `CNContactPickerViewController` は許可が要らない: 選んだ 1 人だけがアプリに渡る仕組みなので、
/// ダイアログも `NSContactsUsageDescription` も要らない。`CNContactStore` は**使わない**
/// (store で一覧を読むと許可ダイアログが出るため。この口の肝)。
///
/// 選んだ値はログ・クラッシュ報告・サーバーに出さない(`description` も伏せる)。

/// 選んだ 1 人から取り出す項目。
public enum ContactItem: String, CaseIterable, Sendable {
    case name
    case phone
    case email
    case birthday
}

/// 選んだ 1 人から取り出した値。無かった項目は入らない(空として扱う)。
public struct PickedContact: Sendable, Equatable, CustomStringConvertible, CustomDebugStringConvertible {
    public var values: [ContactItem: String]

    public init(values: [ContactItem: String] = [:]) {
        self.values = values
    }

    public subscript(item: ContactItem) -> String? { values[item] }

    public var isEmpty: Bool { values.isEmpty }

    /// 中身は出さない(誤ってログに入れても漏れない)
    public var description: String { "PickedContact(items: \(values.keys.map(\.rawValue).sorted()))" }
    public var debugDescription: String { description }
}

public enum ContactPick {
    /// 選んだ 1 人から、頼まれた項目だけを取り出す(単体テストの対象)。
    /// 電話・メールが複数ある時は最初の 1 つ。無い項目・空の項目は入れない。
    public static func read(_ contact: CNContact, items: Set<ContactItem>) -> PickedContact {
        var values: [ContactItem: String] = [:]
        for item in items {
            let text = value(of: item, in: contact)
            if !text.isEmpty { values[item] = text }
        }
        return PickedContact(values: values)
    }

    static func value(of item: ContactItem, in contact: CNContact) -> String {
        switch item {
        case .name: fullName(contact)
        case .phone: firstPhone(contact)
        case .email: firstEmail(contact)
        case .birthday: birthday(contact)
        }
    }

    /// 表示用の氏名(地域の並び順)。取れなければ姓名を並べる。
    static func fullName(_ contact: CNContact) -> String {
        let descriptor = CNContactFormatter.descriptorForRequiredKeys(for: .fullName)
        if contact.areKeysAvailable([descriptor]),
           let formatted = CNContactFormatter.string(from: contact, style: .fullName) {
            let trimmed = trim(formatted)
            if !trimmed.isEmpty { return trimmed }
        }
        let parts = [key(CNContactFamilyNameKey, in: contact, default: "") { $0.familyName },
                     key(CNContactGivenNameKey, in: contact, default: "") { $0.givenName }]
        return trim(parts.filter { !$0.isEmpty }.joined(separator: " "))
    }

    /// 電話が複数の時は最初の 1 つ。
    static func firstPhone(_ contact: CNContact) -> String {
        let numbers = key(CNContactPhoneNumbersKey, in: contact, default: []) { $0.phoneNumbers }
        return trim(numbers.first?.value.stringValue ?? "")
    }

    /// メールが複数の時は最初の 1 つ。
    static func firstEmail(_ contact: CNContact) -> String {
        let addresses = key(CNContactEmailAddressesKey, in: contact, default: []) { $0.emailAddresses }
        return trim(addresses.first.map { $0.value as String } ?? "")
    }

    /// 誕生日は date の field と同じ `YYYY-MM-DD`。年が無い誕生日は入れない(日付として表せないため)。
    static func birthday(_ contact: CNContact) -> String {
        let components = key(CNContactBirthdayKey, in: contact, default: nil) { $0.birthday }
        guard let components, let year = components.year, let month = components.month, let day = components.day else { return "" }
        return String(format: "%04d-%02d-%02d", year, month, day)
    }

    /// 取っていない項目を読むと落ちるので、`isKeyAvailable` を見てから読む。
    private static func key<T>(_ name: String, in contact: CNContact, default fallback: T, _ read: (CNContact) -> T) -> T {
        contact.isKeyAvailable(name) ? read(contact) : fallback
    }

    private static func trim(_ s: String) -> String {
        s.trimmingCharacters(in: .whitespacesAndNewlines)
    }

    /// 選んだ値を field に入れる形にする(単体テストの対象)。
    /// `fields` は項目 → field の id。値が空の項目は入れない(= 項目が無い時は空の辞書)。
    public static func prefillValues(_ picked: PickedContact, fields: [ContactItem: String]) -> [String: JSONValue] {
        var values: [String: JSONValue] = [:]
        for (item, fieldId) in fields {
            guard !fieldId.isEmpty, let text = picked[item], !text.isEmpty else { continue }
            values[fieldId] = .string(text)
        }
        return values
    }

    /// 次に開く新規フォームへ積む。空なら何もしない(前に積んだ値を消さない)。
    @MainActor
    public static func prefill(_ picked: PickedContact, fields: [ContactItem: String]) -> [String: JSONValue] {
        let values = prefillValues(picked, fields: fields)
        if !values.isEmpty { FormPrefill.set(values) }
        return values
    }
}

#if os(iOS)
/// 連絡先を 1 人選ぶ画面(`.sheet` に入れる)。選んだ 1 人から `items` の項目だけを返す。
/// `CNContactStore` を触らないので許可ダイアログは出ない。
public struct ContactPickerSheet: UIViewControllerRepresentable {
    public var items: Set<ContactItem>
    public var onPick: (PickedContact) -> Void
    @Environment(\.dismiss) private var dismiss

    public init(items: Set<ContactItem> = Set(ContactItem.allCases), onPick: @escaping (PickedContact) -> Void) {
        self.items = items
        self.onPick = onPick
    }

    public func makeCoordinator() -> Coordinator { Coordinator(self) }

    public func makeUIViewController(context: Context) -> CNContactPickerViewController {
        let picker = CNContactPickerViewController()
        picker.delegate = context.coordinator
        return picker
    }

    public func updateUIViewController(_ uiViewController: CNContactPickerViewController, context: Context) {}

    public final class Coordinator: NSObject, CNContactPickerDelegate {
        private let parent: ContactPickerSheet
        init(_ parent: ContactPickerSheet) { self.parent = parent }

        public func contactPicker(_ picker: CNContactPickerViewController, didSelect contact: CNContact) {
            parent.onPick(ContactPick.read(contact, items: parent.items))
            parent.dismiss()
        }

        public func contactPickerDidCancel(_ picker: CNContactPickerViewController) {
            parent.dismiss()
        }
    }
}
#endif
