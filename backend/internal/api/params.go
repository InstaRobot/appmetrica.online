package api

import (
	"context"
	"fmt"
	"regexp"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	openapi_types "github.com/oapi-codegen/runtime/types"

	gen "appmetrica.online/backend/internal/gen/api"
	"appmetrica.online/backend/internal/identity"
	"appmetrica.online/backend/internal/reports"
)

// Пределы периода отчётов (06-api-contracts.md §2.4.4, каталог ошибок §9.3): from..to включительно.
const (
	maxRangeDays     = 366
	maxHourRangeDays = 7

	maxAppVersionLength = 64 // FilterAppVersion maxLength
	maxOSNameLength     = 32 // FilterOsName maxLength
)

var countryPattern = regexp.MustCompile(`^[A-Z]{2}$`)

// commonParams — общие параметры отчётов (from, to, granularity, environment, unit, фильтры) в типах
// strict-сервера. oapi-codegen не проверяет enum и ограничения схем query-параметров — это делает
// parseCommonParams.
type commonParams struct {
	From, To    openapi_types.Date
	Granularity *gen.Granularity
	Environment *gen.Environment
	Unit        *gen.Unit
	Platform    *[]gen.Platform
	AppVersion  *[]string
	OsName      *[]string
	Country     *[]string
}

// reportParams — проверенные параметры отчёта приложения.
type reportParams struct {
	reports.Params
	Granularity gen.Granularity // day по умолчанию
}

// paramError — 400 invalid_request или range_too_large с details.field (06-api-contracts.md §1.3).
// Присланные значения в message не цитируются.
type paramError struct {
	code    string
	field   string
	message string
	maxDays int // только для range_too_large
}

func invalidParam(field, message string) *paramError {
	return &paramError{code: "invalid_request", field: field, message: message}
}

func (e *paramError) response(ctx context.Context) gen.BadRequestJSONResponse {
	details := map[string]any{"field": e.field}
	if e.maxDays > 0 {
		details["max_days"] = e.maxDays
	}
	return gen.BadRequestJSONResponse(apiError(ctx, e.code, e.message, details))
}

// parseCommonParams проверяет общие параметры и собирает параметры отчёта приложения appID в его TZ loc:
// from и to — календарные даты в TZ приложения, границы в UTC считает reports.Period.
func parseCommonParams(in commonParams, appID uuid.UUID, loc *time.Location) (reportParams, *paramError) {
	out := reportParams{
		Params: reports.Params{
			AppID:    appID,
			Location: loc,
			Env:      reports.EnvProduction,
			Unit:     identity.Persons,
			Period:   reports.Period{From: in.From.Time, To: in.To.Time},
		},
		Granularity: gen.GranularityDay,
	}

	if in.Unit != nil {
		// Пустой unit= — ошибка: значение по умолчанию действует, только если параметра нет.
		u, err := identity.ParseUnit(string(*in.Unit))
		if *in.Unit == "" || err != nil {
			return reportParams{}, invalidParam("unit", "unit: допустимо persons или installs.")
		}
		out.Unit = u
	}
	if in.Environment != nil {
		if !in.Environment.Valid() {
			return reportParams{}, invalidParam("environment", "environment: допустимо production или development.")
		}
		out.Env = string(*in.Environment)
	}
	if in.Granularity != nil {
		if !in.Granularity.Valid() {
			return reportParams{}, invalidParam("granularity", "granularity: допустимо hour, day, week или month.")
		}
		out.Granularity = *in.Granularity
	}

	var perr *paramError
	if out.Filters, perr = parseFilters(in); perr != nil {
		return reportParams{}, perr
	}

	if in.From.After(in.To.Time) {
		return reportParams{}, invalidParam("from", "from не может быть позже to.")
	}
	days := out.Period.Days()
	if days > maxRangeDays {
		return reportParams{}, &paramError{
			code: "range_too_large", field: "to", maxDays: maxRangeDays,
			message: fmt.Sprintf("Диапазон не может превышать %d дней (запрошено %d).", maxRangeDays, days),
		}
	}
	if out.Granularity == gen.GranularityHour && days > maxHourRangeDays {
		return reportParams{}, &paramError{
			code: "range_too_large", field: "to", maxDays: maxHourRangeDays,
			message: fmt.Sprintf("Диапазон для granularity=hour не может превышать %d дней (запрошено %d).",
				maxHourRangeDays, days),
		}
	}
	return out, nil
}

// parseFilters проверяет значения фильтров по схемам api.yaml (enum Platform, maxLength, pattern страны).
func parseFilters(in commonParams) (reports.Filters, *paramError) {
	var f reports.Filters
	if in.Platform != nil {
		for _, p := range *in.Platform {
			if !p.Valid() {
				return reports.Filters{}, invalidParam("platform", "platform: неизвестная платформа.")
			}
			f.Platform = append(f.Platform, string(p))
		}
	}
	if in.AppVersion != nil {
		for _, v := range *in.AppVersion {
			if utf8.RuneCountInString(v) > maxAppVersionLength {
				return reports.Filters{}, invalidParam("app_version",
					fmt.Sprintf("app_version: не длиннее %d символов.", maxAppVersionLength))
			}
		}
		f.AppVersion = *in.AppVersion
	}
	if in.OsName != nil {
		for _, v := range *in.OsName {
			if utf8.RuneCountInString(v) > maxOSNameLength {
				return reports.Filters{}, invalidParam("os_name",
					fmt.Sprintf("os_name: не длиннее %d символов.", maxOSNameLength))
			}
		}
		f.OSName = *in.OsName
	}
	if in.Country != nil {
		for _, v := range *in.Country {
			if !countryPattern.MatchString(v) {
				return reports.Filters{}, invalidParam("country",
					"country: код страны ISO 3166-1 alpha-2 из двух заглавных латинских букв.")
			}
		}
		f.Country = *in.Country
	}
	return f, nil
}
