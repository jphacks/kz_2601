import UIKit
import Vision
import MetamonDSL

/// `photoField.analyze`(#418)の端末内の画像認識。Vision だけを使い、写真は端末外に出ない。
/// Vision に触るのはこのファイルだけ。シミュレータで使えない要求(画像分類など)は空の結果に落とす。
enum PhotoVision {
    /// 文字認識(日本語 + 英語)。同じ高さの断片(「合計」と「¥493」など)を 1 行にまとめ、上から順に返す。
    static func textLines(in image: CGImage, orientation: CGImagePropertyOrientation = .up) -> [String] {
        let request = VNRecognizeTextRequest()
        request.recognitionLevel = .accurate
        request.recognitionLanguages = ["ja-JP", "en-US"]
        request.usesLanguageCorrection = true
        guard (try? VNImageRequestHandler(cgImage: image, orientation: orientation).perform([request])) != nil else { return [] }
        let pieces = (request.results ?? []).compactMap { obs -> OCRPiece? in
            obs.topCandidates(1).first.map { OCRPiece(text: $0.string, box: obs.boundingBox) }
        }
        return OCRPiece.lines(pieces)
    }

    /// 画像分類のラベル(英語・信頼度順・上位 10)。
    static func labels(in image: CGImage, orientation: CGImagePropertyOrientation = .up) -> [String] {
        let request = VNClassifyImageRequest()
        guard (try? VNImageRequestHandler(cgImage: image, orientation: orientation).perform([request])) != nil else { return [] }
        return (request.results ?? [])
            .filter { $0.confidence >= 0.1 }
            .sorted { $0.confidence > $1.confidence }
            .prefix(10)
            .map(\.identifier)
    }

    /// 写っている人の骨格(一番はっきり写っている 1 人)。座標は Vision の正規化座標(左下原点)。
    static func bodyPose(in image: CGImage, orientation: CGImagePropertyOrientation = .up) -> BodyPose? {
        let request = VNDetectHumanBodyPoseRequest()
        guard (try? VNImageRequestHandler(cgImage: image, orientation: orientation).perform([request])) != nil,
              let best = (request.results ?? []).max(by: { $0.confidence < $1.confidence }),
              let points = try? best.recognizedPoints(.all) else { return nil }
        var pose: BodyPose = [:]
        for (key, name) in jointNames {
            if let p = points[name], p.confidence > 0 {
                pose[key] = PosePoint(x: Double(p.location.x), y: Double(p.location.y), confidence: Double(p.confidence))
            }
        }
        return pose.isEmpty ? nil : pose
    }

    static let jointNames: [(PoseKeypoint, VNHumanBodyPoseObservation.JointName)] = [
        (.nose, .nose), (.neck, .neck), (.root, .root),
        (.leftShoulder, .leftShoulder), (.rightShoulder, .rightShoulder),
        (.leftElbow, .leftElbow), (.rightElbow, .rightElbow),
        (.leftWrist, .leftWrist), (.rightWrist, .rightWrist),
        (.leftHip, .leftHip), (.rightHip, .rightHip),
        (.leftKnee, .leftKnee), (.rightKnee, .rightKnee),
        (.leftAnkle, .leftAnkle), (.rightAnkle, .rightAnkle),
    ]
}

/// OCR の 1 断片(Vision の正規化座標・左下原点)。
struct OCRPiece: Equatable {
    var text: String
    var box: CGRect

    /// 縦の中心が近い断片(高さの半分以内)を 1 行にし、左から並べる。行は上から。
    static func lines(_ pieces: [OCRPiece]) -> [String] {
        var rows: [[OCRPiece]] = []
        for p in pieces.sorted(by: { $0.box.midY > $1.box.midY }) {
            if let i = rows.indices.last, let first = rows[i].first,
               abs(first.box.midY - p.box.midY) < Swift.max(first.box.height, p.box.height) / 2 {
                rows[i].append(p)
            } else {
                rows.append([p])
            }
        }
        return rows.map { $0.sorted { $0.box.minX < $1.box.minX }.map(\.text).joined(separator: " ") }
    }
}

/// 読み取りの結果。values は into の field id → 保存形式の値。
struct PhotoReadOutcome: Sendable {
    var values: [String: JSONValue] = [:]
    /// 読めなかった into の field id
    var missed: [String] = []
    var pose: BodyPose?
}

/// `analyze` の全項目を 1 枚の写真に対して実行する。重い処理は呼び出し側の Task から detached で動かす。
enum PhotoReader {
    static func read(_ image: UIImage, specs: [PhotoAnalyze], entity: Entity) async -> PhotoReadOutcome {
        guard let cg = image.cgImage else { return PhotoReadOutcome(missed: specs.map(\.into)) }
        let aspect = Double(cg.width) / Double(Swift.max(cg.height, 1))
        let (lines, labels, pose) = await Task.detached(priority: .userInitiated) {
            (specs.contains(where: \.needsText) ? PhotoVision.textLines(in: cg) : [],
             specs.contains(where: \.needsLabels) ? PhotoVision.labels(in: cg) : [],
             specs.contains { $0.kind == .jointAngle } ? PhotoVision.bodyPose(in: cg) : nil)
        }.value

        var out = PhotoReadOutcome(pose: pose)
        for spec in specs {
            guard let field = entity.field(spec.into),
                  let raw = await reading(spec, field: field, lines: lines, labels: labels, pose: pose, aspect: aspect),
                  let value = raw.jsonValue(for: field.type) else {
                out.missed.append(spec.into)
                continue
            }
            out.values[spec.into] = value
        }
        return out
    }

    static func reading(_ spec: PhotoAnalyze, field: Field, lines: [String], labels: [String], pose: BodyPose?, aspect: Double) async -> PhotoReadingValue? {
        let text = lines.joined(separator: "\n")
        switch spec.kind {
        case .text:
            return text.isEmpty ? nil : .text(text)
        case .amount:
            return ReceiptParser.amount(lines: lines).map(PhotoReadingValue.number)
        case .date:
            return ReceiptParser.date(lines: lines).map(PhotoReadingValue.date)
        case .classify:
            let options = spec.options ?? field.options ?? []
            guard options.count >= 2, !(labels.isEmpty && text.isEmpty) else { return nil }
            if AIAvailability.current.isAvailable,
               let picked = try? await OnDeviceAI.run(classifyRequest(options: options, labels: labels, text: text)) {
                return .text(picked)
            }
            return PhotoClassifier.classify(options: options, labels: labels, text: text).map(PhotoReadingValue.text)
        case .describe:
            guard !(labels.isEmpty && text.isEmpty) else { return nil }
            if AIAvailability.current.isAvailable, let s = try? await OnDeviceAI.run(describeRequest(labels: labels, text: text)) {
                return .text(s)
            }
            let words = labels.prefix(5).map(PhotoClassifier.japanese)
            return words.isEmpty ? .text(String(text.prefix(200))) : .text(L("写っているもの: ") + words.joined(separator: "、"))
        case .jointAngle:
            guard let pose, let joint = spec.joint else { return nil }
            return PoseAngle.angle(joint: joint, in: pose, aspect: aspect).map(PhotoReadingValue.number)
        case .unknown:
            return nil
        }
    }

    /// 写真の手がかり(ラベル + 文字)を Foundation Models に渡す本文。
    static func clues(labels: [String], text: String) -> String {
        var s = "写真の画像認識のラベル(英語・確からしい順): " + (labels.isEmpty ? "なし" : labels.joined(separator: ", "))
        if !text.isEmpty { s += "\n写真の中の文字:\n" + String(text.prefix(AIRequestBuilder.maxInputCharacters)) }
        return s
    }

    static func classifyRequest(options: [String], labels: [String], text: String) -> AIRequest {
        AIRequest(task: .classify,
                  instructions: "あなたは写真の分類アシスタントです。写真の手がかりから、最も当てはまる候補を 1 つだけ選んでください。",
                  prompt: clues(labels: labels, text: text) + "\n\n候補: " + options.joined(separator: " / "),
                  options: options)
    }

    static func describeRequest(labels: [String], text: String) -> AIRequest {
        AIRequest(task: .summarize,
                  instructions: "あなたは写真の説明アシスタントです。写真の手がかりから、何が写っているかを日本語の短い 1 文で説明してください。結果だけを返し、手がかりに無いことは書かないでください。",
                  prompt: clues(labels: labels, text: text),
                  options: [])
    }
}
