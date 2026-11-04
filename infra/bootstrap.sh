#!/usr/bin/env bash
# 環境を 1 コマンドで init → plan →(確認して)apply する(#307)。
#
#   infra/bootstrap.sh stg            # plan だけ(何も変えない)
#   infra/bootstrap.sh stg apply      # plan を見せ、yes と打ったらその plan を apply
#   ALLOW_PROD=1 infra/bootstrap.sh prod [apply]
#
# 前提(人の作業)は infra/README.md の「stg を空のプロジェクトから作る」。
# plan に削除・作り直しが 1 つでもあれば apply しない。
set -euo pipefail

die() { echo "bootstrap: $*" >&2; exit 1; }

env="${1:-}"
action="${2:-plan}"

case "$env" in
  stg | prod) ;;
  *) die "使い方: $0 <stg|prod> [plan|apply]" ;;
esac
case "$action" in
  plan | apply) ;;
  *) die "2 つ目の引数は plan か apply" ;;
esac

cd "$(dirname "$0")"

tfvars="envs/${env}.tfvars"
backend="envs/${env}.backend.hcl"
planfile="${env}.tfplan"

command -v terraform >/dev/null || die "terraform が無い(>= 1.7)"
command -v gcloud >/dev/null || die "gcloud が無い"

[ -f "$tfvars" ] || die "$tfvars が無い: cp ${tfvars}.example $tfvars して値を埋める"
if grep -v '^[[:space:]]*#' "$tfvars" | grep -Eq 'XXXXXX-XXXXXX-XXXXXX|<digest>|<キー ID>'; then
  die "$tfvars に埋めていない値(XXXXXX / <...>)が残っている"
fi

if [ "$env" = prod ]; then
  [ "${ALLOW_PROD:-}" = 1 ] || die "prod は ALLOW_PROD=1 を付けて実行する(README の「既存の prod を取り込む」を先に読む)"
elif [ -f imports.tf ]; then
  die "imports.tf は prod の取り込み用。stg では消してから実行する"
fi

# 読み取りだけの事前チェック
gcloud auth application-default print-access-token >/dev/null 2>&1 ||
  die "ADC が無い: gcloud auth application-default login"

project_id="$(sed -n 's/^project_id *= *"\([^"]*\)".*/\1/p' "$tfvars")"
[ -n "$project_id" ] || die "$tfvars に project_id が無い"
billing="$(gcloud billing projects describe "$project_id" --format='value(billingEnabled)' 2>/dev/null || true)"
[ "$billing" = True ] || die "$project_id が無いか、課金がつながっていない(README の人の作業 1)"

bucket="$(sed -n 's/^bucket *= *"\([^"]*\)".*/\1/p' "$backend")"
gcloud storage buckets describe "gs://${bucket}" --format='value(name)' >/dev/null 2>&1 ||
  die "state のバケット gs://${bucket} が無いか読めない(README の「state の置き場とロック」)"

# 同じディレクトリで env を切り替えるので毎回 -reconfigure
terraform init -reconfigure -input=false -backend-config="$backend"

if [ "$env" = prod ] && [ ! -f imports.tf ] && [ -z "$(terraform state list)" ]; then
  die "prod の state が空。初回は imports.prod.tf.example を imports.tf にコピーして取り込む"
fi

terraform plan -input=false -var-file="$tfvars" -out="$planfile"

if [ "$action" = plan ]; then
  echo "bootstrap: plan だけ実行した($planfile)。適用は: $0 $env apply"
  exit 0
fi

if terraform show -no-color "$planfile" | grep -Eq 'will be destroyed|must be replaced'; then
  die "plan に削除・作り直しがある。apply しない(中身を確かめてコードか tfvars を直す)"
fi

read -r -p "この plan を ${project_id}(${env})に apply する? yes で実行: " answer
[ "$answer" = yes ] || die "中止した"

terraform apply -input=false "$planfile"
rm -f "$planfile"
terraform output
