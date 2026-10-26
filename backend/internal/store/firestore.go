package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"cloud.google.com/go/firestore"
	"google.golang.org/api/iterator"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Firestore is the production Store.
//
// Layout (architecture.md §3):
//
//	users/{uid}/apps/{appId}                       {name,currentVersion,status,createdAt,updatedAt,dsl(map),dslJson}
//	users/{uid}/apps/{appId}/versions/{n:08d}      {version,parentVersion,dslJson,patchJson,source,createdAt}
//	users/{uid}/apps/{appId}/entities/{e}/records/{id} {entity,data,clientId,createdAt,updatedAt}
//	apps_public/{appId}                            {ownerUid,name,version,dsl(map),dslJson,createdAt,updatedAt}
//	apps_public/{appId}/entities/{e}/records/{id}  {entity,data,clientId,author,authorName,markCounts,createdAt,updatedAt}
//	  .../records/{id}/marks/{markId}/by/{uid}     {at}  (#255: firestore_marks.go)
//
// `dsl` is stored twice: as a native map (so an iOS realtime listener can
// read it) and as the canonical JSON string (what the API serves back).
type Firestore struct {
	c *firestore.Client
}

const publicCollection = "apps_public"

// NewFirestore connects to projectID (honours FIRESTORE_EMULATOR_HOST).
func NewFirestore(ctx context.Context, projectID string) (*Firestore, error) {
	c, err := firestore.NewClient(ctx, projectID)
	if err != nil {
		return nil, err
	}
	return &Firestore{c: c}, nil
}

// Close releases the client.
func (f *Firestore) Close() error { return f.c.Close() }

var _ Store = (*Firestore)(nil)

func (f *Firestore) appRef(uid, appID string) *firestore.DocumentRef {
	return f.c.Collection("users").Doc(uid).Collection("apps").Doc(appID)
}

func (f *Firestore) publicRef(appID string) *firestore.DocumentRef {
	return f.c.Collection(publicCollection).Doc(appID)
}

// recordsCol is the collection a RecordSet maps to.
func (f *Firestore) recordsCol(rs RecordSet) *firestore.CollectionRef {
	var app *firestore.DocumentRef
	if rs.Public {
		app = f.publicRef(rs.AppID)
	} else {
		app = f.appRef(rs.UID, rs.AppID)
	}
	return app.Collection("entities").Doc(rs.Entity).Collection("records")
}

func (f *Firestore) recRef(rs RecordSet, id string) *firestore.DocumentRef {
	return f.recordsCol(rs).Doc(id)
}

func isNotFound(err error) bool { return status.Code(err) == codes.NotFound }

// ---- document <-> struct ----------------------------------------------------

type appDoc struct {
	Name           string         `firestore:"name"`
	CurrentVersion int64          `firestore:"currentVersion"`
	Status         string         `firestore:"status"`
	CreatedAt      time.Time      `firestore:"createdAt"`
	UpdatedAt      time.Time      `firestore:"updatedAt"`
	DSL            map[string]any `firestore:"dsl"`
	DSLJSON        string         `firestore:"dslJson"`
}

func (d *appDoc) toApp(id string) *App {
	return &App{ID: id, Name: d.Name, CurrentVersion: int(d.CurrentVersion), Status: d.Status,
		CreatedAt: d.CreatedAt, UpdatedAt: d.UpdatedAt, DSL: json.RawMessage(d.DSLJSON)}
}

type versionDoc struct {
	Version       int64          `firestore:"version"`
	ParentVersion int64          `firestore:"parentVersion"`
	DSLJSON       string         `firestore:"dslJson"`
	PatchJSON     string         `firestore:"patchJson,omitempty"`
	Source        map[string]any `firestore:"source,omitempty"`
	CreatedAt     time.Time      `firestore:"createdAt"`
}

func (d *versionDoc) toVersion(appID string) (*Version, error) {
	v := &Version{AppID: appID, Version: int(d.Version), ParentVersion: int(d.ParentVersion), DSL: json.RawMessage(d.DSLJSON), CreatedAt: d.CreatedAt}
	if d.PatchJSON != "" {
		if err := json.Unmarshal([]byte(d.PatchJSON), &v.Patch); err != nil {
			return nil, fmt.Errorf("decode patch: %w", err)
		}
	}
	if len(d.Source) > 0 {
		b, err := json.Marshal(d.Source)
		if err != nil {
			return nil, err
		}
		v.Source = &Source{}
		if err := json.Unmarshal(b, v.Source); err != nil {
			return nil, fmt.Errorf("decode source: %w", err)
		}
	}
	return v, nil
}

type recordDoc struct {
	Entity     string           `firestore:"entity"`
	Data       map[string]any   `firestore:"data"`
	ClientID   string           `firestore:"clientId,omitempty"`
	Author     string           `firestore:"author,omitempty"`
	AuthorName string           `firestore:"authorName,omitempty"`
	MarkCounts map[string]int64 `firestore:"markCounts,omitempty"` // #255
	CreatedAt  time.Time        `firestore:"createdAt"`
	UpdatedAt  time.Time        `firestore:"updatedAt"`
}

func (d *recordDoc) toRecord(id string) *Record {
	if d.Data == nil {
		d.Data = map[string]any{}
	}
	rec := &Record{ID: id, Entity: d.Entity, Data: d.Data, ClientID: d.ClientID, Author: d.Author, AuthorName: d.AuthorName, CreatedAt: d.CreatedAt, UpdatedAt: d.UpdatedAt}
	for k, v := range d.MarkCounts {
		if rec.MarkCounts == nil {
			rec.MarkCounts = map[string]int{}
		}
		rec.MarkCounts[k] = int(v)
	}
	return rec
}

type publicAppDoc struct {
	OwnerUID  string         `firestore:"ownerUid"`
	Name      string         `firestore:"name"`
	Version   int64          `firestore:"version"`
	DSL       map[string]any `firestore:"dsl"`
	DSLJSON   string         `firestore:"dslJson"`
	CreatedAt time.Time      `firestore:"createdAt"`
	UpdatedAt time.Time      `firestore:"updatedAt"`
}

func (d *publicAppDoc) toPublicApp(id string) *PublicApp {
	return &PublicApp{ID: id, OwnerUID: d.OwnerUID, Name: d.Name, Version: int(d.Version), DSL: json.RawMessage(d.DSLJSON), CreatedAt: d.CreatedAt, UpdatedAt: d.UpdatedAt}
}

func rawToMap(raw json.RawMessage) (map[string]any, error) {
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, fmt.Errorf("dsl is not an object: %w", err)
	}
	return m, nil
}

func sourceToMap(s *Source) (map[string]any, error) {
	if s == nil {
		return nil, nil
	}
	b, err := json.Marshal(s)
	if err != nil {
		return nil, err
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, err
	}
	return m, nil
}

// ---- apps -------------------------------------------------------------------

// ListApps implements Store.
func (f *Firestore) ListApps(ctx context.Context, uid string, limit, offset int) ([]App, error) {
	q := f.c.Collection("users").Doc(uid).Collection("apps").OrderBy("updatedAt", firestore.Desc)
	if offset > 0 {
		q = q.Offset(offset)
	}
	if limit > 0 {
		q = q.Limit(limit)
	}
	it := q.Documents(ctx)
	defer it.Stop()
	out := []App{}
	for {
		snap, err := it.Next()
		if errors.Is(err, iterator.Done) {
			return out, nil
		}
		if err != nil {
			return nil, err
		}
		var d appDoc
		if err := snap.DataTo(&d); err != nil {
			return nil, err
		}
		a := d.toApp(snap.Ref.ID)
		a.DSL = nil // list = summaries
		out = append(out, *a)
	}
}

// GetApp implements Store.
func (f *Firestore) GetApp(ctx context.Context, uid, appID string) (*App, error) {
	snap, err := f.appRef(uid, appID).Get(ctx)
	if isNotFound(err) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	var d appDoc
	if err := snap.DataTo(&d); err != nil {
		return nil, err
	}
	return d.toApp(appID), nil
}

// EnsureApp implements Store.
func (f *Firestore) EnsureApp(ctx context.Context, uid, appID string, initial json.RawMessage, name string) (*App, bool, error) {
	ref := f.appRef(uid, appID)
	var out *App
	created := false
	err := f.c.RunTransaction(ctx, func(ctx context.Context, tx *firestore.Transaction) error {
		created = false
		snap, err := tx.Get(ref)
		if err == nil {
			var d appDoc
			if err := snap.DataTo(&d); err != nil {
				return err
			}
			out = d.toApp(appID)
			return nil
		}
		if !isNotFound(err) {
			return err
		}
		m, err := rawToMap(initial)
		if err != nil {
			return err
		}
		now := time.Now().UTC()
		d := appDoc{Name: name, CurrentVersion: 1, Status: "active", CreatedAt: now, UpdatedAt: now, DSL: m, DSLJSON: string(initial)}
		if err := tx.Create(ref, d); err != nil {
			return err
		}
		src, _ := sourceToMap(&Source{Kind: "system", Mode: "full"})
		vd := versionDoc{Version: 1, ParentVersion: 0, DSLJSON: string(initial), Source: src, CreatedAt: now}
		if err := tx.Create(ref.Collection("versions").Doc(VersionDocID(1)), vd); err != nil {
			return err
		}
		out = d.toApp(appID)
		created = true
		return nil
	})
	if err != nil {
		return nil, false, err
	}
	return out, created, nil
}

// UpdateAppMeta implements Store.
func (f *Firestore) UpdateAppMeta(ctx context.Context, uid, appID string, name, status *string) (*App, error) {
	ref := f.appRef(uid, appID)
	updates := []firestore.Update{{Path: "updatedAt", Value: time.Now().UTC()}}
	if name != nil {
		updates = append(updates, firestore.Update{Path: "name", Value: *name})
	}
	if status != nil {
		updates = append(updates, firestore.Update{Path: "status", Value: *status})
	}
	if _, err := ref.Update(ctx, updates); err != nil {
		if isNotFound(err) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return f.GetApp(ctx, uid, appID)
}

// CommitVersion implements Store.
func (f *Firestore) CommitVersion(ctx context.Context, uid, appID string, c Commit) (*Version, error) {
	ref := f.appRef(uid, appID)
	var out *Version
	err := f.c.RunTransaction(ctx, func(ctx context.Context, tx *firestore.Transaction) error {
		snap, err := tx.Get(ref)
		if isNotFound(err) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		var d appDoc
		if err := snap.DataTo(&d); err != nil {
			return err
		}
		cur := int(d.CurrentVersion)
		if c.BaseVersion != cur {
			return &ConflictError{Current: cur}
		}
		n := cur + 1
		m, err := rawToMap(c.DSL)
		if err != nil {
			return err
		}
		now := time.Now().UTC()
		upd := []firestore.Update{
			{Path: "currentVersion", Value: n},
			{Path: "dsl", Value: m},
			{Path: "dslJson", Value: string(c.DSL)},
			{Path: "updatedAt", Value: now},
		}
		if c.Name != "" {
			upd = append(upd, firestore.Update{Path: "name", Value: c.Name})
		}
		if err := tx.Update(ref, upd); err != nil {
			return err
		}
		vd := versionDoc{Version: int64(n), ParentVersion: int64(cur), DSLJSON: string(c.DSL), CreatedAt: now}
		if len(c.Patch) > 0 {
			pb, err := json.Marshal(c.Patch)
			if err != nil {
				return err
			}
			vd.PatchJSON = string(pb)
		}
		if vd.Source, err = sourceToMap(c.Source); err != nil {
			return err
		}
		if err := tx.Create(ref.Collection("versions").Doc(VersionDocID(n)), vd); err != nil {
			return err
		}
		out = &Version{AppID: appID, Version: n, ParentVersion: cur, DSL: c.DSL, Patch: c.Patch, Source: c.Source, CreatedAt: now}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// ListVersions implements Store.
func (f *Firestore) ListVersions(ctx context.Context, uid, appID string, limit, offset int) ([]Version, error) {
	if _, err := f.GetApp(ctx, uid, appID); err != nil {
		return nil, err
	}
	q := f.appRef(uid, appID).Collection("versions").OrderBy("version", firestore.Desc)
	if offset > 0 {
		q = q.Offset(offset)
	}
	if limit > 0 {
		q = q.Limit(limit)
	}
	it := q.Documents(ctx)
	defer it.Stop()
	out := []Version{}
	for {
		snap, err := it.Next()
		if errors.Is(err, iterator.Done) {
			return out, nil
		}
		if err != nil {
			return nil, err
		}
		var d versionDoc
		if err := snap.DataTo(&d); err != nil {
			return nil, err
		}
		v, err := d.toVersion(appID)
		if err != nil {
			return nil, err
		}
		out = append(out, *v)
	}
}

// GetVersion implements Store.
func (f *Firestore) GetVersion(ctx context.Context, uid, appID string, n int) (*Version, error) {
	snap, err := f.appRef(uid, appID).Collection("versions").Doc(VersionDocID(n)).Get(ctx)
	if isNotFound(err) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	var d versionDoc
	if err := snap.DataTo(&d); err != nil {
		return nil, err
	}
	return d.toVersion(appID)
}

// ---- public apps ------------------------------------------------------------

// PublishApp implements Store.
func (f *Firestore) PublishApp(ctx context.Context, uid, appID string, raw json.RawMessage, name string, version int) (bool, error) {
	ref := f.publicRef(appID)
	m, err := rawToMap(raw)
	if err != nil {
		return false, err
	}
	published := false
	err = f.c.RunTransaction(ctx, func(ctx context.Context, tx *firestore.Transaction) error {
		published = false
		now := time.Now().UTC()
		snap, err := tx.Get(ref)
		if err == nil {
			var d publicAppDoc
			if err := snap.DataTo(&d); err != nil {
				return err
			}
			if d.OwnerUID != uid {
				return nil
			}
			published = true
			return tx.Update(ref, []firestore.Update{
				{Path: "name", Value: name}, {Path: "version", Value: version},
				{Path: "dsl", Value: m}, {Path: "dslJson", Value: string(raw)}, {Path: "updatedAt", Value: now},
			})
		}
		if !isNotFound(err) {
			return err
		}
		published = true
		return tx.Create(ref, publicAppDoc{OwnerUID: uid, Name: name, Version: int64(version), DSL: m, DSLJSON: string(raw), CreatedAt: now, UpdatedAt: now})
	})
	if err != nil {
		return false, err
	}
	return published, nil
}

// GetPublicApp implements Store.
func (f *Firestore) GetPublicApp(ctx context.Context, appID string) (*PublicApp, error) {
	snap, err := f.publicRef(appID).Get(ctx)
	if isNotFound(err) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	var d publicAppDoc
	if err := snap.DataTo(&d); err != nil {
		return nil, err
	}
	return d.toPublicApp(appID), nil
}

// ---- records ----------------------------------------------------------------

// ListRecords implements Store.
func (f *Firestore) ListRecords(ctx context.Context, rs RecordSet, limit int) ([]Record, error) {
	q := f.recordsCol(rs).OrderBy("createdAt", firestore.Desc)
	if limit > 0 {
		q = q.Limit(limit)
	}
	it := q.Documents(ctx)
	defer it.Stop()
	out := []Record{}
	for {
		snap, err := it.Next()
		if errors.Is(err, iterator.Done) {
			return out, nil
		}
		if err != nil {
			return nil, err
		}
		var d recordDoc
		if err := snap.DataTo(&d); err != nil {
			return nil, err
		}
		out = append(out, *d.toRecord(snap.Ref.ID))
	}
}

// GetRecord implements Store.
func (f *Firestore) GetRecord(ctx context.Context, rs RecordSet, id string) (*Record, error) {
	snap, err := f.recRef(rs, id).Get(ctx)
	if isNotFound(err) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	var d recordDoc
	if err := snap.DataTo(&d); err != nil {
		return nil, err
	}
	return d.toRecord(id), nil
}

// CreateRecord implements Store.
func (f *Firestore) CreateRecord(ctx context.Context, rs RecordSet, rec Record) (*Record, bool, error) {
	ref := f.recRef(rs, rec.ID)
	var out *Record
	created := false
	err := f.c.RunTransaction(ctx, func(ctx context.Context, tx *firestore.Transaction) error {
		created = false
		snap, err := tx.Get(ref)
		if err == nil {
			var d recordDoc
			if err := snap.DataTo(&d); err != nil {
				return err
			}
			out = d.toRecord(rec.ID)
			return nil
		}
		if !isNotFound(err) {
			return err
		}
		now := time.Now().UTC()
		d := recordDoc{Entity: rs.Entity, Data: rec.Data, ClientID: rec.ClientID, Author: rec.Author, AuthorName: rec.AuthorName, CreatedAt: now, UpdatedAt: now}
		if err := tx.Create(ref, d); err != nil {
			return err
		}
		out = d.toRecord(rec.ID)
		created = true
		return nil
	})
	if err != nil {
		return nil, false, err
	}
	return out, created, nil
}

// UpdateRecord implements Store.
func (f *Firestore) UpdateRecord(ctx context.Context, rs RecordSet, id string, data map[string]any) (*Record, error) {
	ref := f.recRef(rs, id)
	_, err := ref.Update(ctx, []firestore.Update{
		{Path: "data", Value: data},
		{Path: "updatedAt", Value: time.Now().UTC()},
	})
	if isNotFound(err) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return f.GetRecord(ctx, rs, id)
}

// DeleteRecord implements Store.
func (f *Firestore) DeleteRecord(ctx context.Context, rs RecordSet, id string) error {
	ref := f.recRef(rs, id)
	if err := f.deleteMarks(ctx, ref); err != nil {
		return err
	}
	_, err := ref.Delete(ctx, firestore.Exists)
	if isNotFound(err) {
		return ErrNotFound
	}
	return err
}

// DeleteAllRecords implements Store.
func (f *Firestore) DeleteAllRecords(ctx context.Context, rs RecordSet) error {
	col := f.recordsCol(rs)
	for {
		it := col.Limit(200).Documents(ctx)
		docs, err := it.GetAll()
		if err != nil {
			return err
		}
		if len(docs) == 0 {
			return nil
		}
		bw := f.c.BulkWriter(ctx)
		for _, d := range docs {
			if _, err := bw.Delete(d.Ref); err != nil {
				return err
			}
		}
		bw.End()
	}
}
