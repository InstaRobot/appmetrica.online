package deletion

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
	"github.com/google/uuid"

	"appmetrica.online/backend/internal/chstore"
)

// Запросы job deletion (05-data-model.md, QDelete*; пользователь appmetrica_rw с ALTER DELETE). Имена таблиц
// без базы: база — из подключения (CLICKHOUSE_DATABASE). Отличия от канонического текста (OQ-062):
//   - установки пользователя — только те, чей первый владелец (linked_user) — U, а не все установки
//     с событиями U: на общем устройстве с первым владельцем u2 удаляются лишь события U (FR-019);
//   - rejected_events ищутся по install_id без учёта регистра и по user_id в виде JSON-строки
//     ("u-42", а также "a\/b" для слеша), а не по подстроке user_id: короткий user_id вроде "1"
//     совпал бы с чужими событиями; payload_sample хранится в каноничном виде ingest.PayloadSample,
//     поэтому экранирование клиента ("\u007531") на поиск не влияет;
//   - mutations_sync и lightweight_deletes_sync задаются в контексте запроса, а не только в профиле пользователя.
const (
	qLinkedInstalls = `SELECT install_id FROM identity_links
WHERE app_id = {app_id:UUID}
GROUP BY install_id
HAVING argMinMerge(first_user) = {user_id:String}`

	qDeleteEventsByUser = `DELETE FROM events
WHERE app_id = {app_id:UUID} AND (user_id = {user_id:String} OR install_id IN {install_ids:Array(UUID)})`

	qDeleteEventsByInstalls = `DELETE FROM events WHERE app_id = {app_id:UUID} AND install_id IN {install_ids:Array(UUID)}`

	// qFirstDayByUser и qFirstDayByInstalls — UTC-дата самого раннего события, которое удалит qDeleteEventsByUser
	// или qDeleteEventsByInstalls, и число таких событий (min пустого набора — 1970-01-01).
	qFirstDayByUser = `SELECT min(event_date) AS day, count() AS n FROM events
WHERE app_id = {app_id:UUID} AND (user_id = {user_id:String} OR install_id IN {install_ids:Array(UUID)})`

	qFirstDayByInstalls = `SELECT min(event_date) AS day, count() AS n FROM events
WHERE app_id = {app_id:UUID} AND install_id IN {install_ids:Array(UUID)}`

	qDeleteInstallFirstSeen = `ALTER TABLE install_first_seen DELETE
WHERE app_id = {app_id:UUID} AND install_id IN {install_ids:Array(UUID)}`

	qDeleteUserFirstSeen = `ALTER TABLE user_first_seen DELETE WHERE app_id = {app_id:UUID} AND user_id = {user_id:String}`

	qDeleteIdentityLinks = `ALTER TABLE identity_links DELETE
WHERE app_id = {app_id:UUID} AND install_id IN {install_ids:Array(UUID)}`

	// qDeleteRejectedMentions — с условиями rejectedByInstalls и/или rejectedByUser (непустые наборы строк).
	qDeleteRejectedMentions = `ALTER TABLE rejected_events DELETE WHERE app_id = {app_id:UUID} AND (%s)`
	rejectedByInstalls      = `multiSearchAnyCaseInsensitive(payload_sample, {install_needles:Array(String)})`
	rejectedByUser          = `multiSearchAny(payload_sample, {user_needles:Array(String)})`

	// subject_type=app: все таблицы приложения, включая daily_stats.
	qDeleteAppTemplate = `ALTER TABLE %s DELETE WHERE app_id = {app_id:UUID}`

	// qFlushAsyncInserts — сброс очереди async insert сервера перед удалением. Блок, который сервер уже принял
	// в очередь, остаётся в ней и после того, как клиент ingest отвалился по таймауту, не дождавшись ack
	// (wait_for_async_insert=1 принятого не отменяет), и пишется по таймеру сервера — при задержке сброса уже
	// после DELETE. Сброс синхронный: по возвращении все принятые блоки лежат в таблицах и попадают под DELETE.
	qFlushAsyncInserts = `SYSTEM FLUSH ASYNC INSERT QUEUE`
)

// appScopedTables — таблицы ClickHouse с app_id; events — первой, агрегаты после неё.
var appScopedTables = []string{"events", "install_first_seen", "user_first_seen", "identity_links", "rejected_events", "daily_stats"}

// maxNeedles — multiSearchAny принимает не больше 255 строк; часть места оставлена под user_needles.
const maxNeedles = 200

type chPurger struct{ conn driver.Conn }

// NewClickHousePurger — Purger поверх соединения ClickHouse рабочего пользователя.
func NewClickHousePurger(conn driver.Conn) Purger { return chPurger{conn: conn} }

func (p chPurger) Ping(ctx context.Context) error {
	if err := p.conn.Ping(ctx); err != nil {
		return fmt.Errorf("clickhouse: ping: %w", err)
	}
	return nil
}

// InstallFirstDay — UTC-дата самого раннего события установки; ok=false — событий нет.
func (p chPurger) InstallFirstDay(ctx context.Context, appID, installID uuid.UUID) (time.Time, bool, error) {
	return p.firstDay(ctx, qFirstDayByInstalls, clickhouse.Parameters{
		"app_id": appID.String(), "install_ids": chstore.UUIDArrayParam([]uuid.UUID{installID}),
	})
}

// UserFirstDay — UTC-дата самого раннего события из тех, что удалит PurgeUser; ok=false — событий нет.
func (p chPurger) UserFirstDay(ctx context.Context, appID uuid.UUID, userID string) (time.Time, bool, error) {
	installs, err := p.linkedInstalls(ctx, appID, userID)
	if err != nil {
		return time.Time{}, false, err
	}
	return p.firstDay(ctx, qFirstDayByUser, clickhouse.Parameters{
		"app_id": appID.String(), "user_id": chstore.StringParam(userID), "install_ids": chstore.UUIDArrayParam(installs),
	})
}

func (p chPurger) firstDay(ctx context.Context, query string, params clickhouse.Parameters) (time.Time, bool, error) {
	var (
		day time.Time
		n   uint64
	)
	if err := p.conn.QueryRow(clickhouse.Context(ctx, clickhouse.WithParameters(params)), query).Scan(&day, &n); err != nil {
		return time.Time{}, false, fmt.Errorf("clickhouse: first event day: %w", err)
	}
	return day, n > 0, nil
}

// PurgeInstall — события, first-seen, связь с пользователем и упоминания в rejected_events установки.
// user_first_seen не трогается: пользователь мог быть и на других установках.
func (p chPurger) PurgeInstall(ctx context.Context, appID, installID uuid.UUID) error {
	if err := p.flushAsyncInserts(ctx); err != nil {
		return err
	}
	installs := []uuid.UUID{installID}
	params := clickhouse.Parameters{"app_id": appID.String(), "install_ids": chstore.UUIDArrayParam(installs)}
	if err := p.exec(ctx, "events", qDeleteEventsByInstalls, params); err != nil {
		return err
	}
	return p.purgeInstallAggregates(ctx, appID, installs, nil)
}

// PurgeUser — установки, первый владелец которых user, и все события с user_id = user. Порядок: события,
// first-seen, rejected_events и в конце identity_links — повтор после сбоя снова найдёт установки.
func (p chPurger) PurgeUser(ctx context.Context, appID uuid.UUID, userID string) error {
	if err := p.flushAsyncInserts(ctx); err != nil {
		return err
	}
	installs, err := p.linkedInstalls(ctx, appID, userID)
	if err != nil {
		return err
	}
	params := clickhouse.Parameters{"app_id": appID.String(), "user_id": chstore.StringParam(userID), "install_ids": chstore.UUIDArrayParam(installs)}
	if err := p.exec(ctx, "events", qDeleteEventsByUser, params); err != nil {
		return err
	}
	if err := p.exec(ctx, "user_first_seen", qDeleteUserFirstSeen, params); err != nil {
		return err
	}
	return p.purgeInstallAggregates(ctx, appID, installs, userNeedles(userID))
}

func (p chPurger) PurgeApp(ctx context.Context, appID uuid.UUID) error {
	if err := p.flushAsyncInserts(ctx); err != nil {
		return err
	}
	params := clickhouse.Parameters{"app_id": appID.String()}
	for _, table := range appScopedTables {
		if err := p.exec(ctx, table, fmt.Sprintf(qDeleteAppTemplate, table), params); err != nil {
			return err
		}
	}
	return nil
}

// purgeInstallAggregates — install_first_seen, rejected_events (installs и userNeedles) и identity_links.
func (p chPurger) purgeInstallAggregates(ctx context.Context, appID uuid.UUID, installs []uuid.UUID, users []string) error {
	params := clickhouse.Parameters{"app_id": appID.String(), "install_ids": chstore.UUIDArrayParam(installs)}
	if len(installs) > 0 {
		if err := p.exec(ctx, "install_first_seen", qDeleteInstallFirstSeen, params); err != nil {
			return err
		}
	}
	chunks := needleChunks(installs)
	if len(chunks) == 0 && len(users) > 0 {
		chunks = [][]string{nil}
	}
	for i, chunk := range chunks {
		var conds []string
		rejected := clickhouse.Parameters{"app_id": appID.String()}
		if len(chunk) > 0 {
			conds = append(conds, rejectedByInstalls)
			rejected["install_needles"] = chstore.StringArrayParam(chunk)
		}
		if i == 0 && len(users) > 0 {
			conds = append(conds, rejectedByUser)
			rejected["user_needles"] = chstore.StringArrayParam(users)
		}
		query := fmt.Sprintf(qDeleteRejectedMentions, strings.Join(conds, " OR "))
		if err := p.exec(ctx, "rejected_events", query, rejected); err != nil {
			return err
		}
	}
	if len(installs) > 0 {
		if err := p.exec(ctx, "identity_links", qDeleteIdentityLinks, params); err != nil {
			return err
		}
	}
	return nil
}

func (p chPurger) linkedInstalls(ctx context.Context, appID uuid.UUID, userID string) ([]uuid.UUID, error) {
	var rows []struct {
		InstallID uuid.UUID `ch:"install_id"`
	}
	ctx = clickhouse.Context(ctx, clickhouse.WithParameters(clickhouse.Parameters{"app_id": appID.String(), "user_id": chstore.StringParam(userID)}))
	if err := p.conn.Select(ctx, &rows, qLinkedInstalls); err != nil {
		return nil, fmt.Errorf("clickhouse: user installs: %w", err)
	}
	out := make([]uuid.UUID, 0, len(rows))
	for _, r := range rows {
		out = append(out, r.InstallID)
	}
	return out, nil
}

func (p chPurger) flushAsyncInserts(ctx context.Context) error { return FlushAsyncInserts(ctx, p.conn) }

// FlushAsyncInserts записывает в таблицы всё, что сервер принял в очередь async insert (qFlushAsyncInserts):
// перед удалением и перед копией событий приложения (шаг copy слияния), чтобы принятый, но ещё не записанный
// блок не остался в источнике.
func FlushAsyncInserts(ctx context.Context, conn driver.Conn) error {
	if err := conn.Exec(ctx, qFlushAsyncInserts); err != nil {
		return fmt.Errorf("clickhouse: flush async insert queue: %w", err)
	}
	return nil
}

// exec выполняет DELETE / ALTER … DELETE и ждёт завершения мутации на всех репликах.
func (p chPurger) exec(ctx context.Context, table, query string, params clickhouse.Parameters) error {
	ctx = clickhouse.Context(ctx,
		clickhouse.WithParameters(params),
		clickhouse.WithSettings(clickhouse.Settings{"mutations_sync": 2, "lightweight_deletes_sync": 2}),
	)
	if err := p.conn.Exec(ctx, query); err != nil {
		return fmt.Errorf("clickhouse: delete from %s: %w", table, err)
	}
	return nil
}

// userNeedles — user_id так, как он записан JSON-строкой в payload_sample: с кавычками, кодировка
// encoding/json без HTML-экранирования — та же, что у ingest.PayloadSample, который снимает экранирование
// клиента; вариант "\/" для слеша (JSONEncoder в Swift) — для строк, записанных до канонизации (TTL 7 дней).
func userNeedles(userID string) []string {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(userID) // строку json кодирует всегда
	plain := strings.TrimSuffix(buf.String(), "\n")
	out := []string{plain}
	if slashed := strings.ReplaceAll(plain, "/", `\/`); slashed != plain {
		out = append(out, slashed)
	}
	return out
}

// needleChunks — install_id строками, порциями не больше maxNeedles.
func needleChunks(ids []uuid.UUID) [][]string {
	var out [][]string
	for start := 0; start < len(ids); start += maxNeedles {
		end := min(start+maxNeedles, len(ids))
		chunk := make([]string, 0, end-start)
		for _, id := range ids[start:end] {
			chunk = append(chunk, id.String())
		}
		out = append(out, chunk)
	}
	return out
}
