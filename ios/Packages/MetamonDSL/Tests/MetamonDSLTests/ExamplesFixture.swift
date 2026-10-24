import Foundation
import XCTest
@testable import MetamonDSL

/// 正典サンプル `docs/dsl/examples/*.json` をリポジトリのパスから直接読む(`swift test` / `xcodebuild test` どちらでも同じ物を見る)。
enum ExamplesFixture {
    /// このファイル → MetamonDSLTests → Tests → MetamonDSL → Packages → ios → リポジトリ root
    static let repoRoot: URL = {
        var url = URL(fileURLWithPath: #filePath)
        for _ in 0..<6 { url.deleteLastPathComponent() }
        return url
    }()

    static let examplesDir = repoRoot.appending(path: "docs/dsl/examples")
    static let schemaURL = repoRoot.appending(path: "docs/dsl/dsl.schema.json")

    static func load(_ name: String) throws -> AppDSL {
        try AppDSL.load(contentsOf: examplesDir.appending(path: "\(name).json"))
    }
}
