package gtfsstatic

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
)

func TestReadFile(t *testing.T) {
	dir := t.TempDir()
	write := func(name string, b []byte) string {
		t.Helper()
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, b, 0o600); err != nil {
			t.Fatal(err)
		}
		return p
	}
	good := write("feed.zip", zipDir(t, "basic", ""))

	t.Run("valid zip", func(t *testing.T) {
		tb, err := ReadFile(good, Limits{})
		if err != nil {
			t.Fatal(err)
		}
		if tb.StopName("S1") != "香林坊" || tb.RouteName("R1") != "10" || tb.TripRoute["T3"] != "R2" {
			t.Fatalf("table %v %v %v", tb.Stops, tb.Routes, tb.TripRoute)
		}
	})

	t.Run("missing file", func(t *testing.T) {
		_, err := ReadFile(filepath.Join(dir, "none.zip"), Limits{})
		if !errors.Is(err, fs.ErrNotExist) {
			t.Fatalf("err = %v, want not exist", err)
		}
		var pe *fs.PathError
		if !errors.As(err, &pe) || errors.Is(err, ErrBad) || errors.Is(err, ErrTooLarge) {
			t.Fatalf("err = %v, want the os error as is", err)
		}
	})

	t.Run("over ZipBytes", func(t *testing.T) {
		st, err := os.Stat(good)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := ReadFile(good, Limits{ZipBytes: st.Size() - 1}); !errors.Is(err, ErrTooLarge) {
			t.Fatalf("err = %v, want ErrTooLarge", err)
		}
		if _, err := ReadFile(good, Limits{ZipBytes: st.Size()}); err != nil {
			t.Fatalf("exactly ZipBytes: %v", err)
		}
	})

	t.Run("not a zip", func(t *testing.T) {
		p := write("feed.txt", []byte("stop_id,stop_name\nS1,x\n"))
		if _, err := ReadFile(p, Limits{}); !errors.Is(err, ErrBad) {
			t.Fatalf("err = %v, want ErrBad", err)
		}
	})
}
