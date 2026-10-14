# References — appmetrica.online / greenfield MVP

> Все визуальные и продуктовые референсы, на которые опирается discovery:
> ссылки на конкурентов, документацию их API, open-source аналоги и выбранные в интервью мокапы.
> Собрано в Phase 1.1 (2026-10-13) по ответу владельца: «Яндекс AppMetrica, Open-source аналоги, Mixpanel / Amplitude».
> Используется в [01-prd.md](./01-prd.md) (competitive landscape) и [14-ux-ui-spec.md](./14-ux-ui-spec.md) (visual patterns).

---

## 1. Skip-if-none

Референсы собраны — секция-заглушка не применяется. Владелец выбрал три группы ориентиров: Яндекс AppMetrica (прямой прототип и текущий инструмент во всех приложениях), open-source аналоги (Aptabase, PostHog, Countly, Umami, плюс Plausible, OpenPanel, Rybbit, Swetrix как дополнительный срез) и Mixpanel / Amplitude (эталон HTTP Ingestion API). Скриншотов и Figma владелец не передавал; визуальный стиль выбран по ASCII-мокапам в интервью (см. §2.2). Выжимка ниже подтверждена владельцем в ходе интервью («Если что-то понял не так, поправь» — правок не последовало, интервью продолжено).

## 2. User-provided references

Референсы названы владельцем по именам продуктов; URL, цены и лимиты собраны 2026-10-13 по открытым источникам (сайты, документация, исходники на GitHub).

### 2.1 Competitor apps / websites

| # | Name | URL | Role | Why useful |
|---|---|---|---|---|
| 1 | Яндекс AppMetrica | https://appmetrica.yandex.ru (зеркало доков: https://yandex.com/support/appmetrica-io/en/llms.txt) | Direct competitor + текущий инструмент владельца | Определения метрик (DAU, новые, classic retention, сессии), UX отчётов (период «неделя», детализация час/день/неделя/месяц, TZ приложения, сегменты), эталон для 14-дневной сверки (NFR-010) |
| 2 | Aptabase | https://aptabase.com, https://github.com/aptabase/aptabase | Ближайший аналог по духу (app analytics для indie) | HTTP-контракт для приложений (`systemProps`, `isDebug`, клиентский `sessionId`), буфер в памяти + пакетная запись в ClickHouse без Kafka |
| 3 | PostHog | https://posthog.com, https://posthog.com/docs/api/capture | Market leader open-source product analytics | `$session_id` UUIDv7, `distinct_id`, cookieless-режим, архитектура ingestion (Rust capture → Kafka → ClickHouse) — как «чего не делать на нашем масштабе» |
| 4 | Countly | https://countly.com, https://support.countly.com/hc/en-us/articles/360037092072-Server-API-Reference | Baseline mobile analytics (self-hosted) | Клиентская модель сессий `begin_session` / `session_duration` / `end_session`, обязательный `device_id` |
| 5 | Umami | https://umami.is, https://docs.umami.is/docs/api/sending-stats | Baseline web analytics | Batch-эндпоинт `/api/batch` (до 500), соль с ротацией month/week/day, явные `ip`/`userAgent` для серверных прокси |
| 6 | Mixpanel Ingestion API | https://docs.mixpanel.com/reference/import-events, https://docs.mixpanel.com/reference/track-event | Эталон надёжного ingestion | `$insert_id`, частичный успех `failed_records`, отсечение будущего времени, backoff-политика |
| 7 | Amplitude HTTP V2 / Batch | https://amplitude.com/docs/apis/analytics/http-v2, https://amplitude.com/docs/apis/analytics/batch-event-upload | Эталон ingestion + структура документации | Дедупликация `insert_id` 7 дней, `events_with_invalid_fields`, блок Considerations в доках, семантика 5xx «могло записаться» |
| 8 | Plausible / OpenPanel / Rybbit / Swetrix | https://plausible.io/docs/events-api, https://github.com/Openpanel-dev/openpanel, https://github.com/rybbit-io/rybbit, https://swetrix.com/docs/events-api | Дополнительный срез privacy-first аналогов | Хранение current+previous соли (OpenPanel), вырезание версии из UA (Rybbit), отдельная таблица ошибок (Swetrix) |
| 9 | Mixpanel `/track` (как анти-пример) | https://docs.mixpanel.com/reference/track-event | What NOT to do | Отвечает `1` даже при невалидном токене — «тихие потери», которых у нас быть не должно |

### 2.2 Screenshots

Скриншотов владелец не прикладывал. Вместо них в интервью использовались ASCII-мокапы в вопросе о визуальном стиле; выбор владельца зафиксирован:

| # | Path/Chat attachment | Source | What it shows | Inferred pattern |
|---|---|---|---|---|
| 1 | ASCII-мокап «Как AppMetrica» (интервью, вопрос «Стиль») | Вариант из интервью, **выбран владельцем** | Переключатель приложения, вкладки «Обзор / Аудитория / Retention», три KPI-карточки (DAU, Новые, Сессии) со спарклайнами, под ними большой график DAU за период | Светлая тема, карточки-виджеты, синий акцент, без тёмной темы — основа app-страниц (S-05) |
| 2 | ASCII-мокап «Минимализм / портфель» (интервью, вопрос «Стиль») | Вариант из интервью, не выбран как стиль | Плотная таблица приложений с DAU, Δ w/w, Новые, D1, спарклайном 30 дней | Структура таблицы взята для главной-портфеля (S-03), выбранной владельцем отдельным ответом «Портфель», но оформлена в стиле карточек AppMetrica |
| 3 | ASCII-мокап «Плотный тёмный» (интервью) | Вариант из интервью, отклонён | Тёмная терминальная таблица | Тёмная тема и моноширинная эстетика — out of scope |

**Как ссылаться в других документах**: `[Ref 1](./00-references.md#22-screenshots)`.

### 2.3 Figma files

> **N/A для этого проекта.** Причина: владелец не использует Figma для этого инструмента; дизайн-токены задаются в [14-ux-ui-spec.md](./14-ux-ui-spec.md) на основе выбранного стиля «Как AppMetrica».

| # | File | URL | Role | How to consume |
|---|---|---|---|---|
| — | Figma-файлы отсутствуют | — | — | Токены и экраны — в 14-ux-ui-spec.md |

### 2.4 Live products

| # | Product | Link | Access | Task |
|---|---|---|---|---|
| 1 | Яндекс AppMetrica | https://appmetrica.yandex.ru | Аккаунт владельца, SDK уже стоит во всех приложениях | Во время 14-дневной сверки сравнивать DAU / новые / D1 по каждому приложению с нашим `unit=installs` (NFR-010) |
| 2 | Aptabase Cloud | https://aptabase.com/pricing | Free до 20 тыс. событий/мес | Свериться с экраном главной (4 метрики + график + топы) и debug/release-переключателем |
| 3 | PostHog Cloud EU | https://eu.posthog.com | Free 1 млн событий/мес | Посмотреть Web analytics вкладку и insight-дашборды как пример «перегруженного» UX, которого избегаем |
| 4 | Umami Cloud | https://umami.is/pricing | Hobby: 100 тыс. событий, 1 сайт | Главный экран: visitors / visits / views / bounce / duration + таблицы топов |

### 2.5 Articles / case studies / videos

| # | Title | URL | Relevance |
|---|---|---|---|
| 1 | AppMetrica Post API — import events | https://yandex.com/support/appmetrica-io/en/mobile-api/post/post-import-events | Доказательство: «без SDK» в AppMetrica невозможно (только ID из SDK, синк раз в 4 ч) |
| 2 | AppMetrica Post API — restrictions | https://yandex.com/support/appmetrica-io/en/mobile-api/post/restrictions | Лимиты: событие ≤ 14 дней, ≤ 100 000 событий на запрос, файл ≤ 1 ГБ |
| 3 | AppMetrica pricing (RU) | https://yandex.com/support/appmetrica-io/en/common/pricing/ru-currency | Free 100 млн кастомных событий/мес; Custom 300 млн = 41 338 ₽; Pro 302 850 ₽/мес |
| 4 | AppMetrica limits | https://yandex.com/support/appmetrica-io/en/common/limits | Буфер > 14 дней → остановка приёма — анти-паттерн «жёсткий стоп» |
| 5 | Aptabase — How to build your own SDK | https://github.com/aptabase/aptabase/wiki/How-to-build-your-own-SDK | Форма payload для приложений без SDK |
| 6 | Aptabase DailyUserHasher | https://github.com/aptabase/aptabase/blob/main/src/Features/Privacy/DailyUserHasher.cs | Почему privacy-хэш с дневной солью ломает retention — мотив выбора `install_id` |
| 7 | PostHog ingestion pipeline | https://posthog.com/docs/how-posthog-works/ingestion-pipeline | Kafka-архитектура как избыточная для 500 rps |
| 8 | PostHog cookieless tracking | https://posthog.com/tutorials/cookieless-tracking | Альтернатива идентификации, отклонённая из-за retention |
| 9 | OpenPanel — How it works | https://github.com/Openpanel-dev/openpanel/blob/main/apps/public/content/docs/(tracking)/how-it-works.mdx | Серверные UA (`Go-http-client`) не создают сессий — учтено в рецептах для ботов |
| 10 | Rybbit userIdService | https://github.com/rybbit-io/rybbit/blob/master/server/src/services/userId/userIdService.ts | Нормализация UA и бакетирование IP дата-центров |
| 11 | FT о приватности AppMetrica (пересказ devby, 2022) | https://devby.io/news/ft-appmetrica-yandeksa-ugrozhaet-bezopasnosti-dannyh-polzovatelei-yandeks-nichego-podobnogo-1648554936 | Репутационный мотив ухода от Яндекса |
| 12 | G2: AppMetrica vs Mixpanel vs Amplitude | https://www.g2.com/compare/appmetrica-vs-mixpanel-vs-amplitude-analytics | Слабые места AppMetrica: мобильный UI беднее, мало управления отчётами |

## 3. Competitive analysis (derived)

Выжимка по 11 продуктам. Попадает в [01-prd.md](./01-prd.md) §6 Competitive landscape. Цены и лимиты — на 2026-10-13.

| Competitor | Что делает хорошо | Что плохо | Наша позиция/differentiator |
|---|---|---|---|
| Яндекс AppMetrica | Полный набор мобильных отчётов, бесплатно до 100 млн кастомных событий/мес (системные не лимитированы), сегменты, сравнение периодов, «скопировать запрос в Reporting API» | SDK обязателен (Android, iOS, Unity, Flutter, React Native; нет web и desktop); Post API принимает только ID, уже пришедшие из SDK, синк раз в 4 ч, CSV/query string; жёсткая остановка приёма при длительном превышении; данные у третьей стороны; 13.10.2026 сервис лежал | Настоящий приём без SDK для любого нового ID, веб + боты + десктоп в одном портфеле, данные в собственном ClickHouse, определения метрик повторяем для сверки |
| Mixpanel | Надёжный `/import`: `$insert_id`, `strict=1` с `failed_records`, ~30 тыс. событий/с, EU-регион | `/track` отвечает `1` при невалидном токене; лимиты в доках противоречат (10 МБ vs 2 МБ); облако и цена за события | Берём контракт надёжности, отказываемся от тихих потерь: каждое отклонённое событие в ответе и на экране «Интеграция» |
| Amplitude | HTTP V2 с дедупом `insert_id` 7 дней, `events_with_invalid_fields`, хорошая структура доков (Considerations, таблицы полей, разбор кодов) | Free: 10 событий в батче; 30 событий/с на устройство; неясно, принимается ли валидная часть батча | Батч до 500 событий, явный частичный успех, единая страница «Ошибки и ретраи» в доках |
| Aptabase | Простой app-first HTTP API (`App-Key`, `systemProps`, `isDebug`), буфер в памяти → ClickHouse раз в 10 с, без Kafka, AGPL | Нет retention, воронок, когорт и профилей (ID меняется ежедневно); API описан только wiki-страницей; батч ≤ 25; события старше суток отклоняются | Retention через стабильный `install_id`, окно досыла 7 дней, полноценная документация API на русском |
| PostHog | Всё в одном (trends, funnels, retention, replay, флаги), Free 1 млн событий/мес, MIT | Python + Rust + Kafka + Node + ClickHouse + Postgres + Redis — тяжело для соло-ops; перегруженный UX | Один Go-бинарник + ClickHouse + Postgres на одном сервере; один экран-портфель |
| Countly | Клиентские сессии с длительностью, крэши, push; self-hosted | MongoDB; retention, воронки, когорты — только платные Flex (от $175/мес) и Enterprise; лицензия запрещает удалять бренд | Retention и профили в MVP бесплатно и self-hosted |
| Umami | Простота, MIT, batch до 500, realtime, Postgres или ClickHouse | Web-first (нужны url/hostname); соль по умолчанию раз в месяц ломает долгий retention; Pro $20/мес за 1 млн событий | App-first модель (платформа, версия, устройство) |
| Plausible | Privacy без cookie-баннера, дневная соль, ClickHouse | Только web, без батча, без ключа, без retention по пользователям; от $9/мес | Не конкурент для приложений; берём принцип «IP не хранится» |
| OpenPanel | «Open-source Mixpanel»: воронки, retention, когорты, профили; хранит текущую и прошлую соль | AGPL; Redis/BullMQ + опционально Kafka; от $2.5/мес за 5 тыс. событий | Меньше компонентов, детерминированные метрики на uniqExact |
| Rybbit | Воронки, retention, профили, replay; нормализует UA и бакетирует IP дата-центров | По умолчанию `sha256(ip+UA)` без соли — слабая приватность; от $13/мес | Стабильный ID генерирует клиент, IP не хранится вообще |
| Swetrix | События, ошибки, производительность, live; ошибки в отдельной таблице | NestJS + ClickHouse + MySQL + Redis; от $19/мес | Ошибки (крэши) — P1 по тому же паттерну отдельной таблицы |

### 3.1 Feature comparison matrix

| Feature | Us (planned) | AppMetrica | Aptabase | PostHog | Countly Lite |
|---|---|---|---|---|---|
| Приём без SDK (HTTP API для новых ID) | ✅ основной канал | ❌ (Post API только для ID из SDK) | ✅ (без публичных доков) | ✅ | ✅ |
| Web + боты + десктоп в одном дашборде | ✅ | ❌ | ✅ | ✅ | ✅ |
| Портфель всех приложений на одном экране | ✅ (главная) | ❌ (по одному приложению) | ❌ | ❌ (настраиваемые дашборды) | ❌ |
| DAU/WAU/MAU, новые, сессии, длительность | ✅ | ✅ | ✅ (daily users) | ✅ | ✅ |
| Retention day-N / week-N | ✅ D0..D30, W0..W12 | ✅ | ❌ | ✅ | ❌ (платно) |
| Склейка устройств по user_id задним числом | ✅ «первый владелец» | ❌ (по устройствам) | ❌ | ✅ (identify/alias) | ❌ |
| Realtime | ✅ (30 мин, polling 5 с) | ✅ | ✅ (live view) | ✅ | ✅ |
| Профиль устройства/пользователя | ✅ | ✅ (Profiles) | ❌ | ✅ | ❌ (платно) |
| Экран здоровья интеграции (причины отказов) | ✅ | ❌ | ❌ | частично (ingestion warnings) | ❌ |
| Кастомные события + отчёт | P1 (API уже совместим) | ✅ | ✅ | ✅ | ✅ |
| Воронки | P1 | ✅ (с 2021) | ❌ | ✅ | ❌ (платно) |
| Крэши | P1 | ✅ | ✅ (errors, новый модуль) | ✅ | ✅ |
| GDPR-удаление по API | ✅ | ✅ (Management API) | ❌ | ✅ | ✅ |
| Хранение сырых данных | 25 мес + агрегаты бессрочно | по тарифу | не найдено | 1 год (free) | self-hosted |
| Стоимость при 100k DAU | ≈ 266 €/мес (свой сервер) | Free (системные события не лимитированы) | $450/мес за 50 млн событий | платно сверх 1 млн событий | Flex от $175/мес |

## 4. Visual language inferred from references

### 4.1 Patterns to adopt

- **KPI-карточки + большой график** (AppMetrica, выбранный мокап Ref 1 §2.2): верх страницы приложения — 4 карточки «Активные / Новые / Сессии / Ср. длительность» с дельтой к предыдущему периоду и спарклайном, под ними один большой график с переключателем метрики.
- **Период по умолчанию «последние 7 дней», детализация час/день/неделя/месяц, сутки в часовом поясе приложения** (AppMetrica): таймзона — настройка приложения, отображается рядом с выбором периода.
- **Фильтры: OR внутри измерения, AND между измерениями** (сегменты AppMetrica): упрощённая панель фильтров `platform`, `app_version`, `os_name`, `country` с мультиселектом; состояние в URL (nuqs).
- **Сравнение с предыдущим периодом** (AppMetrica): тумблер «Сравнить», пунктирная линия на графике и Δ % на карточках.
- **«Скопировать запрос в API»** (AppMetrica Reporting API): кнопка на каждом отчёте копирует curl к Read API с текущими параметрами.
- **Переключатель debug/release** (Aptabase `isDebug`): переключатель `production | development` в шапке, по умолчанию production.
- **Таблицы топов под графиком** (Umami, Aptabase): разбивки по измерениям — вкладки с таблицами «значение / количество / доля».
- **Retention-heatmap как таблица** (AppMetrica, Amplitude): HTML-таблица с цветовой шкалой — копируемая, доступная с клавиатуры.
- **Онбординг «ключ + сниппет + ждём первое событие»** (Aptabase, PostHog onboarding): экран «Интеграция» с переключением статуса в реальном времени.

### 4.2 Anti-patterns to avoid

- **Перегруженный интерфейс** (AppMetrica по ответу владельца «Сложность / лишнее», PostHog): никаких настраиваемых workspace-ов и 20 видов отчётов; фиксированный набор экранов S-01..S-14.
- **Тихие потери данных** (Mixpanel `/track` отвечает `1` при неверном токене, молчаливая обрезка строк): каждое отклонение — код, поле, сообщение в ответе и запись в `rejected_events`.
- **Жёсткая остановка приёма при превышении квоты** (AppMetrica): только мягкий троттлинг `429` + `Retry-After`, лимиты на приложение и на установку, но не по IP.
- **Ключ и данные в query string** (AppMetrica Post API): ключ только в заголовке `X-App-Key` или в теле.
- **Противоречивые лимиты в разных местах доков** (Mixpanel 10 МБ vs 2 МБ, AppMetrica 7 vs 14 дней): лимиты в документации генерируются из одного источника — `contracts/openapi/ingest.yaml` и env-констант.
- **Семплирование отчётов** (AppMetrica `sampled`, `sample_share`): все метрики точные (`uniqExact`), без семплирования.
- **Многошаговые мастера** для простых действий: создание приложения — одна форма из трёх полей.

## 5. Open questions from reference review

- [x] Q1: Брать ли privacy-хэш IP+UA вместо `install_id`, как Aptabase/Plausible? — **Закрыт**: нет, ломает retention; выбран `install_id` + `user_id` с ретро-склейкой.
- [x] Q2: Нужна ли очередь (Kafka/Redis) в ingestion, как у PostHog/OpenPanel? — **Закрыт**: нет, асинхронная вставка ClickHouse с ожиданием подтверждения достаточна для 500 rps; пересмотр при устойчивых > 3000 rps.
- [ ] Q3: Совпадёт ли определение «новый пользователь» AppMetrica с нашим для переустановок на Android (AppMetrica может переживать переустановку через свой device ID)? — проверяется на 14-дневной сверке в unit=installs; при расхождении > 5 % — правка рецепта хранения `install_id` (Auto Backup). Срок: неделя 7 (сверка), владелец.
- [ ] Q4: Нужна ли кнопка «скопировать как curl» на каждом отчёте или достаточно ссылки на раздел Read API в доках? — решается при реализации S-05 (тикет UI аудитории); default — кнопка есть.

## 6. Sign-off

- [x] Юзер подтвердил что список референсов полный (ответ на вопрос «Референсы»: AppMetrica, open-source аналоги, Mixpanel / Amplitude)
- [x] Референсы собраны (сайты, документация, исходники на GitHub), выжимка зафиксирована здесь
- [x] Competitive analysis matrix заполнен
- [x] Cross-links на документы установлены:
  - `01-prd.md` §6 Competitive landscape → `./00-references.md#3-competitive-analysis-derived`
  - `14-ux-ui-spec.md` → `./00-references.md#4-visual-language-inferred-from-references`

## 7. Ссылки

- Discovery PRD: [01-prd.md](./01-prd.md)
- UX spec: [14-ux-ui-spec.md](./14-ux-ui-spec.md)
- API-контракты (форма payload, коды ошибок): [06-api-contracts.md](./06-api-contracts.md)
- Саммари: [00-summary.md](./00-summary.md)
