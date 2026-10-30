# 1イメージに appconfig / ai と、退会済みアカウントを消す purge(Cloud Run Job 用・docs/infra/account-purge.md)を同梱する。
# Cloud Run 側は `--command=/app/ai` でサービスごとに起動バイナリを切り替える(ビルドは1回で済む)。
FROM golang:1.26-bookworm AS build
WORKDIR /src
COPY go.mod go.sum* ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/appconfig ./cmd/appconfig \
 && CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/ai ./cmd/ai \
 && CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/purge ./cmd/purge

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/ /app/
ENV PORT=8080
EXPOSE 8080
# 既定は appconfig。ai は Cloud Run の --command=/app/ai (ローカルは docker run --entrypoint /app/ai) で上書きする。
ENTRYPOINT ["/app/appconfig"]
