#if METAMON_STG
import ObjectiveC
import SwiftUI

/// 開発用メニューの 1 セクション(#2079)。足す時は 1 ファイルに 1 つ、この型の subclass を書くだけ。
/// メニューは起動中のクラスから subclass を探して並べる(`DebugMenuRegistry`)ので、共有の一覧は書き換えない。
///
///     final class DebugFooSection: DebugMenuSection {
///         override class var order: Int { 300 }
///         override class var title: String { "Foo" }
///         override class func makeBody() -> AnyView { AnyView(DebugFooRows()) }
///         override class func reset() { /* metamon.debug.* の外に保存したものを戻す */ }
///     }
class DebugMenuSection: NSObject {
    /// 小さいほど上。同じなら型の名前の順
    class var order: Int { 1000 }
    class var title: String { "" }
    class var footer: String? { nil }

    /// セクションの中身(Section の中に入る行)
    @MainActor class func makeBody() -> AnyView { AnyView(EmptyView()) }

    /// 「全部元に戻す」で呼ばれる。`DebugKnobs`(metamon.debug.*)はメニューが消すので、それ以外に保存したものだけ戻す
    @MainActor class func reset() {}
}

/// つまみが効くまで
enum DebugKnobEffect {
    /// すぐ効く
    case immediate
    /// 起動し直しが要る(変えたらメニューに「アプリを終了する」を出す)
    case relaunch

    var label: String {
        switch self {
        case .immediate: "すぐ効く"
        case .relaunch: "起動し直しが要る"
        }
    }
}

/// メニューの状態。起動し直しが要るつまみを変えたら `markNeedsRelaunch()`
@MainActor @Observable
final class DebugMenuState {
    static let shared = DebugMenuState()
    private(set) var needsRelaunch = false

    func markNeedsRelaunch() { needsRelaunch = true }
}

/// subclass を探して並べる
enum DebugMenuRegistry {
    @MainActor static var sections: [DebugMenuSection.Type] {
        if let cached { return cached }
        let found = ordered(discover())
        cached = found
        return found
    }

    @MainActor private static var cached: [DebugMenuSection.Type]?

    /// 起動中のクラスから `DebugMenuSection` の subclass(孫も)を集める。親をたどるだけなので、ほかのクラスのメソッドは呼ばない
    static func discover() -> [DebugMenuSection.Type] {
        var count: UInt32 = 0
        guard let list = objc_copyClassList(&count) else { return [] }
        let raw = UnsafeMutableRawPointer(list)
        defer { free(raw) }
        // Autoreleasing のまま読むと各クラスに retain が飛び、retain を持たない根のクラス(__NSGenericDeallocHandler など)で落ちる
        let classes = raw.assumingMemoryBound(to: AnyClass.self)
        let base: AnyClass = DebugMenuSection.self
        var found: [DebugMenuSection.Type] = []
        for i in 0..<Int(count) {
            let cls: AnyClass = classes[i]
            var parent: AnyClass? = class_getSuperclass(cls)
            while let p = parent, ObjectIdentifier(p) != ObjectIdentifier(base) { parent = class_getSuperclass(p) }
            if parent != nil, let section = cls as? DebugMenuSection.Type { found.append(section) }
        }
        return found
    }

    static func ordered(_ types: [DebugMenuSection.Type]) -> [DebugMenuSection.Type] {
        types.sorted { a, b in
            a.order != b.order ? a.order < b.order : NSStringFromClass(a) < NSStringFromClass(b)
        }
    }
}

/// つまみの行の見出し(名前と「すぐ効く / 起動し直しが要る」)。後の Issue の項目もこれを使う
struct DebugKnobLabel: View {
    var title: String
    var effect: DebugKnobEffect
    /// 起動時の環境変数で決まっている時はその名前(メニューで変えても効かない)
    var envName: String?

    var body: some View {
        VStack(alignment: .leading, spacing: 2) {
            Text(verbatim: title)
            Text(verbatim: caption)
                .font(.caption)
                .foregroundStyle(effect == .relaunch ? AnyShapeStyle(Theme.Colors.warning) : AnyShapeStyle(.secondary))
        }
    }

    private var caption: String {
        if let envName, DebugKnobs.isFromEnvironment(envName) { return "\(effect.label)・環境変数 \(envName) が優先" }
        return effect.label
    }
}
#endif
