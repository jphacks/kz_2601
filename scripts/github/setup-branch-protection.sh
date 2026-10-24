#!/usr/bin/env bash
# =============================================================================
# main / develop のブランチ保護を設定する(冪等。何度実行しても同じ最終状態になる)。
#
# 使い方:
#   scripts/github/setup-branch-protection.sh            # 適用
#   scripts/github/setup-branch-protection.sh --dry-run  # 送信する JSON を表示するだけ(API 書き込みなし)
#
# 環境変数(任意):
#   REPO=jphacks/kz_2601          対象リポ(省略時は gh repo view から取得)
#   HUMAN_USERS="ko-tarou foo"    マージ(=push)を許可する人間の GitHub ID(空白区切り)
#   STRICT_STATUS_CHECKS=false    true にすると「マージ前に base と up-to-date 必須」(2人開発では遅くなるので既定 false)
#
# 前提: gh CLI でログイン済み(`gh auth status`)、対象リポの admin 権限。
#
# --- なぜ classic branch protection を使うか(rulesets ではなく) ---
# - public リポなら free plan でも classic / rulesets の両方が使える。
# - 要件「マージは人間のみ・bot(GitHub Apps)不可」を最も素直に表現できるのが classic の `restrictions`
#   (= このブランチに push できる主体の allowlist。PR マージも push 扱いなので対象になる)。
#   `users` に人間だけを列挙し `apps: []` にすれば、GitHub Apps(dependabot の auto-merge・各種 bot)は
#   ブランチ保護を満たしていてもマージできない。
# - rulesets は「全員禁止 + bypass actors で例外」というモデルで、人間を bypass に入れると
#   他のルール(PR 必須など)まで素通りする設計のため、今回の要件と相性が悪い。
# - 注意: `restrictions` は **organization 所有リポのみ** 対応。user 所有リポの場合は自動で null にし、
#   警告を出す(その場合は bot に write 権限を与えないことで代替)。
# - 注意: `enforce_admins: true` なので admin も PR 経由でしか入れられない。緊急時は一時的に
#   `gh api -X DELETE repos/$REPO/branches/main/protection/enforce_admins` で外す。
# =============================================================================
set -euo pipefail

DRY_RUN=false
for arg in "$@"; do
  case "$arg" in
    --dry-run) DRY_RUN=true ;;
    -h|--help) sed -n '2,30p' "$0"; exit 0 ;;
    *) echo "unknown arg: $arg" >&2; exit 2 ;;
  esac
done

command -v gh >/dev/null || { echo "gh CLI が必要です: https://cli.github.com" >&2; exit 1; }
command -v jq >/dev/null || { echo "jq が必要です" >&2; exit 1; }

REPO="${REPO:-$(gh repo view --json nameWithOwner --jq .nameWithOwner)}"
HUMAN_USERS="${HUMAN_USERS:-ko-tarou 2410as}"   # チームの人間2名。bot/GitHub Apps は含めない
STRICT_STATUS_CHECKS="${STRICT_STATUS_CHECKS:-false}"

# required status checks。**各ワークフローの job 名と一致させること**(name: ではなくジョブの name/id)。
#   .github/workflows/backend-ci.yml   -> backend-ci
#   .github/workflows/ios-ci.yml       -> ios-ci
#   .github/workflows/secrets-scan.yml -> secrets-scan
# app_id 15368 = GitHub Actions。他の App が同名の check を投げても required を満たせないようにする。
REQUIRED_CHECKS=(backend-ci ios-ci secrets-scan)
GITHUB_ACTIONS_APP_ID=15368

echo "repo: $REPO"
echo "human users (allowed to merge/push): $HUMAN_USERS"

OWNER_TYPE="$(gh api "repos/$REPO" --jq .owner.type)"
echo "owner type: $OWNER_TYPE"

# ---- payload 組み立て --------------------------------------------------------
checks_json="$(printf '%s\n' "${REQUIRED_CHECKS[@]}" \
  | jq -R --argjson app "$GITHUB_ACTIONS_APP_ID" '{context: ., app_id: $app}' | jq -s .)"

if [ "$OWNER_TYPE" = "Organization" ]; then
  users_json="$(printf '%s\n' $HUMAN_USERS | jq -R . | jq -s .)"
  restrictions_json="$(jq -n --argjson users "$users_json" '{users: $users, teams: [], apps: []}')"
else
  echo "WARN: user 所有リポのため restrictions(人間限定)は設定できません。bot に write 権限を与えないでください。" >&2
  restrictions_json=null
fi

build_payload() {
  jq -n \
    --argjson strict "$STRICT_STATUS_CHECKS" \
    --argjson checks "$checks_json" \
    --argjson restrictions "$restrictions_json" \
    '{
      required_status_checks: { strict: $strict, checks: $checks },
      enforce_admins: true,
      required_pull_request_reviews: {
        required_approving_review_count: 0,
        dismiss_stale_reviews: false,
        require_code_owner_reviews: false,
        require_last_push_approval: false
      },
      restrictions: $restrictions,
      required_linear_history: false,
      allow_force_pushes: false,
      allow_deletions: false,
      block_creations: false,
      required_conversation_resolution: true,
      lock_branch: false,
      allow_fork_syncing: false
    }'
}

# ---- develop ブランチをリモートに作る(無ければ main から。既に push 済みなら何もしない) ----
ensure_branch() {
  local branch="$1"
  if gh api "repos/$REPO/branches/$branch" >/dev/null 2>&1; then
    echo "branch exists: $branch"
    return
  fi
  local sha
  sha="$(gh api "repos/$REPO/git/ref/heads/main" --jq .object.sha)"
  echo "create branch: $branch from main@$sha"
  if [ "$DRY_RUN" = true ]; then return; fi
  gh api -X POST "repos/$REPO/git/refs" -f ref="refs/heads/$branch" -f sha="$sha" >/dev/null
  # 同等の git コマンド: git push origin main:develop
}

# ---- 保護を適用(PUT は全置換なので冪等) ---------------------------------------
protect() {
  local branch="$1"
  local payload
  payload="$(build_payload)"
  echo "---- protection for $branch ----"
  if [ "$DRY_RUN" = true ]; then
    echo "$payload" | jq .
    return
  fi
  echo "$payload" | gh api -X PUT "repos/$REPO/branches/$branch/protection" \
    -H "Accept: application/vnd.github+json" --input - \
    --jq '{
      checks: [.required_status_checks.checks[].context],
      approvals: .required_pull_request_reviews.required_approving_review_count,
      enforce_admins: .enforce_admins.enabled,
      conversation_resolution: .required_conversation_resolution.enabled,
      force_push: .allow_force_pushes.enabled,
      deletions: .allow_deletions.enabled,
      restricted_to_users: [.restrictions.users[]?.login]
    }'
}

ensure_branch develop
protect main
protect develop

cat <<EOS

done. 確認:
  gh api repos/$REPO/branches/main/protection | jq .
  gh api repos/$REPO/branches/develop/protection | jq .

補足:
  - PR の base を develop にするのを忘れないこと(既定ブランチは main のまま。変更したい場合:
      gh repo edit $REPO --default-branch develop )
  - 緊急で admin が直 push したい時(非推奨): 一時的に enforce_admins を外す
      gh api -X DELETE repos/$REPO/branches/main/protection/enforce_admins
EOS
