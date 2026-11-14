// 審査用ビルド(APP_STORE_REVIEW・#245)ではバイナリから外す(HealthData.swift と同じ)。
#if !APP_STORE_REVIEW
import Foundation
#if os(iOS)
import HealthKit
#endif

/// ヘルスケアの気分(`HKStateOfMind`・iOS 18・#2234)。HealthKit に依存しない形で持ち、部品・DSL にはまだつながない(#1824 の決定待ち)。
/// 値はログ・クラッシュ報告に出さない(要配慮の情報)。
public struct HealthMood: Sendable, Equatable {
    public enum Kind: String, CaseIterable, Sendable {
        /// その時の気持ち
        case momentaryEmotion
        /// 1 日の気分
        case dailyMood
    }

    /// 使える最低の iOS(`HKStateOfMind` は iOS 18 から)
    public static let minimumOSMajorVersion = 18
    /// valence(とても不快 -1 〜 とても快 1)の範囲
    public static let valenceRange: ClosedRange<Double> = -1...1

    public var date: Date
    public var kind: Kind
    public var valence: Double
    public var labels: [HealthMoodLabel]

    public init(date: Date, kind: Kind, valence: Double, labels: [HealthMoodLabel] = []) {
        self.date = date
        self.kind = kind
        self.valence = valence
        self.labels = labels
    }

    /// この OS の版で使えるか(iOS 18 未満は使えない)
    public static func isSupported(osMajorVersion: Int) -> Bool { osMajorVersion >= minimumOSMajorVersion }

    /// valence が範囲内の有限の数か
    public static func isValidValence(_ v: Double) -> Bool { v.isFinite && valenceRange.contains(v) }

    /// 書く前の確かめ: valence が範囲外なら `HealthMoodError.valenceOutOfRange`。ラベルは重複を除き順を保つ
    public func validated() throws -> HealthMood {
        guard Self.isValidValence(valence) else { throw HealthMoodError.valenceOutOfRange }
        var copy = self
        copy.labels = HealthMoodLabel.unique(labels)
        return copy
    }
}

public enum HealthMoodError: LocalizedError, Equatable {
    /// valence が -1〜1 の外
    case valenceOutOfRange

    public var errorDescription: String? {
        switch self {
        case .valenceOutOfRange: L("気分の値は -1 から 1 の間で指定してください")
        }
    }
}

/// 気分のラベル(`HKStateOfMind.Label` と 1 対 1・文字は HealthKit の名前)。
public enum HealthMoodLabel: String, CaseIterable, Sendable {
    case amazed, amused, angry, annoyed, anxious, ashamed, brave, calm, confident, content
    case disappointed, discouraged, disgusted, drained, embarrassed, excited, frustrated, grateful, guilty, happy
    case hopeful, hopeless, indifferent, irritated, jealous, joyful, lonely, overwhelmed, passionate, peaceful
    case proud, relieved, sad, satisfied, scared, stressed, surprised, worried

    /// 文字の並びからラベルへ(知らない名前・重複は捨てる・大文字小文字と前後の空白は無視)
    public static func parse(_ names: [String]) -> [HealthMoodLabel] {
        unique(names.compactMap { HealthMoodLabel(rawValue: $0.trimmingCharacters(in: .whitespacesAndNewlines).lowercased()) })
    }

    static func unique(_ labels: [HealthMoodLabel]) -> [HealthMoodLabel] {
        var seen: Set<HealthMoodLabel> = []
        return labels.filter { seen.insert($0).inserted }
    }
}

/// `MockHealthData` の気分: 呼んだ日の 3 件(古い順)。書き込みは確かめるだけ。
public extension MockHealthData {
    static func moodSamples(now: Date = Date()) -> [HealthMood] {
        let today = Calendar.current.startOfDay(for: now)
        return [
            HealthMood(date: today.addingTimeInterval(8 * 3600), kind: .momentaryEmotion, valence: 0.4, labels: [.calm, .hopeful]),
            HealthMood(date: today.addingTimeInterval(13 * 3600), kind: .momentaryEmotion, valence: -0.3, labels: [.stressed]),
            HealthMood(date: today.addingTimeInterval(21 * 3600), kind: .dailyMood, valence: 0.6, labels: [.content, .grateful]),
        ]
    }

    func moods(from: Date, to: Date, kinds: Set<HealthMood.Kind>) async throws -> [HealthMood] {
        healthMoods(Self.moodSamples(), from: from, to: to, kinds: kinds)
    }

    func saveMood(_ mood: HealthMood) async throws {
        _ = try mood.validated()
    }
}

/// 期間(`from..<to`)と種類(空なら全部)で絞り、古い順に並べる
func healthMoods(_ moods: [HealthMood], from: Date, to: Date, kinds: Set<HealthMood.Kind>) -> [HealthMood] {
    moods.filter { $0.date >= from && $0.date < to && (kinds.isEmpty || kinds.contains($0.kind)) }
        .sorted { $0.date < $1.date }
}

/// 実機: HealthKit の `HKStateOfMind`。許可は読みと書きを別に求める。
public extension LiveHealthData {
    func moods(from: Date, to: Date, kinds: Set<HealthMood.Kind>) async throws -> [HealthMood] {
        #if os(iOS)
        guard #available(iOS 18, *) else { throw HealthDataError.unavailable }
        try await authorizeMood(write: false)
        let descriptor = HKSampleQueryDescriptor(
            predicates: [.stateOfMind(HKQuery.predicateForSamples(withStart: from, end: to))],
            sortDescriptors: [],
            limit: 500
        )
        let samples = try await descriptor.result(for: store)
        return healthMoods(samples.compactMap(HealthMood.init(stateOfMind:)), from: from, to: to, kinds: kinds)
        #else
        throw HealthDataError.unavailable
        #endif
    }

    func saveMood(_ mood: HealthMood) async throws {
        #if os(iOS)
        let mood = try mood.validated()
        guard #available(iOS 18, *) else { throw HealthDataError.unavailable }
        try await authorizeMood(write: true)
        guard store.authorizationStatus(for: HKObjectType.stateOfMindType()) == .sharingAuthorized else { throw HealthDataError.denied }
        try await store.save(mood.stateOfMind)
        #else
        throw HealthDataError.unavailable
        #endif
    }
}

#if os(iOS)
extension LiveHealthData {
    /// 読む時は読むだけ、書く時は書くだけの許可を求める(初回だけダイアログ)
    @available(iOS 18, *)
    fileprivate func authorizeMood(write: Bool) async throws {
        guard HKHealthStore.isHealthDataAvailable() else { throw HealthDataError.unavailable }
        let type: Set<HKSampleType> = [HKObjectType.stateOfMindType()]
        do {
            try await store.requestAuthorization(toShare: write ? type : [], read: write ? [] : type)
        } catch {
            throw HealthDataError.unavailable
        }
    }
}

@available(iOS 18, *)
extension HealthMood.Kind {
    init(_ kind: HKStateOfMind.Kind) {
        self = kind == .dailyMood ? .dailyMood : .momentaryEmotion
    }

    var healthKit: HKStateOfMind.Kind {
        switch self {
        case .momentaryEmotion: .momentaryEmotion
        case .dailyMood: .dailyMood
        }
    }
}

@available(iOS 18, *)
extension HealthMood {
    /// 知らない種類の記録は読まない
    init?(stateOfMind s: HKStateOfMind) {
        guard s.kind == .momentaryEmotion || s.kind == .dailyMood else { return nil }
        self.init(date: s.startDate, kind: Kind(s.kind), valence: s.valence, labels: s.labels.compactMap(HealthMoodLabel.init))
    }

    var stateOfMind: HKStateOfMind {
        HKStateOfMind(date: date, kind: kind.healthKit, valence: valence, labels: labels.map(\.healthKit), associations: [])
    }
}

@available(iOS 18, *)
extension HealthMoodLabel {
    /// HealthKit のラベルから(新しい OS で増えたラベルは nil)
    init?(_ label: HKStateOfMind.Label) {
        guard let match = Self.allCases.first(where: { $0.healthKit == label }) else { return nil }
        self = match
    }

    var healthKit: HKStateOfMind.Label {
        switch self {
        case .amazed: .amazed
        case .amused: .amused
        case .angry: .angry
        case .annoyed: .annoyed
        case .anxious: .anxious
        case .ashamed: .ashamed
        case .brave: .brave
        case .calm: .calm
        case .confident: .confident
        case .content: .content
        case .disappointed: .disappointed
        case .discouraged: .discouraged
        case .disgusted: .disgusted
        case .drained: .drained
        case .embarrassed: .embarrassed
        case .excited: .excited
        case .frustrated: .frustrated
        case .grateful: .grateful
        case .guilty: .guilty
        case .happy: .happy
        case .hopeful: .hopeful
        case .hopeless: .hopeless
        case .indifferent: .indifferent
        case .irritated: .irritated
        case .jealous: .jealous
        case .joyful: .joyful
        case .lonely: .lonely
        case .overwhelmed: .overwhelmed
        case .passionate: .passionate
        case .peaceful: .peaceful
        case .proud: .proud
        case .relieved: .relieved
        case .sad: .sad
        case .satisfied: .satisfied
        case .scared: .scared
        case .stressed: .stressed
        case .surprised: .surprised
        case .worried: .worried
        }
    }
}
#endif
#endif
