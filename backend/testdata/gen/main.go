// Команда gen генерирует тестовые данные backend.
//
//	go run ./testdata/gen geo   # testdata/geo/test-city.mmdb из testutil.GeoNetworks
//
// Генератор объёма для перф-тестов (22-test-strategy.md §6.1) добавляется сюда же отдельной подкомандой.
package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"

	"appmetrica.online/backend/internal/testutil"
)

func main() {
	if len(os.Args) != 2 || os.Args[1] != "geo" {
		fmt.Fprintln(os.Stderr, "usage: go run ./testdata/gen geo")
		os.Exit(2)
	}
	if err := writeGeoDB(testutil.GeoDBPath()); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func writeGeoDB(path string) error {
	var buf bytes.Buffer
	if err := testutil.WriteGeoDB(&buf); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("gen: %w", err)
	}
	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil { //nolint:gosec // тестовые данные в репозитории
		return fmt.Errorf("gen: %w", err)
	}
	return nil
}
