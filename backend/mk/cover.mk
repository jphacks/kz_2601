# backend/Makefile の末尾から -include される。レシピは backend/ を作業ディレクトリとして動く(make -C backend)。

# パッケージごとの網羅率を低い順の表で出す(#2361)。しきい値は無く、テストが落ちても表は出して終わる。
# エミュレータ試験の分も入れる時は README の「網羅率」を見る。COVER_PKGS で対象を絞れる。
COVER_PKGS ?= ./...
.PHONY: cover
cover:
	@out=$$(mktemp); \
	  go test -cover $(COVER_PKGS) > "$$out" 2>&1 || echo "note: some tests failed (lines below); coverage is still listed"; \
	  grep -E '^(FAIL|--- FAIL|panic:)' "$$out" || true; \
	  mod=$$(go list -m); \
	  echo "| 網羅率 | パッケージ |"; echo "|---:|---|"; \
	  awk -F'\t' -v mod="$$mod/" 'match($$0, /coverage: [0-9.]+%/) { \
	      pct = substr($$0, RSTART + 10, RLENGTH - 11); pkg = $$2; sub("^" mod, "", pkg); \
	      printf "%s\t%s\n", pct, pkg }' "$$out" | sort -n -k1,1 | awk -F'\t' '{ printf "| %s%% | %s |\n", $$1, $$2 }'; \
	  rm -f "$$out"
