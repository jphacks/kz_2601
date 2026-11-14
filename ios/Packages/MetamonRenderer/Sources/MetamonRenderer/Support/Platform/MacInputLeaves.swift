import Foundation
#if os(macOS)
import AppKit
#endif

/// Mac の入力 leaf の決まり(#942)。物理キーボード前提の数値入力・ピッカーの形・日本語変換中の Return。
/// 判定だけの純粋な関数。iOS の描画からは呼ばない。

/// numberField: 数字以外を弾き、↑↓ で step ずつ増減する。
enum MacNumberInput {
    /// 欄に許す文字だけ残す。全角数字は半角に。マイナスは先頭 1 つ、小数点は 1 つだけ。
    static func filter(_ text: String, allowsDecimal: Bool, allowsNegative: Bool) -> String {
        var out = ""
        var hasPoint = false
        for ch in text {
            let c = halfWidth(ch)
            if c.isASCII, c.isNumber {
                out.append(c)
            } else if c == "-", allowsNegative, out.isEmpty {
                out.append(c)
            } else if c == ".", allowsDecimal, !hasPoint {
                hasPoint = true
                out.append(c)
            }
        }
        return out
    }

    /// ↑(+1)・↓(-1)で step ずつ動かした文字。min/max に収め、小数の誤差は step の桁で丸める。
    static func stepped(_ text: String, direction: Int, min: Double?, max: Double?, step: Double?, allowsDecimal: Bool) -> String {
        let s = NumericInput.step(step) ?? 1
        let lo = min.flatMap { $0.isFinite ? $0 : nil }
        let hi = max.flatMap { $0.isFinite ? $0 : nil }.flatMap { h in lo.map { h >= $0 ? h : nil } ?? h }
        let current = Double(text.replacingOccurrences(of: ",", with: "")).flatMap { $0.isFinite ? $0 : nil }
        // 空欄は 0 から動かす(範囲外なら近い端へ)
        var v = (current ?? 0) + Double(direction.signum()) * s
        if let lo, v < lo { v = lo }
        if let hi, v > hi { v = hi }
        return format(v, places: allowsDecimal ? decimals(of: s) : 0)
    }

    /// 表示の文字。整数なら ".0" を付けない。
    static func format(_ v: Double, places: Int) -> String {
        if places == 0 { return String(NumericInput.clampedInt(v)) }
        var s = String(format: "%.\(places)f", v)
        while s.hasSuffix("0") { s.removeLast() }
        if s.hasSuffix(".") { s.removeLast() }
        return s == "-0" ? "0" : s
    }

    /// step の小数の桁(0.25 → 2)。最大 6 桁。
    static func decimals(of step: Double) -> Int {
        for p in 0...6 {
            let scaled = step * pow(10, Double(p))
            if abs(scaled - scaled.rounded()) < 1e-9 { return p }
        }
        return 6
    }

    private static func halfWidth(_ ch: Character) -> Character {
        switch ch {
        case "０"..."９":
            let base = UnicodeScalar("0").value + (ch.unicodeScalars.first!.value - UnicodeScalar("０").value)
            return Character(UnicodeScalar(base)!)
        case "－", "ー", "−": return "-"
        case "．", "。": return "."
        default: return ch
        }
    }
}

/// picker の Mac での形。DSL の style があればそれ、無ければ 4 件以下はラジオ・5 件以上はメニュー。
enum MacPickerStyle: Equatable {
    case menu, radioGroup, segmented

    static func choose(hint: String?, optionCount: Int) -> MacPickerStyle {
        switch hint {
        case "menu": return .menu
        case "segmented": return .segmented
        case "radio", "radioGroup": return .radioGroup
        default: return optionCount <= 4 ? .radioGroup : .menu  // wheel は Mac に無い
        }
    }
}

/// datePicker / timePicker の Mac での形。日付は欄 + クリックでカレンダー、時刻だけは欄 + ステッパー(iOS のホイールにしない)。
enum MacDatePickerStyle: Equatable {
    case fieldWithCalendar, fieldWithStepper

    static func choose(includesDate: Bool) -> MacDatePickerStyle {
        includesDate ? .fieldWithCalendar : .fieldWithStepper
    }
}

/// 未確定の文字(日本語の変換中)を持つか。NSTextView(フィールドエディタ)が満たす。テストでは偽物を渡す。
@MainActor
protocol MarkedTextProviding: AnyObject {
    func hasMarkedText() -> Bool
}

/// 変換中の Return は変換の確定なので、次の欄へ進めたり送ったりしない。
@MainActor
enum IMEReturnGuard {
    static func allowsReturn(_ editor: MarkedTextProviding?) -> Bool {
        !(editor?.hasMarkedText() ?? false)
    }
}

#if os(macOS)
extension NSTextView: MarkedTextProviding {}

extension NSTextView {
    /// 変換中の文字を確定させて全文を返す(#1047)。文書に残すだけでなく入力メソッドの変換も終えさせ、
    /// 送った・保存した後に同じ文字が書き戻されて二重に残らないようにする。
    @discardableResult
    public func commitMarkedTextNow() -> String {
        if hasMarkedText() {
            unmarkText()
            inputContext?.discardMarkedText()
        }
        return string
    }
}

extension KeyboardState {
    /// 今の入力欄で Return を「送る・次へ」に使ってよいか。変換中の Return は確定だけ(#1047)。
    public static func allowsReturnSubmit() -> Bool {
        IMEReturnGuard.allowsReturn(focusedTextView())
    }
}
#endif
