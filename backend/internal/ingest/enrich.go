package ingest

import (
	"bytes"
	"encoding/json"
	"net/netip"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"appmetrica.online/backend/internal/chstore"
	"appmetrica.online/backend/internal/gen/ingestapi"
	"appmetrica.online/backend/internal/geo"
)

// MaxPayloadSampleBytes — лимит rejected_events.payload_sample.
const MaxPayloadSampleBytes = 4096

// contextIPNone в context.ip отключает гео (server-to-server без IP пользователя).
const contextIPNone = "none"

// Enricher — шаг конвейера после валидации события (02-tech-spec.md Story 1.4, FR-004, FR-024):
// итоговый контекст, environment и гео. IP клиента и context.ip используются только для поиска
// и дальше Enrich не уходят.
type Enricher struct {
	Geo geo.Lookuper
	// AcceptDevelopment — INGEST_ACCEPT_DEVELOPMENT; false — события development считаются принятыми,
	// но не пишутся (аварийная разгрузка).
	AcceptDevelopment bool
}

// Enrich сливает context батча (batchCtx из ValidateBatchContext) и события, подставляет apps.platform
// и environment=production по умолчанию и определяет гео: context.ip, если задан ("none" — без гео),
// иначе b.ClientIP. Заполняет ev.Resolved. Возвращает warning geo_ip_invalid для невалидного context.ip
// и write=false, если событие development нужно молча отбросить.
func (e Enricher) Enrich(b *Batch, batchCtx ContextFields, ev *Event) (warning *ingestapi.IngestWarning, write bool) {
	merged := MergeContext(batchCtx, ev.Context)
	ev.Resolved = resolveContext(merged, string(b.Key.AppPlatform))
	if !e.AcceptDevelopment && ev.Resolved.Environment == string(ingestapi.EnvironmentDevelopment) {
		return nil, false
	}
	addr, warning := geoAddr(ev.Index, merged, b.ClientIP)
	if addr.IsValid() {
		loc := e.Geo.Lookup(addr)
		ev.Resolved.Country, ev.Resolved.Region, ev.Resolved.City = loc.Country, loc.Region, loc.City
	}
	return warning, true
}

// geoAddr выбирает адрес для гео; нулевой адрес — гео не определяется.
func geoAddr(index int, merged ContextFields, clientIP netip.Addr) (netip.Addr, *ingestapi.IngestWarning) {
	ip, ok := merged["ip"]
	switch {
	case !ok:
		return clientIP, nil
	case ip == contextIPNone:
		return netip.Addr{}, nil
	}
	addr, err := netip.ParseAddr(ip)
	if err != nil {
		return netip.Addr{}, &ingestapi.IngestWarning{
			Index: index, Code: ingestapi.WarningCodeGeoIpInvalid,
			Message: "context.ip не является IP-адресом; гео не определено.",
		}
	}
	return addr.WithZone(""), nil
}

// NewEventRow — строка appmetrica.events для события после CorrectEventTime и Enrich.
func NewEventRow(b *Batch, ev *Event) chstore.EventRow {
	rc := ev.Resolved
	utc := ev.EventTime.UTC()
	return chstore.EventRow{
		AppID:       b.Key.AppID,
		EventDate:   time.Date(utc.Year(), utc.Month(), utc.Day(), 0, 0, 0, 0, time.UTC),
		EventTime:   ev.EventTime,
		ClientTime:  ev.Timestamp,
		SentAt:      b.SentAt,
		ReceivedAt:  b.ReceivedAt,
		InsertID:    ev.InsertID,
		Name:        string(ev.Name),
		InstallID:   ev.InstallID,
		UserID:      ev.UserID,
		SessionID:   ev.SessionID,
		DurationMs:  uint32(ev.DurationMs), //nolint:gosec // ValidateEvent ограничивает 0..86 400 000
		Environment: rc.Environment,
		Platform:    rc.Platform,
		OSName:      rc.OSName,
		OSVersion:   rc.OSVersion,
		AppVersion:  rc.AppVersion,
		AppBuild:    rc.AppBuild,
		DeviceModel: rc.DeviceModel,
		DeviceType:  rc.DeviceType,
		Locale:      rc.Locale,
		Country:     rc.Country,
		Region:      rc.Region,
		City:        rc.City,
		WriteKeyID:  b.Key.WriteKeyID,
	}
}

// PayloadSample — rejected_events.payload_sample: JSON события или тела батча без context.ip
// (верхний уровень и events[i]) и без user_id, похожих на email или телефон (AC-1.5.2 — такие значения
// заменяются на "[redacted]" и при batch-ошибке, когда до проверки событий дело не дошло), длиной не
// больше MaxPayloadSampleBytes, обрезанный по границе символа. Если JSON не разбирается (invalid_json,
// обрезанное тело), значения ключей "ip" и PII-подобных "user_id" затираются по шаблону.
func PayloadSample(raw []byte) string {
	if sample, ok := stripContextIP(raw); ok {
		return truncateUTF8(sample, MaxPayloadSampleBytes)
	}
	// Берём запас, чтобы пара "ip": "…" на границе обрезки попала под шаблон целиком.
	head := []byte(truncateUTF8(raw, 2*MaxPayloadSampleBytes))
	head = ipValueRe.ReplaceAll(head, []byte(`"ip":null`))
	head = userIDValueRe.ReplaceAllFunc(head, redactPIIUserIDMatch)
	return truncateUTF8(head, MaxPayloadSampleBytes)
}

// ipValueRe — пара "ip": значение, включая незакрытую строку в конце обрезанного тела.
var ipValueRe = regexp.MustCompile(`(?i)"ip"\s*:\s*(?:"(?:[^"\\]|\\.)*"?|[^,}\]\s]*)`)

// userIDValueRe — пара "user_id": "строка", включая незакрытую строку в конце обрезанного тела.
var userIDValueRe = regexp.MustCompile(`(?i)"user_id"\s*:\s*"((?:[^"\\]|\\.)*)"?`)

// redactPIIUserIDMatch заменяет найденную по шаблону пару на "user_id":"[redacted]", если значение
// (как есть, с JSON-экранированием) похоже на email или телефон. Незакрытая строка в конце обрезанного
// тела может быть началом email или телефона — она затирается всегда. Иначе пара остаётся как есть.
func redactPIIUserIDMatch(match []byte) []byte {
	sub := userIDValueRe.FindSubmatch(match)
	if len(sub) < 2 {
		return match
	}
	closed := bytes.HasSuffix(match, append(append([]byte(`"`), sub[1]...), '"'))
	if closed && !LooksLikePII(string(sub[1])) {
		return match
	}
	return []byte(`"user_id":"` + redactedUserID + `"`)
}

// stripContextIP удаляет ip из context объекта и из context каждого элемента events и заменяет
// PII-подобные user_id (объекта и каждого события) на "[redacted]". ok=false — raw не JSON-объект.
// Без изменений возвращается исходный raw (форматирование клиента сохраняется).
func stripContextIP(raw []byte) (out []byte, ok bool) {
	var obj map[string]json.RawMessage
	if json.Unmarshal(raw, &obj) != nil || obj == nil {
		return nil, false
	}
	changed := dropContextIP(obj)
	changed = redactPIIUserID(obj) || changed
	for k, v := range obj {
		if !strings.EqualFold(k, "events") {
			continue
		}
		var events []json.RawMessage
		if json.Unmarshal(v, &events) != nil {
			continue
		}
		eventsChanged := false
		for i, ev := range events {
			var evObj map[string]json.RawMessage
			if json.Unmarshal(ev, &evObj) != nil || evObj == nil {
				continue
			}
			ipDropped := dropContextIP(evObj)
			if redactPIIUserID(evObj) || ipDropped {
				events[i] = marshalCompact(evObj)
				eventsChanged = true
			}
		}
		if eventsChanged {
			obj[k] = marshalCompact(events)
			changed = true
		}
	}
	if !changed {
		return raw, true
	}
	return marshalCompact(obj), true
}

// dropContextIP удаляет ip из всех ключей context объекта. Регистр ключей не важен: encoding/json
// сопоставляет поля структур без учёта регистра.
func dropContextIP(obj map[string]json.RawMessage) bool {
	changed := false
	for k, v := range obj {
		if !strings.EqualFold(k, "context") {
			continue
		}
		var ctx map[string]json.RawMessage
		if json.Unmarshal(v, &ctx) != nil || ctx == nil {
			continue
		}
		dropped := false
		for ck := range ctx {
			if strings.EqualFold(ck, "ip") {
				delete(ctx, ck)
				dropped = true
			}
		}
		if dropped {
			obj[k] = marshalCompact(ctx)
			changed = true
		}
	}
	return changed
}

// redactPIIUserID заменяет строковые значения ключей user_id объекта, похожие на email или телефон,
// на "[redacted]" (AC-1.5.2). Регистр ключа не важен, как и в dropContextIP.
func redactPIIUserID(obj map[string]json.RawMessage) bool {
	changed := false
	for k, v := range obj {
		if !strings.EqualFold(k, "user_id") {
			continue
		}
		var s string
		if json.Unmarshal(v, &s) != nil || !LooksLikePII(s) {
			continue
		}
		obj[k] = json.RawMessage(`"` + redactedUserID + `"`)
		changed = true
	}
	return changed
}

// marshalCompact — JSON без HTML-экранирования: сэмпл показывается владельцу как есть.
func marshalCompact(v any) []byte {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(v) // map[string]json.RawMessage и []json.RawMessage из разобранного JSON кодируются всегда
	return bytes.TrimSuffix(buf.Bytes(), []byte("\n"))
}

// truncateUTF8 обрезает b до n байт, не разрезая многобайтовый символ.
func truncateUTF8(b []byte, n int) string {
	if len(b) <= n {
		return string(b)
	}
	for n > 0 && !utf8.RuneStart(b[n]) {
		n--
	}
	return string(b[:n])
}
