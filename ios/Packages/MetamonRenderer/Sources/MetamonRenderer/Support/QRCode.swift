import UIKit
import CoreImage
import CoreImage.CIFilterBuiltins
import MetamonDSL

/// `qrCode`(#422)の画像。CoreImage `CIQRCodeGenerator`(誤り訂正 M)の 1 モジュール = 1px を整数倍に拡大し、
/// 補間なしで白地に描く(ぼけない)。周りに 4 モジュールの余白(読み取りに要る quiet zone)。
enum QRCodeImage {
    static let quietZone = 4
    /// 保存・共有する画像の目安の辺(px)
    static let exportSide = 1024

    private static let context = CIContext(options: [.useSoftwareRenderer: false])
    private static let cache: NSCache<NSString, UIImage> = {
        let c = NSCache<NSString, UIImage>()
        c.countLimit = 32
        return c
    }()

    /// 1 モジュール = 1px・余白なしの QR。空・容量超え(バージョン 40 に入らない)は nil。
    static func modules(for text: String) -> CGImage? {
        guard !text.isEmpty else { return nil }
        let f = CIFilter.qrCodeGenerator()
        f.message = Data(text.utf8)
        f.correctionLevel = "M"
        guard let out = f.outputImage, !out.extent.isEmpty else { return nil }
        return context.createCGImage(out, from: out.extent)
    }

    /// 1 モジュール `scale` px(余白込み)の白地・黒の QR 画像。`scale` nil = 辺が `exportSide` 前後になる倍率。
    static func image(for text: String, scale: Int? = nil) -> UIImage? {
        let key = "\(scale ?? 0)|\(text)" as NSString
        if let hit = cache.object(forKey: key) { return hit }
        guard let cg = modules(for: text) else { return nil }
        let side = cg.width + quietZone * 2
        let s = max(1, scale ?? exportSide / side)
        let format = UIGraphicsImageRendererFormat.default()
        format.scale = 1
        format.opaque = true
        let px = CGFloat(side * s)
        let img = UIGraphicsImageRenderer(size: CGSize(width: px, height: px), format: format).image { ctx in
            UIColor.white.setFill()
            ctx.fill(CGRect(x: 0, y: 0, width: px, height: px))
            ctx.cgContext.interpolationQuality = .none
            let inset = CGFloat(quietZone * s)
            UIImage(cgImage: cg).draw(in: CGRect(x: inset, y: inset, width: CGFloat(cg.width * s), height: CGFloat(cg.height * s)))
        }
        cache.setObject(img, forKey: key)
        return img
    }
}

/// `qrCode` の中身(text / field / value のどれか 1 つ)。screen.entity の画面では表示中のレコード(フォームのドラフト)を読む。
enum QRCodeContent {
    @MainActor
    static func resolve(_ c: Component, form: FormDraft?, evaluator: () -> ExprEvaluator) -> String {
        let row = form.map { ExprEvaluator.Row(entity: $0.entity, record: Record(id: $0.recordId ?? "draft", data: $0.values), memoizable: false) }
        if let text = c.text, !text.isEmpty {
            guard let form, text.contains("{{") else { return text }
            var s = text
            for f in form.entity.fields where s.contains("{{\(f.id)}}") {
                s = s.replacingOccurrences(of: "{{\(f.id)}}", with: fieldText(f.id, form: form, row: row, evaluator: evaluator))
            }
            return s
        }
        if let id = c.field, !id.isEmpty {
            guard let form else { return "" }
            return fieldText(id, form: form, row: row, evaluator: evaluator)
        }
        if let expr = c.valueExpr {
            return evaluator().evaluate(expr, row: row)?.text ?? ""
        }
        return ""
    }

    /// 保存値はそのまま(日時・数値を整形しない)、computed は評価。
    @MainActor
    private static func fieldText(_ id: String, form: FormDraft, row: ExprEvaluator.Row?, evaluator: () -> ExprEvaluator) -> String {
        if form.entity.field(id)?.computed != nil, let row {
            return evaluator().fieldValue(id, row: row)?.text ?? ""
        }
        return (form.values[id] ?? .null).displayString
    }
}
