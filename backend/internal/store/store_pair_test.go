package store

import (
	"os"
	"strconv"
	"testing"
	"time"
)

// bothStores runs f against Memory and, when FIRESTORE_EMULATOR_HOST is set,
// against the Firestore emulator (ids get a per-run suffix there, #550).
func bothStores(t *testing.T, f func(t *testing.T, s Store, sfx string)) {
	t.Helper()
	t.Run("memory", func(t *testing.T) { f(t, NewMemory(), "") })
	t.Run("firestore", func(t *testing.T) { f(t, emulatorStore(t), runSuffix()) })
}

// emulatorStore connects to the emulator's demo project only; never staging/prod.
func emulatorStore(t *testing.T) *Firestore {
	t.Helper()
	if os.Getenv("FIRESTORE_EMULATOR_HOST") == "" {
		t.Skip("FIRESTORE_EMULATOR_HOST not set")
	}
	f, err := NewFirestore(t.Context(), "demo-kz2601", "(default)")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = f.Close() })
	return f
}

func runSuffix() string { return "-" + strconv.FormatInt(time.Now().UnixNano(), 36) }
