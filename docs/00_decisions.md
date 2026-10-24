# 00 決定事項（フェーズ0・2026-10-24 確定）

> このファイルがプロジェクトの正典。変更は必ずここを更新してから。

## プロダクト
- iOS専用アプリ。初期画面は音声入力(ChatGPT音声画面風)。「こういうアプリが欲しい」と話すと、アプリ自体がそのアプリに変化する。
- 方式: Server-Driven UI。画面・遷移・コンポーネントは全てアプリに事前ビルド済み。バックエンドが「どの画面/コンポーネントをどう組むか」を **DSL JSON** として保持し、AIがそのJSONを書き換えることでアプリが動的に変わる。
- 初期状態のJSONは「AI会話画面のみ」。

## チーム
- 2人。iOS(Swift)は @ko-tarou がフル担当。@2410as がJSON周り(appconfig)のバックエンド担当。
- Claude Code(メイン＋サブエージェント)が設計・実装を並走。

## リポジトリ構成(モノレポ)
iOS アプリとバックエンドは **同じリポジトリ** に置く。ルート直下で領域ごとにフォルダを分け、CI は変更された領域のジョブだけ走る。

```
kz_2601/
├── ios/        # iOS アプリ本体(Swift / SwiftUI、xcodegen で project.yml から生成、.xcodeproj はコミットしない)
├── backend/    # Go 単一モジュール。cmd/appconfig・cmd/ai が各 Cloud Run サービス、pkg/dsl が共有型
├── docs/       # 00_decisions(正典)・01_goals・02_github_workflow・ios/・backend/・infra/
├── scripts/    # 運用スクリプト(github/ ブランチ保護など)
└── .github/    # workflows(backend-ci / ios-ci / secrets-scan / deploy-cloudrun)・PR/Issue テンプレ・CODEOWNERS
```
- 担当: `ios/` = @ko-tarou、`backend/cmd/appconfig/` = @2410as、それ以外は共同。
- iOS と backend の契約は `docs/dsl/dsl.schema.json`(DSL v1・決定表は `docs/dsl/README.md`)と `docs/backend/openapi-*.yaml`(API)。両側はここから導出し、二重定義しない。

## 技術スタック
| 領域 | 決定 | 備考 |
|---|---|---|
| iOS | Swift / SwiftUI、iOS 17+、xcodegen でプロジェクト生成 | 実機 iPhone 13 で検証。カメラ等はシミュレータ不可 |
| STT | Whisper → Claude の2段 | Whisperは **WhisperKit(オンデバイス)** を第一候補、不足なら OpenAI Whisper API |
| 生成AI | Claude API (claude-sonnet-5-5 を既定、品質不足なら claude-opus-5-5) | 構造化出力でDSL JSONを生成し、サーバー側でスキーマ検証。チューニングは後回し |
| バックエンド | Go 1.24、単一モジュール `backend/` に複数 `cmd/` | 1 cmd = 1 Cloud Run サービス。DSL型は `backend/pkg/dsl` で共有 |
| DB | **Firestore (Native mode)** | DSL JSONはドキュメントそのもの。スキーマ移行不要・無料枠・Cloud SQLの接続設定/VPC/課金を回避。RDBが要る根拠が出たら再検討 |
| 認証 | **Firebase Auth (Sign in with Apple)** | ログインサーバーを自作しない。GoはIDトークン検証のみ |
| インフラ | GCP (Cloud Run / Firestore / Secret Manager)、Terraform なし | gcloud コマンドの手順書で構築 |
| CI | GitHub Actions | Go: lint+test で **5分以内必須**。iOS: build(+unit test)、時間制限なし(超えてもローカル並行実行で代替可)。Draft PRでは発火しない |

## サーバー分割（当面）
| サービス | cmd | 担当 | 役割 |
|---|---|---|---|
| appconfig | `backend/cmd/appconfig` | @2410as | アプリDSL JSONのCRUD(エンティティ毎エンドポイント)、バージョン管理 |
| ai | `backend/cmd/ai` | Claude Code | 音声(or文字)→Claude→DSL JSONパッチ生成、スキーマ検証して appconfig に反映 |
- 当初2サービス。増やすのは「担当が分かれる」か「スケール特性が違う」時のみ。

## Git運用
- ブランチ: `main` / `develop` / `feature/*` / `hotfix/*` / `release/*`。エピックブランチ無し。
- `feature/*` → `develop`: PR必須、0 approve でマージ可(マージは人間のみ・botは不可)。
- `develop` → `main`: 実機テスト完了時のみ。
- CI: PRがOpen(非Draft)になった時点で発火。backend CIは5分以内、iOS CIは制限なし。
- PR/Issueテンプレートは `.github/` に用意。
- コミットは Conventional Commits (`feat:`/`fix:`/`docs:`/`chore:`)。
- **publicリポジトリ**。APIキー・GoogleService-Info.plist 等の秘密は絶対にコミットしない(`.gitignore` と secret scanning で二重防御)。

## 2日間の時間割（目安）
- Day1 午前: 設計並列(iOS/backend/要件/GCP手順/GitHub整備) → 昼: DSLスキーマv0をすり合わせ確定
- Day1 午後: iOSレンダラー骨格・音声画面、backend appconfig/ai 骨格、GCP構築、E2E 1本通す
- Day2 午前: デモ3本が通るまで詰める。Day2 午後: 動画・README・磨き

## 判断ログ(ユーザー確定)
| 日時 | 論点 | 決定 |
|---|---|---|
| 10/24 午前 | 生成アプリのユーザーデータ(明細・記録)の置き場 | **バックエンド(Firestore)**。DSL JSONもユーザーデータもユーザーに紐付けて保持。引き継ぎのためログイン機能あり |
| 10/24 午前 | 2回目以降の発話での改変方式 | **JSON Patch(RFC 6902)差分**。aiサービスはPatchを生成し、appconfigが適用・検証・バージョン保存 |
| 10/24 午前 | P0ネイティブ機能 | **カメラ と ローカル通知の両方**。他(位置/センサー/他アプリ連携)はP1以下 |
| 10/24 午前 | デモ3本 | 家計簿 / トレーニング記録 / 散歩メモ |
| 10/24 午前 | Sign in with Apple | 簡単ならやる。詰まったら固定デバイスID(匿名認証)で代替してよい |
| 10/24 午前 | STT | WhisperKit(オンデバイス)で確定。不足時のみ OpenAI Whisper API |
| 10/24 午前 | CI時間 | backend 5分以内必須。iOS は制限なし |
| 10/24 午前 | 秘密管理 | publicリポ。APIキー・plist は .gitignore + gitleaks。ローカルは `~/Desktop/JPHACKS2026/.env`(リポ外) |
| 10/24 昼 | DSL v1 統一(backend 案 × iOS 案) | **構造は backend 案(非再帰2段・閉じた enum・`additionalProperties:false`)、命名は iOS 案(camelCase・props フラット)**。`screens` は配列、`appId/version` は API 封筒、`voiceInput` は Leaf、`datePicker` 追加、式 `{{}}`/state は v1 に無し、写真は data URL(≤800px)、集計は iOS クライアント。正典 `docs/dsl/`(決定表 22 論点は `docs/dsl/README.md`) |
| 10/24 昼 | 画面レイアウト・タブ | **グリッド座標配置**: 各 Screen = 横 A〜F の 6 列 × 縦 1 行=可視高さ÷12(13 行目以降はスクロール)。Screen 直下の部品に省略可能な `area:"A1:C3"`、兄弟の重なりは backend 検証でエラー(id と area を人間可読で列挙)、可変長はセル内スクロール、`area` 無しは最終使用行の次から積む。Container 内は積むだけ(v1)。**タブ(`app.tabs[]`)は v1 に残す**(「タブ無し」却下)。仕様は `docs/dsl/README.md` §3.4 |
| 10/24 昼 | iOSアプリ名 | **メタモン**(表示名)。内部識別子は `Metamon`(ターゲット/スキーム/Bundle ID `jp.kz2601.metamon`)。旧仮名 Morph は廃止。リネームは #5 の実機確認完了後に単独PRで |
| 10/24 夜 | 正式名(#62・#69) | **Tailor It**(表示名・UI 文言・README・台本・ピッチ)。由来は「あなたに合わせて仕立てる(tailor)」。仮名「メタモン」は商標リスク(Q6)もあり廃止。内部識別子(ターゲット `Metamon`・Bundle ID `jp.kz2601.metamon(.widget)`・App Group `group.jp.kz2601.metamon`・`Metamon{DSL,Renderer,Services}`・`METAMON_*`・Firebase 登録・Firestore パス)は変更しない。「変身」「変身一覧」の比喩は残す |
| 10/24 昼 | PRテンプレ | 「重点レビューファイル」節を追加。レビュアーが読む順と見どころを最大5ファイルで明示 |
