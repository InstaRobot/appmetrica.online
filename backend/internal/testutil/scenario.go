package testutil

import (
	"errors"
	"io"
	"path/filepath"
	"time"

	"github.com/google/uuid"

	"appmetrica.online/backend/internal/chstore"
)

// ScenarioSeed — seed, с которым сгенерирован закоммиченный testdata/fixtures/scenario.ndjson.
const ScenarioSeed uint64 = 42

// ScenarioApp — приложение сценария golden-тестов отчётов.
type ScenarioApp struct {
	Code       string // короткий код для таблиц README: msk, nyc, tyo
	ID         uuid.UUID
	Name       string
	Platform   string
	Timezone   string
	WriteKeyID uuid.UUID
}

// ScenarioApps — три приложения сценария.
var ScenarioApps []ScenarioApp

// ScenarioFirstDay и ScenarioLastDay — границы сценария (локальные даты приложений, включительно).
var ScenarioFirstDay, ScenarioLastDay time.Time

// ScenarioPath — backend/testdata/fixtures/scenario.ndjson.
func ScenarioPath() string { return filepath.Join(FixturesDir(), "scenario.ndjson") }

// GenerateScenario строит события сценария для seed.
func GenerateScenario(seed uint64) []chstore.EventRow {
	_ = seed
	return nil
}

// WriteScenario пишет сценарий для seed в формате NDJSON-фикстуры.
func WriteScenario(w io.Writer, seed uint64) error {
	_, _ = w, seed
	return errors.New("testutil: WriteScenario не реализован")
}

// ScenarioDay — ожидаемые метрики приложения за локальный день (production, unit=persons).
type ScenarioDay struct {
	Day        string // YYYY-MM-DD в TZ приложения
	Active     int
	New        int
	D1Returned int // сколько из New активны на следующий день
}

// ScenarioStats считает DAU, новых и D1 по дням сценария для приложения app.
func ScenarioStats(rows []chstore.EventRow, app ScenarioApp) ([]ScenarioDay, error) {
	_, _ = rows, app
	return nil, errors.New("testutil: ScenarioStats не реализован")
}

// ScenarioStatsTable — markdown-таблица ожидаемых чисел для README фикстур.
func ScenarioStatsTable(rows []chstore.EventRow) (string, error) {
	_ = rows
	return "", errors.New("testutil: ScenarioStatsTable не реализован")
}
