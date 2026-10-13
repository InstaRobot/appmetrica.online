// Команда gen генерирует тестовые данные backend.
//
//	go run ./testdata/gen geo                  # testdata/geo/test-city.mmdb из testutil.GeoNetworks
//	go run ./testdata/gen -seed 42             # testdata/fixtures/scenario.ndjson — сценарий golden-тестов отчётов
//	go run ./testdata/gen -seed 42 -stats      # таблица ожидаемых DAU/новых/D1 для testdata/fixtures/README.md
//
// Генератор объёма для перф-тестов (22-test-strategy.md §6.1) добавляется сюда же отдельной подкомандой.
package main

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"appmetrica.online/backend/internal/testutil"
)

const usage = `usage: go run ./testdata/gen geo
       go run ./testdata/gen [-seed N] [-out FILE] [-stats]`

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	if len(args) > 0 && args[0] == "geo" {
		if len(args) != 1 {
			fmt.Fprintln(stderr, usage)
			return 2
		}
		return report(stderr, writeGeoDB(testutil.GeoDBPath()))
	}

	fs := flag.NewFlagSet("gen", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	seed := fs.Uint64("seed", testutil.ScenarioSeed, "seed сценария")
	out := fs.String("out", testutil.ScenarioPath(), "куда записать scenario.ndjson")
	stats := fs.Bool("stats", false, "напечатать таблицу ожидаемых чисел для README вместо записи файла")
	if err := fs.Parse(args); err != nil || fs.NArg() != 0 {
		if errors.Is(err, flag.ErrHelp) {
			fmt.Fprintln(stdout, usage)
			return 0
		}
		if err != nil {
			fmt.Fprintln(stderr, err)
		}
		fmt.Fprintln(stderr, usage)
		return 2
	}

	if *stats {
		table, err := testutil.ScenarioStatsTable(testutil.GenerateScenario(*seed))
		if err == nil {
			_, err = io.WriteString(stdout, table)
		}
		return report(stderr, err)
	}
	var buf bytes.Buffer
	if err := testutil.WriteScenario(&buf, *seed); err != nil {
		return report(stderr, err)
	}
	return report(stderr, writeFile(*out, buf.Bytes()))
}

func report(stderr io.Writer, err error) int {
	if err != nil {
		fmt.Fprintln(stderr, "gen:", err)
		return 1
	}
	return 0
}

func writeGeoDB(path string) error {
	var buf bytes.Buffer
	if err := testutil.WriteGeoDB(&buf); err != nil {
		return err
	}
	return writeFile(path, buf.Bytes())
}

func writeFile(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil { //nolint:gosec // каталог testdata в репозитории
		return err
	}
	return os.WriteFile(path, data, 0o644) //nolint:gosec // тестовые данные в репозитории
}
