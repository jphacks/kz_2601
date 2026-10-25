import Foundation
import MetamonDSL
#if canImport(FoundationModels)
import FoundationModels
#endif

/// `aiButton`(#78)のオンデバイス AI。Apple Intelligence の Foundation Models(iOS 26+)だけで動き、データは端末外に出ない。
/// FoundationModels に触るのはこのファイルだけ(iOS 17 でもビルド・起動できるよう `canImport` + `#available` で囲う)。

// MARK: - 可用性

/// AI を使えるか。使えない時は理由をボタンの下に出す。
enum AIAvailability: Equatable, Sendable {
    case available
    case unavailable(Reason)

    enum Reason: Equatable, Sendable {
        /// Apple Intelligence 非対応の端末
        case deviceNotEligible
        /// 対応端末だが設定で Apple Intelligence が OFF
        case appleIntelligenceNotEnabled
        /// モデルのダウンロード・準備中
        case modelNotReady
        /// iOS 26 未満(FoundationModels が無い)
        case osTooOld
        /// SDK が返した未知の理由
        case other
    }

    static let title = "この端末では AI を使えません"

    var isAvailable: Bool { self == .available }

    /// ボタンの下に出す理由(使える時は nil)。
    var message: String? {
        guard case .unavailable(let reason) = self else { return nil }
        switch reason {
        case .deviceNotEligible: return "Apple Intelligence に対応していない端末です"
        case .appleIntelligenceNotEnabled: return "設定 > Apple Intelligence と Siri でオンにしてください"
        case .modelNotReady: return "AI モデルを準備中です。しばらくしてから再度お試しください"
        case .osTooOld: return "iOS 26 以降が必要です"
        case .other: return "Apple Intelligence を利用できません"
        }
    }

    /// 現在の端末の状態。
    static var current: AIAvailability {
        #if canImport(FoundationModels)
        if #available(iOS 26.0, macOS 26.0, *) {
            return AIAvailability(SystemLanguageModel.default.availability)
        }
        #endif
        return .unavailable(.osTooOld)
    }
}

#if canImport(FoundationModels)
@available(iOS 26.0, macOS 26.0, *)
extension AIAvailability {
    init(_ a: SystemLanguageModel.Availability) {
        switch a {
        case .available: self = .available
        case .unavailable(.deviceNotEligible): self = .unavailable(.deviceNotEligible)
        case .unavailable(.appleIntelligenceNotEnabled): self = .unavailable(.appleIntelligenceNotEnabled)
        case .unavailable(.modelNotReady): self = .unavailable(.modelNotReady)
        case .unavailable: self = .unavailable(.other)
        }
    }
}
#endif

// MARK: - リクエスト組み立て(純粋関数・テスト対象)

/// AI に渡す 1 回分の指示。`AIRequestBuilder` が Component とフォームの値から作る。
struct AIRequest: Equatable, Sendable {
    enum Task: String, Sendable { case summarize, classify, rewrite, extract, suggest }

    var task: Task
    /// セッションの instructions(役割・出力形式)
    var instructions: String
    /// ユーザー側のプロンプト(入力本文を含む)
    var prompt: String
    /// classify の候補(それ以外は空)
    var options: [String]
}

enum AIRequestBuilder {
    /// オンデバイスモデルのコンテキスト(約 4k トークン)に収めるための入力上限(文字)。
    static let maxInputCharacters = 3000
    /// entity 入力の既定件数。
    static let defaultLimit = 20

    enum BuildError: LocalizedError, Equatable {
        case unknownTask
        case emptyInput
        case noOptions

        var errorDescription: String? {
            switch self {
            case .unknownTask: "この AI ボタンの設定が正しくありません"
            case .emptyInput: "先に入力してください"
            case .noOptions: "分類の候補がありません"
            }
        }
    }

    /// - Parameters:
    ///   - values: フォームのドラフト(field id → 値)
    ///   - records: `c.entity` 指定時のレコード(新しい順に並べ替えて limit 件使う)
    static func build(_ c: Component, entity: Entity, values: [String: JSONValue],
                      sourceEntity: Entity? = nil, records: [Record] = []) throws -> AIRequest {
        guard let task = c.task.flatMap(AIRequest.Task.init(rawValue:)) else { throw BuildError.unknownTask }

        var options: [String] = []
        if task == .classify {
            options = c.options ?? c.output.flatMap { entity.field($0)?.options } ?? []
            guard !options.isEmpty else { throw BuildError.noOptions }
        }

        let input = String(inputText(c, entity: entity, values: values, sourceEntity: sourceEntity, records: records)
            .prefix(maxInputCharacters))
        guard !input.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty else { throw BuildError.emptyInput }

        var prompt: String
        if let template = c.prompt, !template.isEmpty {
            prompt = expand(template, input: input, entity: entity, values: values)
            // プレースホルダが無いテンプレは「指示」とみなして入力を後ろに付ける
            if !template.contains("{{") { prompt += "\n\n" + input }
        } else {
            prompt = input
        }
        if task == .classify {
            prompt += "\n\n候補: " + options.joined(separator: " / ")
        }
        return AIRequest(task: task, instructions: instructions(for: task), prompt: prompt, options: options)
    }

    static func instructions(for task: AIRequest.Task) -> String {
        let common = "入力と同じ言語で、結果だけを返してください。前置き・引用符・説明は不要です。"
        switch task {
        case .summarize: return "あなたは要約アシスタントです。与えられた文章の要点を短くまとめてください。" + common
        case .classify: return "あなたは分類アシスタントです。与えられた文章に最も当てはまる候補を 1 つだけ選んでください。"
        case .rewrite: return "あなたは文章の清書アシスタントです。意味を変えずに、読みやすく自然な文章に書き直してください。" + common
        case .extract: return "あなたは情報抽出アシスタントです。与えられた文章から重要な点や値を短い箇条書きで抜き出してください。" + common
        case .suggest: return "あなたは前向きなアシスタントです。与えられた内容を踏まえて、短い一言や提案を 1 つだけ返してください。" + common
        }
    }

    /// AI に渡す本文: entity のレコード群 / input field の値 / フォーム全体(output 以外)。
    static func inputText(_ c: Component, entity: Entity, values: [String: JSONValue],
                          sourceEntity: Entity?, records: [Record]) -> String {
        if let source = sourceEntity {
            let recent = records.sorted { ($0.updatedAt ?? $0.createdAt ?? "") > ($1.updatedAt ?? $1.createdAt ?? "") }
                .prefix(Swift.max(c.limit ?? defaultLimit, 1))
            return recent.map { r in
                "- " + describe(source.storedFields.filter { $0.id != c.output }, value: { r.data[$0] ?? .null }, separator: " / ")
            }.joined(separator: "\n")
        }
        if let input = c.input {
            return FieldCoding.display(values[input] ?? .null, field: entity.field(input))
        }
        return describe(entity.storedFields.filter { $0.id != c.output }, value: { values[$0] ?? .null }, separator: "\n")
    }

    /// `{{input}}` → 本文、`{{fieldId}}` → フォームの値。
    static func expand(_ template: String, input: String, entity: Entity, values: [String: JSONValue]) -> String {
        var out = template.replacingOccurrences(of: "{{input}}", with: input)
        for f in entity.fields {
            out = out.replacingOccurrences(of: "{{\(f.id)}}", with: FieldCoding.display(values[f.id] ?? .null, field: f))
        }
        return out
    }

    /// 「名前: 値」を並べる(写真・位置・空は落とす)。
    private static func describe(_ fields: [Field], value: (String) -> JSONValue, separator: String) -> String {
        fields.compactMap { f -> String? in
            guard f.type != .photo, f.type != .video, f.type != .location else { return nil }
            let s = FieldCoding.display(value(f.id), field: f)
            return s.isEmpty ? nil : "\(f.name): \(s)"
        }.joined(separator: separator)
    }

    /// モデルの返答を output field に入れる形に整える。classify は候補に寄せる(候補外なら nil)。
    static func normalize(_ raw: String, for request: AIRequest) -> String? {
        let s = raw.trimmingCharacters(in: .whitespacesAndNewlines.union(CharacterSet(charactersIn: "\"「」")))
        guard request.task == .classify else { return s.isEmpty ? nil : s }
        if request.options.contains(s) { return s }
        return request.options.first { s.contains($0) }
    }
}

// MARK: - 実行

enum OnDeviceAIError: LocalizedError {
    case unavailable(AIAvailability)
    case invalidResponse
    case failed(String)

    var errorDescription: String? {
        switch self {
        case .unavailable(let a): a.message ?? AIAvailability.title
        case .invalidResponse: "AI の答えを読み取れませんでした。もう一度お試しください"
        case .failed(let detail): "AI の処理に失敗しました(\(detail))"
        }
    }
}

/// Foundation Models で 1 回実行する。
enum OnDeviceAI {
    static func run(_ request: AIRequest) async throws -> String {
        #if canImport(FoundationModels)
        if #available(iOS 26.0, macOS 26.0, *) {
            let availability = AIAvailability(SystemLanguageModel.default.availability)
            guard availability.isAvailable else { throw OnDeviceAIError.unavailable(availability) }
            let session = LanguageModelSession(instructions: request.instructions)
            do {
                let raw: String
                if request.task == .classify {
                    // 候補を列挙型の構造化出力で縛る(@Generable enum と同じ guided generation。候補は DSL 由来なので動的スキーマ)
                    let choice = DynamicGenerationSchema(name: "Choice", description: "最も当てはまる候補", anyOf: request.options)
                    let schema = try GenerationSchema(root: choice, dependencies: [])
                    let content = try await session.respond(to: request.prompt, schema: schema).content
                    raw = (try? content.value(String.self)) ?? content.jsonString
                } else {
                    raw = try await session.respond(to: request.prompt).content
                }
                guard let out = AIRequestBuilder.normalize(raw, for: request) else { throw OnDeviceAIError.invalidResponse }
                return out
            } catch let e as OnDeviceAIError {
                throw e
            } catch {
                throw OnDeviceAIError.failed(error.localizedDescription)
            }
        }
        #endif
        throw OnDeviceAIError.unavailable(.unavailable(.osTooOld))
    }
}
