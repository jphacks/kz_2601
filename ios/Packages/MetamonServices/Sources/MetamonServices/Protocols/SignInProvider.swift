import Foundation

/// アカウントにつなげるサインインの方法(#361)。値は Firebase の providerID。
public enum SignInProvider: String, Sendable, CaseIterable, Identifiable {
    case apple = "apple.com"
    case google = "google.com"

    public var id: String { rawValue }

    /// Firebase の providerID の並びから、知っているものだけを取り出す(password・phone などは無視)。
    public static func from(providerIDs: [String]) -> [SignInProvider] {
        allCases.filter { providerIDs.contains($0.rawValue) }
    }
}
