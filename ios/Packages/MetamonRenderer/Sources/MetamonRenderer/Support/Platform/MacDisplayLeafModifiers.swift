import SwiftUI

// 表示 leaf・一覧 leaf の Mac だけの操作(#941・#944)。iOS は何もしない。決まりは MacDisplayLeaves.swift。

#if os(macOS)
extension MacLeafKey {
    init(_ press: KeyPress) {
        switch press.key {
        case .leftArrow: self = .left
        case .rightArrow: self = .right
        case .upArrow: self = .up
        case .downArrow: self = .down
        case .space: self = .space
        case .delete, .deleteForward: self = .delete
        default:
            // 修飾キー付き(⌘1 など)はメニューへ渡す
            if press.modifiers.isDisjoint(with: [.command, .control, .option]),
               let ch = press.characters.first, let d = ch.wholeNumberValue, ch.isASCII {
                self = .digit(d)
            } else {
                self = .other
            }
        }
    }
}
#endif

extension View {
    /// Mac: 文字を選んでコピーできるようにする(text・markdown)。
    /// 「この部品について話す」(#1323)がある時は、選べる文字の右クリックが OS の文字のメニューになって話す項目が隠れるので、
    /// 選ぶ代わりに自前のメニュー(話す・全文をコピー)を出す。`copy` は全文(コピーの項目で使う)。
    @ViewBuilder
    func macTextSelection(copy: @escaping () -> String) -> some View {
        #if os(macOS)
        modifier(MacSelectableText(copy: copy))
        #else
        self
        #endif
    }

    /// Mac: rating をキーボードと VoiceOver で付ける。Tab で入り、←→↑↓ で 1 つずつ・数字キーでその数(0 と delete で消す)。
    /// VoiceOver は 1 つの調整できる部品として「評価, 5 段階中 3」と読み、上下の操作で増減する。
    @ViewBuilder
    func macRatingControl(label: String?, current: Int, count: Int, set: @escaping (Int) -> Void) -> some View {
        #if os(macOS)
        focusable()
            .onKeyPress { press in
                guard let next = MacRatingKeys.value(after: MacLeafKey(press), current: current, count: count) else { return .ignored }
                set(next)
                return .handled
            }
            .accessibilityElement(children: .ignore)
            .accessibilityLabel(label.flatMap { $0.isEmpty ? nil : $0 } ?? L("評価"))
            .accessibilityValue(L("\(count) 段階中 \(current)"))
            .accessibilityAdjustableAction { direction in
                switch direction {
                case .increment: set(MacRatingKeys.adjusted(increment: true, current: current, count: count))
                case .decrement: set(MacRatingKeys.adjusted(increment: false, current: current, count: count))
                @unknown default: break
                }
            }
        #else
        self
        #endif
    }

    /// Mac: flashcards のカードを Space でめくり、←→ で「まだ」「覚えた」。カードをクリックするとキーを受ける。
    @ViewBuilder
    func macFlashcardKeys(focused: FocusState<Bool>.Binding, perform: @escaping (MacFlashcardKeys.Command) -> Void) -> some View {
        #if os(macOS)
        focusable()
            .focused(focused)
            .onKeyPress { press in
                guard let command = MacFlashcardKeys.command(for: MacLeafKey(press)) else { return .ignored }
                perform(command)
                return .handled
            }
        #else
        self
        #endif
    }

    /// Mac: calendar の月の表で矢印キーで日を動かす(日をクリックするか Tab で入る)。
    @ViewBuilder
    func macCalendarKeys(focused: FocusState<Bool>.Binding, move: @escaping (MacLeafKey) -> Bool) -> some View {
        #if os(macOS)
        focusable()
            .focused(focused)
            .onKeyPress { press in move(MacLeafKey(press)) ? .handled : .ignored }
        #else
        self
        #endif
    }
}

/// 選べる文字にするか: 部品について話せる時(話す先がある)は選ばせず、右クリックを自前のメニューにする
enum MacTextSelectionPolicy {
    static func selectable(canTalk: Bool) -> Bool { !canTalk }
}

#if os(macOS)
private struct MacSelectableText: ViewModifier {
    @Environment(\.componentTalk) private var talk
    @Environment(\.componentTalkTarget) private var target
    let copy: () -> String

    func body(content: Content) -> some View {
        if MacTextSelectionPolicy.selectable(canTalk: talk != nil && target != nil) {
            content.textSelection(.enabled)
        } else {
            content.contextMenu {
                ComponentTalkImageMenuItem() // 外側の部品のメニューより先に出るので、ここにも置く
                Button(L("コピー"), systemImage: "doc.on.doc") { PlatformPasteboard.setString(copy()) }
            }
        }
    }
}
#endif
