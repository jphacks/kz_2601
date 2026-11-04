package appconfig

import (
	"net/http"
	"testing"

	"github.com/jphacks/kz_2601/backend/internal/store"
	"github.com/jphacks/kz_2601/backend/pkg/dsl"
)

// wantInstalled: uid opening appPath gets an app named name.
func wantInstalled(t *testing.T, e *env, uid, appPath, name string) {
	t.Helper()
	r := e.do(uid, http.MethodGet, appPath+"/dsl", nil, nil)
	d, _ := r.body["dsl"].(map[string]any)
	a, _ := d["app"].(map[string]any)
	if r.code != http.StatusOK || a["name"] != name {
		t.Fatalf("%s opens %s: want %q, got %d %s", uid, appPath, name, r.code, r.raw)
	}
}

// Opening an appId installs the DSL its creator published, never one another
// user published under the same appId.
func TestInstallUsesCreatorsPublishedDSL(t *testing.T) {
	ownerStores(t, func(t *testing.T, e *env, sfx string) {
		owner, other := "io"+sfx, "ix"+sfx
		app := "/v1/apps/INSTALL-DSL-1" + sfx
		e.ownApp(owner, app, kakeibo(t)) // private: nothing published yet
		e.editCopy(other, app, withScope(t, sharedKakeibo(t), `"name": "家計簿"`, `"name": "別物"`))

		wantInstalled(t, e, "iu1"+sfx, app, dsl.Initial().App.Name)

		e.editCopy(owner, app, sharedKakeibo(t))
		wantInstalled(t, e, "iu2"+sfx, app, "家計簿")
	})
}

// A mirror written by someone other than the creator is not installed.
func TestInstallIgnoresOthersMirror(t *testing.T) {
	storeEnvs(t, func(t *testing.T, e *env, st store.Store, sfx string) {
		owner, other := "jo"+sfx, "jx"+sfx
		id := "INSTALL-DSL-2" + sfx
		app := "/v1/apps/" + id
		e.ownApp(owner, app, kakeibo(t))
		if ok, err := st.PublishApp(t.Context(), other, id, walk(t), "散歩メモ", 2); err != nil || !ok {
			t.Fatalf("seed mirror: %v %v", ok, err)
		}
		wantInstalled(t, e, "ju1"+sfx, app, dsl.Initial().App.Name)
		// the creator's own copy is unaffected
		wantInstalled(t, e, owner, app, "家計簿")
	})
}
