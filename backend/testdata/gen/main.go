// Команда gen генерирует тестовые данные backend.
//
//	go run ./testdata/gen geo                  # testdata/geo/test-city.mmdb из testutil.GeoNetworks
//	go run ./testdata/gen -seed 42             # testdata/fixtures/scenario.ndjson — сценарий golden-тестов отчётов
//	go run ./testdata/gen -seed 42 -stats      # таблица ожидаемых DAU/новых/D1 для testdata/fixtures/README.md
//	go run ./testdata/gen -mode perf -stats    # объём перф-синтетики 100k DAU × 90 дней (T-035)
//	go run ./testdata/gen -mode perf -dau 1000 -days 30 -out /tmp/perf.ndjson
//
// Режим perf — синтетика перф-теста отчётов (testutil.GeneratePerf, 22-test-strategy.md §6.1): по умолчанию
// ≈ 27 млн событий (≈ 20 ГБ NDJSON), поэтому -out обязателен ("-" — stdout); сам перф-тест
// (make perf-reports) вставляет события в ClickHouse напрямую, без файла.
package main

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"

	"appmetrica.online/backend/internal/chstore"
	"appmetrica.online/backend/internal/testutil"
)

const usage = `usage: go run ./testdata/gen geo
       go run ./testdata/gen [-seed N] [-out FILE] [-stats]
       go run ./testdata/gen -mode perf [-seed N] [-dau N] [-days N] (-out FILE|- | -stats)`

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

	perfDefaults := testutil.DefaultPerfConfig()
	fs := flag.NewFlagSet("gen", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	mode := fs.String("mode", "scenario", "scenario — фикстура golden-тестов, perf — синтетика перф-теста")
	seed := fs.Uint64("seed", testutil.ScenarioSeed, "seed")
	out := fs.String("out", testutil.ScenarioPath(), "куда записать NDJSON (perf: обязателен, \"-\" — stdout)")
	stats := fs.Bool("stats", false, "напечатать сводку вместо записи файла")
	dau := fs.Int("dau", perfDefaults.DAU, "perf: DAU основного приложения")
	days := fs.Int("days", perfDefaults.Days, "perf: дней в окне")
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
	var set []string
	fs.Visit(func(f *flag.Flag) { set = append(set, f.Name) })

	switch *mode {
	case "perf":
		cfg := perfDefaults
		cfg.Seed, cfg.DAU, cfg.Days = *seed, *dau, *days
		return runPerf(cfg, *out, slices.Contains(set, "out"), *stats, stdout, stderr)
	case "scenario":
		if slices.Contains(set, "dau") || slices.Contains(set, "days") {
			fmt.Fprintln(stderr, "-dau и -days — только для -mode perf")
			fmt.Fprintln(stderr, usage)
			return 2
		}
	default:
		fmt.Fprintf(stderr, "неизвестный -mode %q\n", *mode)
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

// runPerf печатает сводку (-stats) или пишет синтетику потоком в файл out ("-" — stdout).
func runPerf(cfg testutil.PerfConfig, out string, outSet, stats bool, stdout, stderr io.Writer) int {
	if err := cfg.Validate(); err != nil {
		fmt.Fprintln(stderr, err)
		fmt.Fprintln(stderr, usage)
		return 2
	}
	switch {
	case stats:
		st, err := testutil.GeneratePerf(cfg, func([]chstore.EventRow) error { return nil })
		if err == nil {
			_, err = io.WriteString(stdout, testutil.PerfStatsTable(cfg, st))
		}
		return report(stderr, err)
	case !outSet:
		fmt.Fprintln(stderr, "-mode perf: укажите -out FILE (\"-\" — stdout) или -stats")
		fmt.Fprintln(stderr, usage)
		return 2
	case out == "-":
		_, err := testutil.WritePerf(stdout, cfg)
		return report(stderr, err)
	}
	return report(stderr, writePerfFile(out, cfg))
}

func writePerfFile(path string, cfg testutil.PerfConfig) (err error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil { //nolint:gosec // каталог задаёт разработчик
		return err
	}
	f, err := os.Create(path) //nolint:gosec // путь задаёт разработчик
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, f.Close()) }()
	_, err = testutil.WritePerf(f, cfg)
	return err
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
