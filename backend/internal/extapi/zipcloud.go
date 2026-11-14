package extapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

// zipcloud: 郵便番号から住所を引く(https://zipcloud.ibsnet.co.jp/doc/api)。
// 無料・鍵なし・OAuth なし: 1 回 = `runs` 1。届く先は zipcloud.ibsnet.co.jp だけ。
//
//	address(zipcode)   行: 住所ごと(見出し = 住所・補足 = カナ・値 = 〒100-0001)   EXT_ZIPCLOUD_ENABLED=1
//
// zipcode は 7 桁の数字(「100-0001」の形も可)。同じ番号に町域が複数ある時は
// 行が複数(Item は最初の行)。無い番号は 400 bad_request。データは月ごとの
// 更新なので答えは 1 日持つ(規約 5.1 の「最新の情報」の範囲)。
//
// 規約(利用規約 https://zipcloud.ibsnet.co.jp/rule/api・2026-11-14 に読んだ):
// データの権利は運営(株式会社アイビス)・出典の表示の決まりは無し・回数の上限は
// 運営の判断(4.2)・求めがあれば表示をやめる(4.3)・予告なく止まる(3)。
// 「本 API の機能を提供することを主目的とするサイト」での利用の禁止(2.2)が
// 生成アプリに当たるかは人の判断(external-api-terms.md)。
const Zipcloud = "zipcloud"

var zipcloudHosts = []string{"https://zipcloud.ibsnet.co.jp"}

const (
	zipcloudPath    = "/api/search"
	zipcloudMaxBody = 64 << 10
	zipcloudMaxRows = 20
)

var zipcodeRe = regexp.MustCompile(`^[0-9]{3}-?[0-9]{4}$`)

func init() {
	register(Provider{ID: Zipcloud, Billing: BillingFree, Hosts: zipcloudHosts,
		Ops: []OpSpec{
			{Name: "address", Args: []string{"zipcode"}, Check: checkZipcode, CacheTTL: 24 * time.Hour},
		},
		New: func(Secrets) (map[string]Operation, error) {
			return newZipcloud(zipcloudHosts[0], NewHTTPClient(zipcloudHosts, DefaultTimeout)), nil
		}})
}

// checkZipcode: zipcode は 7 桁(3-4 のハイフンは可)。
func checkZipcode(a map[string]string) error {
	if _, err := zipcode(a["zipcode"]); err != nil {
		return err
	}
	return nil
}

// zipcode は 7 桁の数字に直した郵便番号。
func zipcode(s string) (string, error) {
	s = strings.TrimSpace(s)
	if !zipcodeRe.MatchString(s) {
		return "", errors.New("zipcode must be 7 digits like 1000001")
	}
	return strings.ReplaceAll(s, "-", ""), nil
}

// newZipcloud は base に向けた op(テストは httptest のサーバーを渡す)。
func newZipcloud(base string, hc *http.Client) map[string]Operation {
	z := &zipcloud{base: strings.TrimRight(base, "/"), hc: hc}
	return map[string]Operation{"address": OperationFunc(z.address)}
}

type zipcloud struct {
	base string
	hc   *http.Client
}

// zipcloudAnswer は API の JSON(行になる所だけ)。status は数字。
type zipcloudAnswer struct {
	Status  int    `json:"status"`
	Message string `json:"message"`
	Results []struct {
		Zipcode  string `json:"zipcode"`
		Address1 string `json:"address1"`
		Address2 string `json:"address2"`
		Address3 string `json:"address3"`
		Kana1    string `json:"kana1"`
		Kana2    string `json:"kana2"`
		Kana3    string `json:"kana3"`
	} `json:"results"`
}

func (z *zipcloud) address(ctx context.Context, in Input) (Output, error) {
	zip, err := zipcode(in.Args["zipcode"])
	if err != nil {
		return Output{}, fmt.Errorf("%w: %v", ErrBadArgs, err)
	}
	q := url.Values{"zipcode": {zip}, "limit": {"20"}}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, z.base+zipcloudPath+"?"+q.Encode(), nil)
	if err != nil {
		return Output{}, errors.New("zipcloud: building the request failed")
	}
	req.Header.Set("Accept", "application/json")
	resp, err := z.hc.Do(req)
	if err != nil {
		return Output{}, transportError(ctx, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return Output{}, statusError(resp.StatusCode)
	}
	body, err := ReadBody(io.LimitReader(resp.Body, zipcloudMaxBody+1))
	if err == nil && len(body) > zipcloudMaxBody {
		err = fmt.Errorf("body over %d bytes", zipcloudMaxBody)
	}
	if err != nil {
		if ctx.Err() != nil {
			return Output{}, ctx.Err()
		}
		return Output{}, &ProviderError{Status: resp.StatusCode, Err: err}
	}
	var a zipcloudAnswer
	if err := json.Unmarshal(body, &a); err != nil {
		return Output{}, &ProviderError{Status: resp.StatusCode, Err: errors.New("zipcloud answered an unexpected shape")}
	}
	switch {
	case a.Status == http.StatusBadRequest:
		// 入力の誤り(ここまで来るのは API の決まりが変わった時)
		return Output{}, fmt.Errorf("%w: zipcloud refused the zipcode", ErrUnknown)
	case a.Status != http.StatusOK:
		// 500 は API の中の失敗(本文のメッセージは出さない)
		return Output{}, &ProviderError{Status: a.Status, Retryable: a.Status >= 500, Err: errors.New("zipcloud failed")}
	}
	rows := []Row{}
	for _, r := range a.Results {
		addr := strings.TrimSpace(r.Address1 + r.Address2 + r.Address3)
		if addr == "" {
			continue
		}
		code := strings.TrimSpace(r.Zipcode)
		if len(code) != 7 || strings.TrimLeft(code, "0123456789") != "" {
			code = zip
		}
		rows = append(rows, Row{Title: clip(addr, 100), Subtitle: clip(joinSpaces(r.Kana1, r.Kana2, r.Kana3), 100), Value: "〒" + code[:3] + "-" + code[3:]})
		if len(rows) == zipcloudMaxRows {
			break
		}
	}
	if len(rows) == 0 {
		return Output{}, fmt.Errorf("%w: zipcloud has no address for %s", ErrUnknown, zip)
	}
	item := rows[0]
	return Output{Item: &item, Rows: rows}, nil
}

// joinSpaces は空でないものを空白でつなぐ。
func joinSpaces(parts ...string) string {
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return strings.Join(out, " ")
}
