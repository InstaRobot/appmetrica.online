package worker

import (
	"fmt"
	"html"
	"regexp"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/stretchr/testify/require"

	"appmetrica.online/backend/internal/reports"
)

func ptr[T any](v T) *T { return &v }

// 2026-10-12 — понедельник: Δ к тому же дню прошлой недели подписывается «к пн».
var digestDate = time.Date(2026, 10, 12, 0, 0, 0, 0, time.UTC)

func TestDigestJob_Schedule(t *testing.T) {
	job := DigestJob(nil, nil, nil, nil, nil, "https://appmetrica.online")
	require.Equal(t, "digest", job.Name)
	require.Equal(t, time.Minute, job.Every, "FR-020: проверка каждую минуту")
	require.GreaterOrEqual(t, job.Timeout, 3*time.Minute, "три попытки Telegram с retry_after не обрываются тиком")
}

// OQ-012: HTML, сверху дата и сумма DAU, по строке на приложение по убыванию DAU, снизу ссылка на портфель.
func TestFormatDigest_Layout(t *testing.T) {
	rows := []digestRow{
		{Name: "Трекер Android", Row: reports.PortfolioRow{
			ActiveDate: 8902, ActiveDeltaPct: ptr(-1.2), NewDate: 861, SessionsDate: 20114, RetentionD1: ptr(0.3149),
		}},
		{Name: "Заметки iOS", Row: reports.PortfolioRow{
			ActiveDate: 12480, ActiveDeltaPct: ptr(4.1), NewDate: 1204, SessionsDate: 31022, RetentionD1: ptr(0.38),
		}},
		{Name: "Бот <FAQ> & co", Row: reports.PortfolioRow{}},
	}
	want := "<b>Портфель за 12.10.2026</b> · DAU 21 382\n" +
		"<b>Заметки iOS</b> · DAU 12 480 (▲4,1 % к пн) · новые 1 204 · сессии 31 022 · D1 38 %\n" +
		"<b>Трекер Android</b> · DAU 8 902 (▼1,2 % к пн) · новые 861 · сессии 20 114 · D1 31 %\n" +
		"<b>Бот &lt;FAQ&gt; &amp; co</b> · DAU 0 · новые 0 · сессии 0 · D1 —\n" +
		"<a href=\"https://appmetrica.online/\">Открыть портфель</a>"
	require.Equal(t, want, formatDigest(digestDate, rows, "https://appmetrica.online"))
}

func TestFormatDigest_DeltaAndWeekday(t *testing.T) {
	tests := []struct {
		delta *float64
		day   time.Time
		want  string
	}{
		{ptr(0.0), digestDate, "DAU 10 (0,0 % к пн)"},
		{ptr(12.0), digestDate.AddDate(0, 0, 1), "DAU 10 (▲12,0 % к вт)"},
		{ptr(-100.0), digestDate.AddDate(0, 0, 2), "DAU 10 (▼100,0 % к ср)"},
		{ptr(250.5), digestDate.AddDate(0, 0, 3), "DAU 10 (▲250,5 % к чт)"},
		{ptr(1.0), digestDate.AddDate(0, 0, 4), "(▲1,0 % к пт)"},
		{ptr(1.0), digestDate.AddDate(0, 0, 5), "(▲1,0 % к сб)"},
		{ptr(1.0), digestDate.AddDate(0, 0, 6), "(▲1,0 % к вс)"},
		{nil, digestDate, "DAU 10 · новые"}, // неделю назад активных не было
	}
	for _, tt := range tests {
		rows := []digestRow{{Name: "A", Row: reports.PortfolioRow{ActiveDate: 10, ActiveDeltaPct: tt.delta}}}
		require.Contains(t, formatDigest(tt.day, rows, "https://x.example"), tt.want)
	}
}

func TestFormatDigest_Numbers(t *testing.T) {
	tests := []struct {
		n    uint64
		want string
	}{
		{0, "0"},
		{7, "7"},
		{999, "999"},
		{1000, "1 000"},
		{12480, "12 480"},
		{1234567, "1 234 567"},
		{100000000, "100 000 000"},
	}
	for _, tt := range tests {
		require.Equal(t, tt.want, groupThousands(tt.n), tt.n)
	}

	// D1 — целые проценты, округление до ближайшего.
	for v, want := range map[float64]string{0: "D1 0 %", 0.005: "D1 1 %", 0.384: "D1 38 %", 0.385: "D1 39 %", 1: "D1 100 %"} {
		rows := []digestRow{{Name: "A", Row: reports.PortfolioRow{RetentionD1: ptr(v)}}}
		require.Contains(t, formatDigest(digestDate, rows, "https://x.example"), want, v)
	}
}

// OQ-012: не больше 30 строк приложений, остальные — «и ещё N приложений»; сумма DAU — по всем.
func TestFormatDigest_TruncatesAfter30Apps(t *testing.T) {
	for _, tt := range []struct {
		apps int
		tail string
	}{
		{30, ""},
		{31, "и ещё 1 приложение"},
		{32, "и ещё 2 приложения"},
		{35, "и ещё 5 приложений"},
		{41, "и ещё 11 приложений"},
		{51, "и ещё 21 приложение"},
		{54, "и ещё 24 приложения"},
		{142, "и ещё 112 приложений"},
	} {
		rows := make([]digestRow, tt.apps)
		var sum uint64
		for i := range rows {
			rows[i] = digestRow{Name: fmt.Sprintf("app-%03d", i), Row: reports.PortfolioRow{ActiveDate: uint64(i)}}
			sum += uint64(i)
		}
		text := formatDigest(digestDate, rows, "https://x.example")
		lines := strings.Split(text, "\n")
		require.Contains(t, lines[0], "DAU "+groupThousands(sum), tt.apps)
		appLines := min(tt.apps, 30)
		require.Contains(t, lines[1], fmt.Sprintf("app-%03d", tt.apps-1), "первым — больший DAU")
		if tt.tail == "" {
			require.Len(t, lines, appLines+2)
			continue
		}
		require.Len(t, lines, appLines+3)
		require.Equal(t, tt.tail, lines[appLines+1])
	}
}

// Строки приложений не выводят сообщение за лимит sendMessage (4096 символов без разметки): длинные имена и
// большие числа уходят в «и ещё N приложений».
func TestFormatDigest_FitsTelegramLimit(t *testing.T) {
	rows := make([]digestRow, 40)
	for i := range rows {
		rows[i] = digestRow{
			Name: fmt.Sprintf("%02d %s", i, strings.Repeat("&", 61)),
			Row: reports.PortfolioRow{
				ActiveDate: 9_000_000 - uint64(i), ActiveDeltaPct: ptr(-100.0), NewDate: 9_000_000,
				SessionsDate: 99_000_000, RetentionD1: ptr(1.0),
			},
		}
	}
	text := formatDigest(digestDate, rows, "https://appmetrica.online")
	visible := html.UnescapeString(regexp.MustCompile(`<[^>]+>`).ReplaceAllString(text, ""))
	require.LessOrEqual(t, utf8.RuneCountInString(visible), 4096)
	lines := strings.Split(text, "\n")
	shown := len(lines) - 3
	require.Less(t, shown, 30)
	require.Positive(t, shown)
	require.Equal(t, fmt.Sprintf("и ещё %d %s", 40-shown, pluralApps(40-shown)), lines[len(lines)-2])
}

// Равный DAU — по имени, как в таблице портфеля.
func TestFormatDigest_TiesByName(t *testing.T) {
	rows := []digestRow{
		{Name: "Б", Row: reports.PortfolioRow{ActiveDate: 5}},
		{Name: "а", Row: reports.PortfolioRow{ActiveDate: 5}},
		{Name: "В", Row: reports.PortfolioRow{ActiveDate: 9}},
	}
	lines := strings.Split(formatDigest(digestDate, rows, "https://x.example"), "\n")
	require.True(t, strings.HasPrefix(lines[1], "<b>В</b>"))
	require.True(t, strings.HasPrefix(lines[2], "<b>а</b>"))
	require.True(t, strings.HasPrefix(lines[3], "<b>Б</b>"))
}
