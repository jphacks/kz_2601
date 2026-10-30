import SwiftUI
import MetamonDSL

// MARK: - 入力系バッチ(slider / stepper / rating / timePicker / colorPicker)の束縛

extension FormDraft {
    /// 数値 field の Double 束縛。未入力は `fallback`。integer/currency field は丸めて保存。
    func double(_ field: String, fallback: Double) -> Binding<Double> {
        let isInteger = entity.field(field)?.type != .number
        return Binding(
            get: {
                let v = self.values[field] ?? .null
                return v.doubleValue ?? v.stringValue.flatMap(Double.init) ?? fallback
            },
            set: { self.values[field] = .number(isInteger ? $0.rounded() : $0) })
    }

    /// string/text field に "HH:mm" で保存する時刻束縛(日付部分は今日・表示専用)。
    func time(_ field: String) -> Binding<Date> {
        Binding(
            get: {
                guard let s = (self.values[field] ?? .null).stringValue, let d = Self.timeFormatter.date(from: s) else { return Date() }
                let hm = Calendar.current.dateComponents([.hour, .minute], from: d)
                return Calendar.current.date(bySettingHour: hm.hour ?? 0, minute: hm.minute ?? 0, second: 0, of: Date()) ?? Date()
            },
            set: { self.values[field] = .string(Self.timeFormatter.string(from: $0)) })
    }

    /// timePicker は未入力でも今の時刻を表示する。見えている時刻をそのまま保存値にする(#229:
    /// 触らずに保存すると出勤時刻が空のまま = 勤務時間・給料が「—」になっていた)。入力済みなら何もしない。
    func seedTimeIfEmpty(_ field: String, now: Date = Date()) {
        guard let f = entity.field(field), FieldCoding.normalize(values[field] ?? .null, for: f) == nil else { return }
        if f.type == .datetime {
            values[field] = .string(FieldCoding.encode(now, for: f))
        } else {
            values[field] = .string(Self.timeFormatter.string(from: now))
        }
    }

    /// string/text field に "#RRGGBB" で保存する色束縛。
    func color(_ field: String) -> Binding<Color> {
        Binding(
            get: { Color(hex: (self.values[field] ?? .null).stringValue) ?? .accentColor },
            set: { self.values[field] = .string($0.hexString) })
    }

    static let timeFormatter: DateFormatter = {
        let f = DateFormatter()
        f.locale = Locale(identifier: "en_US_POSIX")
        f.dateFormat = "HH:mm"
        return f
    }()
}

extension Color {
    /// "#RRGGBB"(不透明度は捨てる)。
    var hexString: String {
        var r: CGFloat = 0, g: CGFloat = 0, b: CGFloat = 0, a: CGFloat = 0
        UIColor(self).getRed(&r, green: &g, blue: &b, alpha: &a)
        let to255 = { (v: CGFloat) in Int((Swift.max(0, Swift.min(1, v)) * 255).rounded()) }
        return String(format: "#%02X%02X%02X", to255(r), to255(g), to255(b))
    }
}
