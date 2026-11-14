package appconfig

import (
	"log/slog"
	"net/http"

	"github.com/jphacks/kz_2601/backend/internal/httpx"
	"github.com/jphacks/kz_2601/backend/internal/moderation/textfilter"
	"github.com/jphacks/kz_2601/backend/pkg/dsl"
)

// textNotAllowedMsg is shown as is; it never says which word matched.
const textNotAllowedMsg = "表示できない言葉が含まれています"

// WithTextFilter sets TEXT_FILTER (textfilter.ModeFromEnv); "" = enforce.
func WithTextFilter(m textfilter.Mode) Option {
	return func(h *Handler) { h.textMode = m }
}

// checkText (#321): text other people see (shared records, member names,
// listings) must not contain a word of textfilter. Logs the surface and the
// category only, never the text. TEXT_FILTER=log only logs, off skips.
func (h *Handler) checkText(r *http.Request, surface, field string, values ...string) error {
	if h.textMode == textfilter.ModeOff {
		return nil
	}
	for _, v := range values {
		cat, bad := textfilter.Check(v)
		if !bad {
			continue
		}
		if h.textMode == textfilter.ModeLog {
			slog.InfoContext(r.Context(), "moderation: text would be refused", slog.String("surface", surface), slog.String("category", string(cat)))
			return nil
		}
		slog.InfoContext(r.Context(), "moderation: text refused", slog.String("surface", surface), slog.String("category", string(cat)))
		return apiErr(http.StatusUnprocessableEntity, httpx.CodeTextNotAllowed, textNotAllowedMsg).With("field", field)
	}
	return nil
}

// checkRecordText checks the string / text fields of data and the author
// name, on public/shared entities only: a private entity's records are the
// creator's own.
func (h *Handler) checkRecordText(r *http.Request, ent *dsl.Entity, data map[string]any, authorName string) error {
	if !ent.IsShared() {
		return nil
	}
	for i := range ent.Fields {
		f := &ent.Fields[i]
		if f.Type != dsl.FieldString && f.Type != dsl.FieldText {
			continue
		}
		if s, ok := data[f.ID].(string); ok {
			if err := h.checkText(r, "record", f.ID, s); err != nil {
				return err
			}
		}
	}
	return h.checkText(r, "record", "authorName", authorName)
}

// checkListingText checks what 「探す」 shows of a listing.
func (h *Handler) checkListingText(r *http.Request, name, desc, author string, tags []string) error {
	if err := h.checkText(r, "listing", "name", name); err != nil {
		return err
	}
	if err := h.checkText(r, "listing", "description", desc); err != nil {
		return err
	}
	if err := h.checkText(r, "listing", "authorName", author); err != nil {
		return err
	}
	return h.checkText(r, "listing", "tags", tags...)
}
