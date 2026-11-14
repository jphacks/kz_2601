// Command dslgen generates the Android (Kotlin) DSL types from the canonical
// JSON Schema (docs/dsl/dsl.schema.json, embedded as pkg/dsl/schema.json),
// so Android never drifts from iOS / backend (#351 §7).
//
//	cd backend && go run ./cmd/dslgen        # or: make dsl-gen
//
// The output is committed. TestGeneratedKotlinIsUpToDate fails when it is stale.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/jphacks/kz_2601/backend/pkg/dsl"
)

// defaultOut is relative to backend/.
const defaultOut = "../android/dsl/src/main/kotlin/jp/kz2601/metamon/dsl/generated"

func main() {
	out := flag.String("out", defaultOut, "directory for the generated .kt files (replaced as a whole)")
	flag.Parse()
	if err := write(*out); err != nil {
		fmt.Fprintln(os.Stderr, "dslgen:", err)
		os.Exit(1)
	}
}

func write(dir string) error {
	files, err := Generate(dsl.SchemaJSON())
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	// Remove stale .kt files so a renamed output does not linger.
	existing, err := filepath.Glob(filepath.Join(dir, "*.kt"))
	if err != nil {
		return err
	}
	for _, p := range existing {
		if _, ok := files[filepath.Base(p)]; !ok {
			if err := os.Remove(p); err != nil {
				return err
			}
		}
	}
	names := make([]string, 0, len(files))
	for n := range files {
		names = append(names, n)
	}
	slices.Sort(names)
	for _, n := range names {
		if err := os.WriteFile(filepath.Join(dir, n), []byte(files[n]), 0o644); err != nil {
			return err
		}
	}
	fmt.Printf("dslgen: wrote %s (%s)\n", dir, strings.Join(names, ", "))
	return nil
}
