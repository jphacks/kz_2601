# ios (Swift / SwiftUI) — メタモン

xcodegen で `project.yml` から Xcode プロジェクトを生成する(`.xcodeproj` はコミットしない)。

```sh
cd ios && METAMON_TEAM_ID=CHP337D75C xcodegen generate && open Metamon.xcodeproj
```

## 構成(マルチモジュール)

```
ios/
├── Metamon/                 # アプリ本体(薄い): MetamonApp / AppRuntime / RootView / VoiceInputView / BuildConfig(DI の入口)
└── Packages/
    ├── MetamonDSL/          # DSL モデル・GridArea・Record/Aggregator(Foundation のみ・swift test 可)
    ├── MetamonRenderer/     # SwiftUI View。Components/ は 1 種 = 1 ファイル、ComponentRegistry で登録
    └── MetamonServices/     # Protocols/ + Live/ Mock/ Native/ 実装。WhisperKit 依存はここ
```

依存の向き: `App → Renderer → DSL`、`App → Services → DSL`。Renderer は Services を知らない(詳細: `docs/ios/architecture.md` §1)。

## テスト

```sh
xcodebuild -scheme Metamon -destination 'platform=iOS Simulator,name=iPhone 17' test CODE_SIGNING_ALLOWED=NO  # 3 パッケージ一括
(cd Packages/MetamonDSL && swift test)   # DSL のみ、macOS で数秒
```

## コンポーネントを足す(3 行)
1. `Packages/MetamonRenderer/Sources/MetamonRenderer/Components/Leaves/FooLeaf.swift` に `struct FooLeaf: View`
2. `Packages/MetamonDSL/.../Model/Component.swift` の `ComponentType` に `case foo`(`docs/dsl/dsl.schema.json` にも)
3. `ComponentRegistry.standard` に `r.register(.foo) { c, ctx in FooLeaf(c: c, ctx: ctx) }`

`MetamonRendererTests` が schema と登録の一致を検証するので、抜けると赤になる。

## 名称
- 表示名「メタモン」は `Metamon/Resources/Info.plist` の `CFBundleDisplayName` の 1 箇所(提出前に正式名を決める。商標要確認)。
- 内部識別子 `Metamon` / Bundle ID `jp.kz2601.metamon` は Firebase 登録値なので変更しない。
