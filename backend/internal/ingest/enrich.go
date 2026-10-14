package ingest

import (
	"bytes"
	"encoding/json"
	"net/netip"
	"strconv"
	"strings"
	"time"

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

// PayloadSample — rejected_events.payload_sample: тело батча или JSON события, переписанные токен за
// токеном в компактный каноничный JSON (encoding/json без HTML-экранирования). Пары с ключом "ip" на любом
// уровне (контекст батча, события, вложенные объекты) выбрасываются целиком, строковые user_id, похожие
// на email или телефон, заменяются на "[redacted]" (AC-1.5.2 — и при batch-ошибке, когда до проверки
// событий дело не дошло), строковые app_key обрезаются до key_prefix: сэмпл отдаётся Read API как
// example_payload, полный ключ записи в нём недопустим. Ключи и значения сравниваются после снятия
// JSON-экранирования, поэтому
// "\u0069p" и "ivan\u0040example.com" распознаются, а дубликаты ключей обрабатываются каждый. Неразбираемое
// тело (invalid_json, обрезанное) даёт префикс до последнего полного токена: незакрытая строка в сэмпл не
// попадает. Длина ≤ MaxPayloadSampleBytes, обрезка только по границе токена — значение длиннее лимита
// выбрасывается целиком. Удаление ищет user_id в сэмпле в этом же каноничном виде (deletion.userNeedles).
func PayloadSample(raw []byte) string {
	s := sampler{dec: json.NewDecoder(bytes.NewReader(raw))}
	s.dec.UseNumber()
	s.enc = json.NewEncoder(&s.tmp)
	s.enc.SetEscapeHTML(false)
	s.run()
	return s.out.String()
}

// sampler — потоковый переписчик JSON для PayloadSample.
type sampler struct {
	dec   *json.Decoder
	enc   *json.Encoder
	tmp   bytes.Buffer // буфер enc для одной строки
	out   bytes.Buffer
	stack []sampleFrame
	// skipDepth > 0 — внутри пропускаемого объекта или массива (значение ключа ip).
	skipDepth int
	// skipValue — следующий токен (значение ключа ip) пропустить.
	skipValue bool
	// userID — следующий токен — значение ключа user_id.
	userID bool
	// appKey — следующий токен — значение ключа app_key.
	appKey bool
}

// sampleFrame — открытый контейнер: для объекта key=true, когда ожидается ключ; n — выведено элементов или пар.
type sampleFrame struct {
	object bool
	key    bool
	n      int
}

func (s *sampler) run() {
	for {
		tok, err := s.dec.Token()
		if err != nil {
			return
		}
		if s.skipValue || s.skipDepth > 0 {
			s.skipValue = false
			if d, ok := tok.(json.Delim); ok {
				if d == '{' || d == '[' {
					s.skipDepth++
				} else {
					s.skipDepth--
				}
			}
			continue
		}
		if !s.emit(tok) {
			return
		}
		if len(s.stack) == 0 {
			return // корневое значение закончилось; хвост тела не интересен
		}
	}
}

// emit выводит токен в позиции ключа или значения. false — лимит длины исчерпан.
func (s *sampler) emit(tok json.Token) bool {
	top := s.top()
	if top != nil && top.object && top.key {
		if _, ok := tok.(json.Delim); ok { // "}"
			return s.closeContainer("}")
		}
		key, _ := tok.(string) // Token выдаёт ключи объекта только строками
		switch {
		case strings.EqualFold(key, "ip"):
			s.skipValue = true
			return true
		case strings.EqualFold(key, "user_id"):
			s.userID = true
		case strings.EqualFold(key, "app_key"):
			s.appKey = true
		}
		sep := ""
		if top.n > 0 {
			sep = ","
		}
		if !s.write(sep + s.encodeString(key) + ":") {
			return false
		}
		top.n++
		top.key = false
		return true
	}
	sep := ""
	if top != nil && !top.object && top.n > 0 {
		sep = ","
	}
	switch v := tok.(type) {
	case json.Delim:
		if v == '}' || v == ']' {
			return s.closeContainer(string(v))
		}
		s.userID, s.appKey = false, false
		if !s.write(sep + string(v)) {
			return false
		}
		if top != nil {
			top.n++
		}
		s.stack = append(s.stack, sampleFrame{object: v == '{', key: v == '{'})
		return true
	case string:
		if s.userID && LooksLikePII(v) {
			v = redactedUserID
		}
		if s.appKey {
			v = keyPrefix(v)
		}
		return s.finishValue(sep + s.encodeString(v))
	case json.Number:
		return s.finishValue(sep + v.String())
	case bool:
		return s.finishValue(sep + strconv.FormatBool(v))
	default: // nil
		return s.finishValue(sep + "null")
	}
}

// finishValue выводит скалярное значение и переводит родителя к следующему элементу или ключу.
func (s *sampler) finishValue(text string) bool {
	s.userID, s.appKey = false, false
	if !s.write(text) {
		return false
	}
	if top := s.top(); top != nil {
		if top.object {
			top.key = true
		} else {
			top.n++
		}
	}
	return true
}

func (s *sampler) closeContainer(text string) bool {
	if !s.write(text) {
		return false
	}
	s.stack = s.stack[:len(s.stack)-1]
	if top := s.top(); top != nil && top.object {
		top.key = true
	}
	return true
}

func (s *sampler) top() *sampleFrame {
	if len(s.stack) == 0 {
		return nil
	}
	return &s.stack[len(s.stack)-1]
}

// write добавляет text, если он помещается в MaxPayloadSampleBytes.
func (s *sampler) write(text string) bool {
	if s.out.Len()+len(text) > MaxPayloadSampleBytes {
		return false
	}
	s.out.WriteString(text)
	return true
}

func (s *sampler) encodeString(v string) string {
	s.tmp.Reset()
	_ = s.enc.Encode(v) // строку json кодирует всегда
	return strings.TrimSuffix(s.tmp.String(), "\n")
}

// marshalCompact — JSON без HTML-экранирования: сэмпл показывается владельцу как есть.
func marshalCompact(v any) []byte {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(v) // map[string]json.RawMessage из разобранного JSON кодируется всегда
	return bytes.TrimSuffix(buf.Bytes(), []byte("\n"))
}
