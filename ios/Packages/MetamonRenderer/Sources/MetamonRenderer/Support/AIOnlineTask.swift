import Foundation
import MetamonDSL

/// aiButton の要約・分類・書き直し・抜き出し・提案のオンライン版。
/// 端末内 AI が使えない端末だけ、ホストの `POST /v1/text`(task 付き・Pro)に回す。材料と指示は端末内 AI と同じ所から取る。
extension AIRequestBuilder {
    /// `POST /v1/text` の依頼。材料は 4 KB(length long は 16 KB)に切る。`{{input}}` は目印に置き換え、材料は `input` で別に渡す。
    static func online(_ c: Component, entity: Entity, values: [String: JSONValue],
                       sourceEntity: Entity? = nil, records: [Record] = [], locale: String) throws -> AITextRequest {
        guard let task = c.task.flatMap(AIRequest.Task.init(rawValue:)) else { throw BuildError.unknownTask }
        var options: [String]?
        if task == .classify {
            let list = c.options ?? c.output.flatMap { entity.field($0)?.options } ?? []
            guard list.count >= 2 else { throw BuildError.noOptions }
            options = Array(list.prefix(20)) // backend の上限(TextMaxOptions)
        }
        let long = c.length == "long" && task != .classify
        let input = AITextLimits.clip(inputText(c, entity: entity, values: values, sourceEntity: sourceEntity, records: records),
                                      maxBytes: AITextLimits.maxInputBytes(long: long))
        let hasText = !input.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty
        guard hasText || c.inputPhoto != nil else { throw BuildError.emptyInput } // 写真だけでも材料になる

        var instruction = ""
        if let template = c.prompt, !template.isEmpty {
            instruction = expand(template, input: locale == "en" ? "the data below" : "下の材料", entity: entity, values: values)
        }
        instruction = String(instruction.trimmingCharacters(in: .whitespacesAndNewlines).prefix(AITextLimits.maxInstruction))
        return AITextRequest(instruction: instruction, shape: .line, input: hasText ? input : nil, locale: locale,
                             task: task.rawValue, options: options, length: long ? "long" : nil)
    }
}

/// オンライン版を 1 回実行して、答えを output のドラフトに入れる。同意・Pro の確認(画面を出す)はホストが持つ。
/// ホストが断った時(needsConsent / needsPro など)は投げ、ドラフトは変えない。
enum AIButtonOnline {
    @MainActor
    static func fill(_ c: Component, form: FormDraft, values: [String: JSONValue], sourceEntity: Entity?, records: [Record],
                     host: any AITextHost, locale: String) async throws {
        guard let output = c.output else { throw AIRequestBuilder.BuildError.unknownTask }
        var request = try AIRequestBuilder.online(c, entity: form.entity, values: values, sourceEntity: sourceEntity,
                                                  records: records, locale: locale)
        try await AIPhotoInput.attach(c, values: values, to: &request)
        let result = try await host.generateAIText(request)
        form.values[output] = .string(result.text)
    }
}
