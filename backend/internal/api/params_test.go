package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	openapi_types "github.com/oapi-codegen/runtime/types"
	"github.com/stretchr/testify/require"

	gen "appmetrica.online/backend/internal/gen/api"
	"appmetrica.online/backend/internal/identity"
	"appmetrica.online/backend/internal/reports"
)

var paramsApp = uuid.MustParse("5ce0a000-0000-4000-8000-000000000001")

func day(t *testing.T, s string) openapi_types.Date {
	t.Helper()
	d, err := time.Parse(time.DateOnly, s)
	require.NoError(t, err)
	return openapi_types.Date{Time: d}
}

func ptr[T any](v T) *T { return &v }

func TestParseCommonParams(t *testing.T) {
	msk, err := time.LoadLocation("Europe/Moscow")
	require.NoError(t, err)

	base := func(t *testing.T) commonParams {
		return commonParams{From: day(t, "2026-10-01"), To: day(t, "2026-10-07")}
	}

	t.Run("defaults", func(t *testing.T) {
		got, perr := parseCommonParams(base(t), paramsApp, msk)
		require.Nil(t, perr)
		require.Equal(t, reportParams{
			Params: reports.Params{
				AppID:    paramsApp,
				Location: msk,
				Env:      reports.EnvProduction,
				Unit:     identity.Persons,
				Period:   reports.Period{From: day(t, "2026-10-01").Time, To: day(t, "2026-10-07").Time},
			},
			Granularity: gen.GranularityDay,
		}, got)
	})

	t.Run("explicit", func(t *testing.T) {
		in := base(t)
		in.Granularity = ptr(gen.GranularityHour)
		in.Environment = ptr(gen.EnvironmentDevelopment)
		in.Unit = ptr(gen.UnitInstalls)
		in.Platform = &[]gen.Platform{gen.PlatformIos, gen.PlatformAndroid}
		in.AppVersion = &[]string{"2.4.0", ""}
		in.OsName = &[]string{"iOS"}
		in.Country = &[]string{"RU"}

		got, perr := parseCommonParams(in, paramsApp, msk)
		require.Nil(t, perr)
		require.Equal(t, gen.GranularityHour, got.Granularity)
		require.Equal(t, reports.EnvDevelopment, got.Env)
		require.Equal(t, identity.Installs, got.Unit)
		require.Equal(t, reports.Filters{
			Platform:   []string{"ios", "android"},
			AppVersion: []string{"2.4.0", ""},
			OSName:     []string{"iOS"},
			Country:    []string{"RU"},
		}, got.Filters)
	})

	t.Run("empty_filter_lists", func(t *testing.T) {
		in := base(t)
		in.Platform = &[]gen.Platform{}
		in.Country = &[]string{}
		got, perr := parseCommonParams(in, paramsApp, msk)
		require.Nil(t, perr)
		require.Empty(t, got.Filters.Platform)
		require.Empty(t, got.Filters.Country)
	})

	t.Run("bounds_in_app_timezone", func(t *testing.T) {
		got, perr := parseCommonParams(base(t), paramsApp, msk)
		require.Nil(t, perr)
		start, end := got.Period.Bounds(got.Location)
		require.Equal(t, "2026-09-30T21:00:00Z", start.Format(time.RFC3339))
		require.Equal(t, "2026-10-07T21:00:00Z", end.Format(time.RFC3339))
	})

	// AC-027.4 (AC-5.2.3): unit=people, from > to, 367 дней, hour на 8 дней.
	t.Run("ranges", func(t *testing.T) {
		for _, tc := range []struct {
			name     string
			from, to string
			gran     gen.Granularity
			unit     gen.Unit
			code     string
			field    string
			maxDays  int
		}{
			{name: "unit_people", from: "2026-10-01", to: "2026-10-07", unit: "people", code: "invalid_request", field: "unit"},
			{name: "from_after_to", from: "2026-10-12", to: "2026-10-06", code: "invalid_request", field: "from"},
			{name: "range_367_days", from: "2025-10-11", to: "2026-10-12", code: "range_too_large", field: "to", maxDays: 366},
			{name: "range_400_days", from: "2025-09-01", to: "2026-10-12", code: "range_too_large", field: "to", maxDays: 366},
			{name: "hour_8_days", from: "2026-10-01", to: "2026-10-08", gran: gen.GranularityHour, code: "range_too_large", field: "to", maxDays: 7},
			{name: "hour_367_days", from: "2025-10-11", to: "2026-10-12", gran: gen.GranularityHour, code: "range_too_large", field: "to", maxDays: 366},
		} {
			t.Run(tc.name, func(t *testing.T) {
				in := commonParams{From: day(t, tc.from), To: day(t, tc.to)}
				if tc.gran != "" {
					in.Granularity = &tc.gran
				}
				if tc.unit != "" {
					in.Unit = &tc.unit
				}
				_, perr := parseCommonParams(in, paramsApp, msk)
				require.NotNil(t, perr)
				require.Equal(t, tc.code, perr.code)
				require.Equal(t, tc.field, perr.field)
				require.Equal(t, tc.maxDays, perr.maxDays)
				require.NotEmpty(t, perr.message)
			})
		}

		// Границы включительно: 366 дней и hour на 7 дней — допустимы, from = to — один день.
		for _, ok := range []struct {
			from, to string
			gran     gen.Granularity
		}{
			{"2025-10-12", "2026-10-12", gen.GranularityDay},
			{"2027-10-12", "2028-10-11", gen.GranularityMonth}, // 366 дней с 29 февраля 2028
			{"2026-10-01", "2026-10-07", gen.GranularityHour},
			{"2026-10-01", "2026-10-01", gen.GranularityHour},
			{"2026-10-01", "2026-10-08", gen.GranularityWeek},
		} {
			_, perr := parseCommonParams(commonParams{From: day(t, ok.from), To: day(t, ok.to), Granularity: &ok.gran}, paramsApp, msk)
			require.Nil(t, perr, "%s..%s %s", ok.from, ok.to, ok.gran)
		}
	})

	// AC-2.1.4.
	t.Run("invalid_unit", func(t *testing.T) {
		for _, u := range []gen.Unit{"devices", "Persons", ""} {
			in := base(t)
			in.Unit = &u
			_, perr := parseCommonParams(in, paramsApp, msk)
			require.NotNil(t, perr, "unit=%q", u)
			require.Equal(t, "invalid_request", perr.code)
			require.Equal(t, "unit", perr.field)
		}
	})

	// AC-028.4 (AC-5.1.4): дата не разбирается по format: date — 400 invalid_request с details.field,
	// значение в ответе не цитируется.
	t.Run("invalid_date", func(t *testing.T) {
		srv := &Server{}
		w := &gen.ServerInterfaceWrapper{
			Handler: gen.NewStrictHandlerWithOptions(srv, nil, gen.StrictHTTPServerOptions{
				RequestErrorHandlerFunc:  invalidRequest,
				ResponseErrorHandlerFunc: srv.internalError,
			}),
			ErrorHandlerFunc: invalidRequest,
		}
		for _, tc := range []struct {
			name, query, field string
			handler            http.HandlerFunc
		}{
			{"portfolio_month_13", "date=2026-13-01", "date", w.GetPortfolio},
			{"portfolio_feb_30", "date=2026-02-30", "date", w.GetPortfolio},
			{"portfolio_russian_format", "date=12.10.2026", "date", w.GetPortfolio},
			{"portfolio_include_archived", "include_archived=yes", "include_archived", w.GetPortfolio},
			{"summary_from", "from=2026-13-01&to=2026-10-12", "from", w.GetAudienceSummary},
			{"summary_to_missing", "from=2026-10-01", "to", w.GetAudienceSummary},
		} {
			t.Run(tc.name, func(t *testing.T) {
				req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/v1/x?"+tc.query, nil)
				req.SetPathValue("app_id", paramsApp.String())
				rec := httptest.NewRecorder()
				tc.handler(rec, req)
				require.Equal(t, http.StatusBadRequest, rec.Code)
				var body struct {
					Error struct {
						Code    string         `json:"code"`
						Message string         `json:"message"`
						Details map[string]any `json:"details"`
					} `json:"error"`
				}
				require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
				require.Equal(t, "invalid_request", body.Error.Code)
				require.Equal(t, map[string]any{"field": tc.field}, body.Error.Details)
				require.Contains(t, body.Error.Message, tc.field)
				require.NotContains(t, rec.Body.String(), "2026-13")
				require.NotContains(t, rec.Body.String(), "yes")
			})
		}
	})

	t.Run("invalid_values", func(t *testing.T) {
		for _, tc := range []struct {
			name  string
			edit  func(*commonParams)
			field string
		}{
			{"environment", func(p *commonParams) { p.Environment = ptr(gen.Environment("staging")) }, "environment"},
			{"granularity", func(p *commonParams) { p.Granularity = ptr(gen.Granularity("minute")) }, "granularity"},
			{"platform", func(p *commonParams) { p.Platform = &[]gen.Platform{gen.PlatformIos, "symbian"} }, "platform"},
			{"app_version_too_long", func(p *commonParams) { p.AppVersion = &[]string{strings.Repeat("9", 65)} }, "app_version"},
			{"os_name_too_long", func(p *commonParams) { p.OsName = &[]string{strings.Repeat("я", 33)} }, "os_name"},
			{"country_lowercase", func(p *commonParams) { p.Country = &[]string{"ru"} }, "country"},
			{"country_alpha3", func(p *commonParams) { p.Country = &[]string{"RUS"} }, "country"},
			{"country_empty", func(p *commonParams) { p.Country = &[]string{""} }, "country"},
			{"country_cyrillic", func(p *commonParams) { p.Country = &[]string{"РУ"} }, "country"},
		} {
			t.Run(tc.name, func(t *testing.T) {
				in := base(t)
				tc.edit(&in)
				_, perr := parseCommonParams(in, paramsApp, msk)
				require.NotNil(t, perr)
				require.Equal(t, "invalid_request", perr.code)
				require.Equal(t, tc.field, perr.field)
			})
		}

		// Длина — в символах: 64 символа кириллицы допустимы, хотя это 128 байт.
		in := base(t)
		in.AppVersion = &[]string{strings.Repeat("я", 64)}
		in.OsName = &[]string{strings.Repeat("я", 32)}
		_, perr := parseCommonParams(in, paramsApp, msk)
		require.Nil(t, perr)
	})
}

// Ответ ошибки параметров — конверт 06-api-contracts.md §1.3 с details.field (и max_days для range_too_large);
// присланные значения в тексте не цитируются.
func TestParamError_Response(t *testing.T) {
	_, perr := parseCommonParams(commonParams{
		From:        day(t, "2026-10-01"),
		To:          day(t, "2026-10-14"),
		Granularity: ptr(gen.GranularityHour),
	}, paramsApp, time.UTC)
	require.NotNil(t, perr)

	rec := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/v1/apps/x/audience/timeseries", nil)
	require.NoError(t, gen.GetAudienceTimeseries400JSONResponse{BadRequestJSONResponse: perr.response(req.Context())}.
		VisitGetAudienceTimeseriesResponse(rec))
	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.JSONEq(t, `{"error":{
		"code":"range_too_large",
		"message":"Диапазон для granularity=hour не может превышать 7 дней (запрошено 14).",
		"request_id":"",
		"details":{"field":"to","max_days":7}
	}}`, rec.Body.String())

	in := commonParams{From: day(t, "2026-10-01"), To: day(t, "2026-10-07"), Country: &[]string{"<script>"}}
	_, perr = parseCommonParams(in, paramsApp, time.UTC)
	require.NotNil(t, perr)
	rec = httptest.NewRecorder()
	require.NoError(t, gen.GetAudienceTimeseries400JSONResponse{BadRequestJSONResponse: perr.response(req.Context())}.
		VisitGetAudienceTimeseriesResponse(rec))
	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.NotContains(t, rec.Body.String(), "script")
	require.Contains(t, rec.Body.String(), `"field":"country"`)
	require.Contains(t, rec.Body.String(), `"code":"invalid_request"`)
}

// AC-006.2 (AC-6.2): split — none (по умолчанию) или platform; остальное, в том числе пустое значение, —
// 400 invalid_request с details.field=split, присланное значение не цитируется.
func TestParseSplit(t *testing.T) {
	for _, tc := range []struct {
		name string
		in   *string
		want bool
	}{
		{"default", nil, false},
		{"none", ptr("none"), false},
		{"platform", ptr("platform"), true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, perr := parseSplit(tc.in)
			require.Nil(t, perr)
			require.Equal(t, tc.want, got)
		})
	}
	for _, in := range []string{"os", "", "Platform", "platform,os"} {
		t.Run("invalid_"+in, func(t *testing.T) {
			got, perr := parseSplit(&in)
			require.False(t, got)
			require.NotNil(t, perr)
			require.Equal(t, "invalid_request", perr.code)
			require.Equal(t, "split", perr.field)
			if in != "" {
				require.NotContains(t, perr.message, in)
			}
		})
	}
}

// Неверные environment и unit портфеля — 400 до обращения к хранилищам.
func TestGetPortfolio_InvalidParams(t *testing.T) {
	srv := &Server{}
	for _, tc := range []struct {
		name   string
		params gen.GetPortfolioParams
		field  string
	}{
		{"environment", gen.GetPortfolioParams{Environment: ptr(gen.Environment("staging"))}, "environment"},
		{"unit_people", gen.GetPortfolioParams{Unit: ptr(gen.Unit("people"))}, "unit"},
		{"unit_empty", gen.GetPortfolioParams{Unit: ptr(gen.Unit(""))}, "unit"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			resp, err := srv.GetPortfolio(t.Context(), gen.GetPortfolioRequestObject{Params: tc.params})
			require.NoError(t, err)
			bad, ok := resp.(gen.GetPortfolio400JSONResponse)
			require.True(t, ok, "%T", resp)
			require.Equal(t, "invalid_request", bad.Error.Code)
			require.Equal(t, map[string]any{"field": tc.field}, *bad.Error.Details)
		})
	}
}

// AC-030.4 (AC-5.3.4): periods — day 1..90 (по умолчанию 30), week 1..52 (по умолчанию 12); вне пределов и
// неизвестная cohort_granularity — 400 invalid_request с details.field. Общие параметры проверяются как у
// остальных отчётов.
func TestParseRetentionParams(t *testing.T) {
	msk, err := time.LoadLocation("Europe/Moscow")
	require.NoError(t, err)
	base := func(t *testing.T) gen.GetRetentionParams {
		return gen.GetRetentionParams{From: day(t, "2026-09-13"), To: day(t, "2026-10-12")}
	}

	t.Run("defaults", func(t *testing.T) {
		p, q, perr := parseRetentionParams(base(t), paramsApp, msk)
		require.Nil(t, perr)
		require.Equal(t, reports.RetentionQuery{Granularity: reports.GranularityDay, Periods: reports.DefaultDayPeriods}, q)
		require.Equal(t, reports.Params{
			AppID: paramsApp, Location: msk, Env: reports.EnvProduction, Unit: identity.Persons,
			Period: reports.Period{From: day(t, "2026-09-13").Time, To: day(t, "2026-10-12").Time},
		}, p)

		in := base(t)
		in.CohortGranularity = ptr(gen.CohortGranularityWeek)
		_, q, perr = parseRetentionParams(in, paramsApp, msk)
		require.Nil(t, perr)
		require.Equal(t, reports.RetentionQuery{Granularity: reports.GranularityWeek, Periods: reports.DefaultWeekPeriods}, q)
	})

	t.Run("explicit", func(t *testing.T) {
		in := base(t)
		in.Periods = ptr(90)
		in.Environment = ptr(gen.EnvironmentDevelopment)
		in.Unit = ptr(gen.UnitInstalls)
		in.Platform = &[]gen.Platform{gen.PlatformAndroid}
		in.Country = &[]string{"RU", "DE"}
		p, q, perr := parseRetentionParams(in, paramsApp, msk)
		require.Nil(t, perr)
		require.Equal(t, reports.RetentionQuery{Granularity: reports.GranularityDay, Periods: 90}, q)
		require.Equal(t, reports.EnvDevelopment, p.Env)
		require.Equal(t, identity.Installs, p.Unit)
		require.Equal(t, reports.Filters{Platform: []string{"android"}, Country: []string{"RU", "DE"}}, p.Filters)

		in = base(t)
		in.CohortGranularity = ptr(gen.CohortGranularityWeek)
		in.Periods = ptr(52)
		_, q, perr = parseRetentionParams(in, paramsApp, msk)
		require.Nil(t, perr)
		require.Equal(t, reports.RetentionQuery{Granularity: reports.GranularityWeek, Periods: 52}, q)

		in.Periods = ptr(1)
		_, q, perr = parseRetentionParams(in, paramsApp, msk)
		require.Nil(t, perr)
		require.Equal(t, 1, q.Periods)
	})

	for _, tc := range []struct {
		name        string
		granularity *gen.CohortGranularity
		periods     *int
		mutate      func(*gen.GetRetentionParams)
		code, field string
	}{
		{name: "day_91", periods: ptr(91), code: "invalid_request", field: "periods"},
		{name: "week_53", granularity: ptr(gen.CohortGranularityWeek), periods: ptr(53), code: "invalid_request", field: "periods"},
		{name: "week_90", granularity: ptr(gen.CohortGranularityWeek), periods: ptr(90), code: "invalid_request", field: "periods"},
		{name: "zero", periods: ptr(0), code: "invalid_request", field: "periods"},
		{name: "negative", granularity: ptr(gen.CohortGranularityWeek), periods: ptr(-1), code: "invalid_request", field: "periods"},
		{name: "granularity_month", granularity: ptr(gen.CohortGranularity("month")), code: "invalid_request", field: "cohort_granularity"},
		{name: "granularity_empty", granularity: ptr(gen.CohortGranularity("")), code: "invalid_request", field: "cohort_granularity"},
		{name: "unit", mutate: func(p *gen.GetRetentionParams) { p.Unit = ptr(gen.Unit("people")) }, code: "invalid_request", field: "unit"},
		{name: "from_after_to", mutate: func(p *gen.GetRetentionParams) { p.From = day(t, "2026-10-13") }, code: "invalid_request", field: "from"},
		{name: "range_367", mutate: func(p *gen.GetRetentionParams) { p.From = day(t, "2025-10-11") }, code: "range_too_large", field: "to"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			in := base(t)
			in.CohortGranularity, in.Periods = tc.granularity, tc.periods
			if tc.mutate != nil {
				tc.mutate(&in)
			}
			_, _, perr := parseRetentionParams(in, paramsApp, msk)
			require.NotNil(t, perr)
			require.Equal(t, tc.code, perr.code)
			require.Equal(t, tc.field, perr.field)
			require.NotContains(t, perr.message, "month")
			require.NotContains(t, perr.message, "people")
		})
	}
}

// AC-033.3 (AC-5.6.4): hours — 1..168, по умолчанию 24; вне диапазона — 400 invalid_request с field=hours.
func TestParseIntegrationParams(t *testing.T) {
	hours, perr := parseIntegrationParams(gen.GetIntegrationHealthParams{})
	require.Nil(t, perr)
	require.Equal(t, reports.DefaultIntegrationHours, hours)

	for _, h := range []int{1, 24, 167, reports.MaxIntegrationHours} {
		hours, perr := parseIntegrationParams(gen.GetIntegrationHealthParams{Hours: ptr(h)})
		require.Nil(t, perr, "hours=%d", h)
		require.Equal(t, h, hours)
	}

	for _, h := range []int{0, 169, -1, -24, 1_000_000} {
		_, perr := parseIntegrationParams(gen.GetIntegrationHealthParams{Hours: ptr(h)})
		require.NotNil(t, perr, "hours=%d", h)
		require.Equal(t, "invalid_request", perr.code)
		require.Equal(t, "hours", perr.field)
		require.Contains(t, perr.message, "168")
		if h != 1_000_000 {
			require.NotContains(t, perr.message, fmt.Sprint(h), "присланное значение не цитируется")
		}
	}
}
