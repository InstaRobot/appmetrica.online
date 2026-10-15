// Параметры синтетической модели демо-приложений. Приложение — набор сегментов-платформ: у каждого
// свой масштаб DAU, тренд, дата запуска, сезонность, retention, релизы и распределения значений
// измерений breakdown. Сюжеты (календарь, выход в страну, кампании, баги релизов, всплески отказов,
// сбои приёма) — данные, model.ts и live.ts их применяют. Веса относительные, нормируются в model.ts.
import type { BreakdownDimension, Platform } from "@/lib/api/gen/model";
import { DEMO_APP_IDS } from "./apps";
import type { Weighted } from "./random";

export type Weights = Weighted<string>;
export type ParentDim = "os_name" | "country";
// Веса дочернего измерения внутри значения родителя (город внутри страны, версия ОС внутри ОС).
// Ключ "*" — значения для родителя без собственного списка.
export type ChildWeights = Readonly<Record<string, Weights>>;

// Releases — версии сегмента от старой к новой с днём выхода; первая есть у всех до выхода второй.
// Обновление на вышедшую версию — логистика: половина аудитории обновляется за halfDays дней, после
// выхода следующей версии на прежнюю больше не обновляются. laggards — доля аудитории, которая отстаёт
// от остальных на lagDays дней (старые версии не исчезают сразу).
export type Releases = {
  readonly versions: readonly (readonly [version: string, released: string])[];
  readonly halfDays: number;
  readonly laggards: number;
  readonly lagDays: number;
};

export type SegmentDims = {
  readonly os_name: Weights;
  readonly app_version: Releases;
  readonly country: Weights;
  readonly os_version: ChildWeights;
  readonly device_model: ChildWeights;
  readonly device_type: ChildWeights;
  readonly city: ChildWeights;
  readonly locale: ChildWeights;
};

export const DIM_PARENT: Readonly<Partial<Record<BreakdownDimension, ParentDim>>> = {
  os_version: "os_name",
  device_model: "os_name",
  device_type: "os_name",
  city: "country",
  locale: "country",
};

// SegmentModel — одна платформа приложения. События платформы начинаются в launch; rampDays > 0 —
// аудитория растёт с нуля (1 − e^(−дни/rampDays)), иначе сразу в масштабе тренда.
export type SegmentModel = {
  platform: Platform;
  launch: string;
  rampDays: number;
  baseDau: number;
  devDau: number;
  growthPerYear: number;
  newShare: readonly [number, number];
  sessionsPerUser: number;
  avgDurationSec: number;
  installsExtra: number;
  weekly: readonly number[];
  hourly: readonly number[];
  retention: { d1: number; d30: number; w1: number; w12: number };
  dims: SegmentDims;
};

// Season — множитель активности в окне from…to (MM-DD, каждый год; окно может переходить через Новый
// год); smooth — множитель нарастает к середине окна и спадает к краям.
export type Season = { from: string; to: string; factor: number; smooth?: boolean };

// Market — выход в страну: вес страны в сегментах растёт с нуля за rampDays дней с launch, а
// аудитория — вместе с ним (baseDau — уже с этой страной).
export type Market = { country: string; launch: string; rampDays: number };

// Campaign — маркетинговая кампания: days дней с from сегмент platform получает newBoost × обычных
// новых из страны country; retention — множитель удержания когорт сегмента в эти дни.
export type Campaign = {
  platform: Platform;
  country: string;
  from: string;
  days: number;
  newBoost: number;
  retention: number;
};

// ReleaseBug — дефект версии с from до until (не включая): сессии короче в durationFactor раз.
export type ReleaseBug = { version: string; from: string; until: string; durationFactor: number };

// RejectBurst — всплеск отказов code с from до until (не включая): rate от принятых событий; с version —
// от событий этой версии, которые шлют context.app_version числом.
export type RejectBurst = {
  code: string;
  from: string;
  until: string;
  rate: number;
  version?: string;
};

export type AppModel = {
  segments: readonly SegmentModel[];
  // Доля людей, активных сразу на нескольких платформах: итог по людям меньше суммы сегментов.
  crossPlatformShare: number;
  // Уникальные за n дней ≈ средний DAU × n^stickiness.
  stickiness: number;
  rejectRate: number;
  rejections: Weights;
  calendar: readonly Season[];
  markets: readonly Market[];
  campaigns: readonly Campaign[];
  bugs: readonly ReleaseBug[];
  rejectBursts: readonly RejectBurst[];
};

// IngestOutage — недоступность приёма с from до to (UTC) у всех приложений: доходит residual событий,
// resend потерянных досылается офлайн-очередью SDK за resendHours часов после восстановления.
export type IngestOutage = {
  from: string;
  to: string;
  residual: number;
  resend: number;
  resendHours: number;
};

export const INGEST_OUTAGES: readonly IngestOutage[] = [
  // 10:00–14:00 MSK.
  {
    from: "2026-09-23T07:00:00Z",
    to: "2026-09-23T11:00:00Z",
    residual: 0.03,
    resend: 0.7,
    resendHours: 2,
  },
];

// Российский календарь: новогодние каникулы, майские праздники, летние отпуска.
const NEW_YEAR = { from: "12-31", to: "01-08" };
const MAY_HOLIDAYS = { from: "05-01", to: "05-11" };
const SUMMER = { from: "06-10", to: "08-25", smooth: true };

const WEEKLY_MOBILE = [1.0, 1.02, 1.03, 1.02, 0.98, 0.88, 0.85];
const WEEKLY_WEB = [1.05, 1.07, 1.06, 1.04, 0.97, 0.74, 0.72];
const WEEKLY_BOT = [1.02, 1.03, 1.03, 1.02, 1.0, 0.92, 0.9];

const HOURLY_MOBILE = [
  1.2, 0.7, 0.4, 0.3, 0.3, 0.5, 1.4, 3.0, 4.6, 5.0, 5.0, 5.2, 5.6, 5.5, 5.2, 5.1, 5.2, 5.5, 6.0,
  6.6, 7.0, 6.8, 5.2, 2.8,
];
const HOURLY_WEB = [
  0.6, 0.3, 0.2, 0.2, 0.2, 0.4, 1.2, 2.8, 4.8, 6.4, 7.2, 7.4, 7.0, 7.1, 7.2, 7.0, 6.6, 5.8, 4.8,
  4.4, 4.2, 3.6, 2.4, 1.2,
];
const HOURLY_BOT = [
  1.6, 0.9, 0.5, 0.4, 0.4, 0.6, 1.5, 3.2, 4.8, 5.4, 5.4, 5.3, 5.4, 5.3, 5.1, 5.0, 5.1, 5.3, 5.6,
  6.0, 6.2, 5.9, 4.6, 3.0,
];

const UNKNOWN: Weights = [["", 1]];

const CITIES: Readonly<Record<string, Weights>> = {
  RU: [
    ["Moscow", 30],
    ["Saint Petersburg", 14],
    ["Novosibirsk", 5],
    ["Yekaterinburg", 5],
    ["Kazan", 4],
    ["Nizhny Novgorod", 3],
    ["Krasnodar", 3],
    ["Samara", 3],
    ["Rostov-on-Don", 3],
    ["Chelyabinsk", 2],
    ["Perm", 2],
    ["", 12],
  ],
  KZ: [
    ["Almaty", 50],
    ["Astana", 35],
    ["Shymkent", 5],
    ["", 10],
  ],
  BY: [
    ["Minsk", 80],
    ["Gomel", 8],
    ["", 12],
  ],
  UZ: [
    ["Tashkent", 85],
    ["Samarkand", 5],
    ["", 10],
  ],
  AM: [
    ["Yerevan", 90],
    ["", 10],
  ],
  GE: [
    ["Tbilisi", 85],
    ["Batumi", 10],
    ["", 5],
  ],
  DE: [
    ["Berlin", 40],
    ["Munich", 15],
    ["Frankfurt am Main", 10],
    ["", 35],
  ],
  RS: [
    ["Belgrade", 80],
    ["Novi Sad", 15],
    ["", 5],
  ],
  TR: [
    ["Istanbul", 60],
    ["Antalya", 30],
    ["", 10],
  ],
  US: [
    ["New York", 30],
    ["San Francisco", 20],
    ["", 50],
  ],
  NL: [
    ["Amsterdam", 70],
    ["", 30],
  ],
  "*": UNKNOWN,
};

const LOCALES: Readonly<Record<string, Weights>> = {
  RU: [
    ["ru-RU", 92],
    ["en-US", 5],
    ["uk-UA", 2],
    ["en-GB", 1],
  ],
  KZ: [
    ["ru-KZ", 60],
    ["kk-KZ", 30],
    ["en-US", 10],
  ],
  BY: [
    ["ru-BY", 70],
    ["be-BY", 20],
    ["en-US", 10],
  ],
  UZ: [
    ["ru-UZ", 55],
    ["uz-UZ", 35],
    ["en-US", 10],
  ],
  AM: [
    ["ru-AM", 50],
    ["hy-AM", 35],
    ["en-US", 15],
  ],
  GE: [
    ["ru-GE", 45],
    ["ka-GE", 35],
    ["en-US", 20],
  ],
  DE: [
    ["de-DE", 45],
    ["ru-DE", 40],
    ["en-US", 15],
  ],
  RS: [
    ["ru-RS", 45],
    ["sr-RS", 40],
    ["en-US", 15],
  ],
  TR: [
    ["ru-TR", 55],
    ["tr-TR", 30],
    ["en-US", 15],
  ],
  US: [
    ["en-US", 80],
    ["ru-US", 20],
  ],
  NL: [
    ["en-NL", 40],
    ["ru-NL", 35],
    ["nl-NL", 25],
  ],
  "*": [
    ["ru-RU", 70],
    ["en-US", 30],
  ],
};

// Telegram передаёт language_code без региона.
const BOT_LOCALES: Readonly<Record<string, Weights>> = {
  RU: [
    ["ru", 93],
    ["en", 4],
    ["uk", 3],
  ],
  KZ: [
    ["ru", 70],
    ["kk", 20],
    ["en", 10],
  ],
  BY: [
    ["ru", 85],
    ["be", 5],
    ["en", 10],
  ],
  UZ: [
    ["ru", 60],
    ["uz", 30],
    ["en", 10],
  ],
  "*": [
    ["ru", 60],
    ["en", 40],
  ],
};

const IOS_OS: Weights = [
  ["iOS", 93],
  ["iPadOS", 7],
];
const IOS_OS_VERSIONS: Readonly<Record<string, Weights>> = {
  iOS: [
    ["26.0.1", 34],
    ["26.0", 14],
    ["18.6.2", 24],
    ["18.6", 8],
    ["18.5", 7],
    ["17.7.2", 8],
    ["16.7.11", 5],
  ],
  iPadOS: [
    ["26.0.1", 45],
    ["18.6.2", 40],
    ["17.7.2", 15],
  ],
};
const IOS_DEVICES: Readonly<Record<string, Weights>> = {
  iOS: [
    ["iPhone17,3", 14],
    ["iPhone17,1", 9],
    ["iPhone17,4", 6],
    ["iPhone16,2", 12],
    ["iPhone16,1", 8],
    ["iPhone15,4", 10],
    ["iPhone15,2", 7],
    ["iPhone14,7", 9],
    ["iPhone14,5", 8],
    ["iPhone14,6", 4],
    ["iPhone13,2", 7],
    ["iPhone12,1", 6],
  ],
  iPadOS: [
    ["iPad14,3", 30],
    ["iPad13,18", 25],
    ["iPad16,3", 20],
    ["iPad12,1", 25],
  ],
};
const IOS_DEVICE_TYPES: Readonly<Record<string, Weights>> = {
  iOS: [["phone", 1]],
  iPadOS: [["tablet", 1]],
};

function iosDims(appVersions: Releases, kz = 6): SegmentDims {
  return {
    os_name: IOS_OS,
    os_version: IOS_OS_VERSIONS,
    app_version: appVersions,
    device_model: IOS_DEVICES,
    device_type: IOS_DEVICE_TYPES,
    country: [
      ["RU", 78],
      ["KZ", kz],
      ["BY", 5],
      ["UZ", 3],
      ["AM", 2],
      ["GE", 2],
      ["DE", 2],
      ["RS", 1],
      ["TR", 1],
    ],
    city: CITIES,
    locale: LOCALES,
  };
}

const ANDROID_DIMS: Omit<SegmentDims, "app_version" | "country"> = {
  os_name: [["Android", 1]],
  os_version: {
    Android: [
      ["16", 18],
      ["15", 33],
      ["14", 21],
      ["13", 13],
      ["12", 8],
      ["11", 5],
      ["10", 2],
    ],
  },
  device_model: {
    Android: [
      ["SM-A556E", 9],
      ["23129RAA4G", 9],
      ["SM-A356E", 7],
      ["SM-S928B", 6],
      ["24069RA21G", 6],
      ["M2101K6G", 6],
      ["SM-S921B", 5],
      ["2312DRA50G", 5],
      ["CPH2581", 5],
      ["RMX3852", 5],
      ["V2324", 4],
      ["TECNO KJ5", 4],
      ["Pixel 8", 3],
      ["Infinix X6853", 3],
      ["SM-X710", 3],
      ["Pixel 9 Pro", 2],
    ],
  },
  device_type: {
    Android: [
      ["phone", 95],
      ["tablet", 5],
    ],
  },
  city: CITIES,
  locale: LOCALES,
};

const DESKTOP: Weights = [["desktop", 1]];

// Браузеры: ОС и устройство из User-Agent, модель телефона — только у части Android.
const WEB_DIMS: Omit<SegmentDims, "app_version" | "country"> = {
  os_name: [
    ["Windows", 38],
    ["Android", 27],
    ["iOS", 17],
    ["macOS", 12],
    ["Linux", 4],
    ["", 2],
  ],
  os_version: {
    Windows: [
      ["10", 58],
      ["11", 42],
    ],
    Android: [
      ["15", 35],
      ["14", 30],
      ["13", 20],
      ["12", 15],
    ],
    iOS: [
      ["26.0.1", 45],
      ["18.6.2", 35],
      ["17.7.2", 20],
    ],
    macOS: [
      ["15.6.1", 50],
      ["26.0", 30],
      ["14.7.6", 20],
    ],
    "*": UNKNOWN,
  },
  device_model: {
    Android: [
      ["SM-A556E", 20],
      ["23129RAA4G", 20],
      ["Pixel 8", 10],
      ["SM-S928B", 10],
      ["", 40],
    ],
    iOS: [
      ["iPhone", 85],
      ["iPad", 15],
    ],
    macOS: [["Macintosh", 1]],
    "*": UNKNOWN,
  },
  device_type: {
    Windows: DESKTOP,
    macOS: DESKTOP,
    Linux: DESKTOP,
    Android: [
      ["phone", 92],
      ["tablet", 8],
    ],
    iOS: [
      ["phone", 85],
      ["tablet", 15],
    ],
    "*": UNKNOWN,
  },
  city: CITIES,
  locale: LOCALES,
};

// Версии «Notes»: 3.4.0 вышла с багом, 3.4.1 — хотфикс через четыре дня.
const NOTES_RELEASES: Releases["versions"] = [
  ["3.0.2", "2025-03-17"],
  ["3.1.0", "2026-02-10"],
  ["3.1.4", "2026-04-21"],
  ["3.2.0", "2026-06-16"],
  ["3.3.0", "2026-08-04"],
  ["3.4.0", "2026-09-08"],
  ["3.4.1", "2026-09-12"],
  ["3.5.0", "2026-10-06"],
];
const NOTES_MOBILE_VERSIONS: Releases = {
  versions: NOTES_RELEASES,
  halfDays: 6,
  laggards: 0.03,
  lagDays: 45,
};

// «Notes»: iOS растёт, Android ровный, веб запущен 2026-08-14 и набирает аудиторию с нуля. Сюжеты:
// выход в Казахстан, кампании в RU на Android и в KZ на iOS, баг релиза 3.4.0.
const NOTES: AppModel = {
  crossPlatformShare: 0.15,
  stickiness: 0.6,
  rejectRate: 0.0025,
  rejections: [
    ["duration_required", 50],
    ["event_too_old", 35],
    ["invalid_field", 15],
  ],
  calendar: [
    { ...NEW_YEAR, factor: 1.15 },
    { ...MAY_HOLIDAYS, factor: 0.9 },
    { ...SUMMER, factor: 0.86 },
  ],
  markets: [{ country: "KZ", launch: "2026-06-01", rampDays: 30 }],
  campaigns: [
    {
      platform: "android",
      country: "RU",
      from: "2026-05-20",
      days: 5,
      newBoost: 0.6,
      retention: 0.65,
    },
    {
      platform: "ios",
      country: "KZ",
      from: "2026-09-15",
      days: 5,
      newBoost: 0.25,
      retention: 0.65,
    },
  ],
  bugs: [{ version: "3.4.0", from: "2026-09-08", until: "2026-09-12", durationFactor: 0.7 }],
  rejectBursts: [
    {
      code: "invalid_field",
      from: "2026-09-08",
      until: "2026-09-12",
      rate: 0.25,
      version: "3.4.0",
    },
  ],
  segments: [
    {
      platform: "ios",
      launch: "2025-03-17",
      rampDays: 0,
      baseDau: 11_500,
      devDau: 30,
      growthPerYear: 0.35,
      newShare: [0.08, 0.12],
      sessionsPerUser: 2.35,
      avgDurationSec: 262,
      installsExtra: 0.06,
      weekly: WEEKLY_MOBILE,
      hourly: HOURLY_MOBILE,
      retention: { d1: 0.36, d30: 0.07, w1: 0.42, w12: 0.12 },
      dims: iosDims(NOTES_MOBILE_VERSIONS, 9),
    },
    {
      platform: "android",
      launch: "2025-04-02",
      rampDays: 0,
      baseDau: 8_400,
      devDau: 22,
      growthPerYear: 0.02,
      newShare: [0.08, 0.12],
      sessionsPerUser: 2.3,
      avgDurationSec: 236,
      installsExtra: 0.05,
      weekly: WEEKLY_MOBILE,
      hourly: HOURLY_MOBILE,
      retention: { d1: 0.33, d30: 0.062, w1: 0.39, w12: 0.1 },
      dims: {
        ...ANDROID_DIMS,
        app_version: NOTES_MOBILE_VERSIONS,
        country: [
          ["RU", 74],
          ["KZ", 9],
          ["BY", 6],
          ["UZ", 6],
          ["AM", 2],
          ["GE", 1],
          ["TR", 1],
          ["DE", 1],
          ["RS", 1],
        ],
      },
    },
    {
      platform: "web",
      launch: "2026-08-14",
      rampDays: 24,
      baseDau: 3_300,
      devDau: 10,
      growthPerYear: 0,
      newShare: [0.09, 0.12],
      sessionsPerUser: 2.1,
      avgDurationSec: 214,
      installsExtra: 0.07,
      weekly: WEEKLY_WEB,
      hourly: HOURLY_WEB,
      retention: { d1: 0.24, d30: 0.035, w1: 0.28, w12: 0.06 },
      dims: {
        ...WEB_DIMS,
        // Веб-версия обновляется при перезагрузке страницы.
        app_version: {
          versions: NOTES_RELEASES.slice(4),
          halfDays: 0.5,
          laggards: 0.04,
          lagDays: 3,
        },
        country: [
          ["RU", 82],
          ["KZ", 5],
          ["BY", 4],
          ["DE", 3],
          ["UZ", 2],
          ["US", 2],
          ["NL", 1],
          ["", 1],
        ],
      },
    },
  ],
};

const SITE: AppModel = {
  crossPlatformShare: 0,
  stickiness: 0.68,
  rejectRate: 0.004,
  rejections: [
    ["user_id_looks_like_pii", 58],
    ["invalid_field", 42],
  ],
  calendar: [
    { ...NEW_YEAR, factor: 0.75 },
    { ...MAY_HOLIDAYS, factor: 0.85 },
    { ...SUMMER, factor: 0.9 },
  ],
  markets: [],
  campaigns: [],
  bugs: [],
  // Форма отправляла timestamp в локальном формате.
  rejectBursts: [{ code: "invalid_field", from: "2026-10-02", until: "2026-10-03", rate: 0.04 }],
  segments: [
    {
      platform: "web",
      launch: "2025-06-10",
      rampDays: 0,
      baseDau: 3_100,
      devDau: 14,
      growthPerYear: 0.08,
      newShare: [0.09, 0.12],
      sessionsPerUser: 2.15,
      avgDurationSec: 198,
      installsExtra: 0.07,
      weekly: WEEKLY_WEB,
      hourly: HOURLY_WEB,
      retention: { d1: 0.2, d30: 0.03, w1: 0.24, w12: 0.05 },
      dims: {
        ...WEB_DIMS,
        app_version: {
          versions: [
            ["2025.06.1", "2025-06-10"],
            ["2025.12.2", "2025-12-15"],
            ["2026.03.1", "2026-03-02"],
            ["2026.06.2", "2026-06-15"],
            ["2026.09.1", "2026-09-01"],
            ["2026.09.2", "2026-09-08"],
            ["2026.09.3", "2026-09-21"],
            ["2026.09.4", "2026-09-28"],
            ["2026.10.1", "2026-10-05"],
            ["2026.10.2", "2026-10-12"],
          ],
          halfDays: 1,
          laggards: 0.12,
          lagDays: 8,
        },
        country: [
          ["RU", 85],
          ["KZ", 4],
          ["BY", 4],
          ["UZ", 2],
          ["DE", 2],
          ["US", 1],
          ["NL", 1],
          ["", 1],
        ],
      },
    },
  ],
};

const BOT: AppModel = {
  crossPlatformShare: 0,
  stickiness: 0.55,
  rejectRate: 0.0035,
  rejections: [
    ["duration_required", 70],
    ["unknown_event_name", 30],
  ],
  calendar: [
    { ...NEW_YEAR, factor: 1.1 },
    { ...SUMMER, factor: 0.95 },
  ],
  markets: [],
  campaigns: [],
  bugs: [],
  rejectBursts: [],
  segments: [
    {
      platform: "bot",
      launch: "2025-11-20",
      rampDays: 0,
      baseDau: 1_000,
      devDau: 12,
      growthPerYear: 0.2,
      newShare: [0.08, 0.12],
      sessionsPerUser: 2.5,
      avgDurationSec: 305,
      installsExtra: 0.04,
      weekly: WEEKLY_BOT,
      hourly: HOURLY_BOT,
      retention: { d1: 0.31, d30: 0.055, w1: 0.36, w12: 0.09 },
      dims: {
        os_name: [
          ["Android", 52],
          ["iOS", 31],
          ["Windows", 10],
          ["macOS", 5],
          ["Linux", 2],
        ],
        os_version: { "*": UNKNOWN },
        app_version: {
          versions: [
            ["1.6.0", "2025-11-20"],
            ["1.7.0", "2026-05-12"],
            ["1.7.3", "2026-07-21"],
            ["1.8.0", "2026-09-29"],
          ],
          halfDays: 6,
          laggards: 0.08,
          lagDays: 60,
        },
        device_model: { "*": UNKNOWN },
        device_type: { Windows: DESKTOP, macOS: DESKTOP, Linux: DESKTOP, "*": [["phone", 1]] },
        country: [
          ["RU", 88],
          ["KZ", 5],
          ["BY", 4],
          ["UZ", 2],
          ["AM", 1],
        ],
        city: CITIES,
        locale: BOT_LOCALES,
      },
    },
  ],
};

const OLD_GAME: AppModel = {
  crossPlatformShare: 0,
  stickiness: 0.6,
  rejectRate: 0,
  rejections: [["app_archived", 1]],
  calendar: [{ ...NEW_YEAR, factor: 1.2 }],
  markets: [],
  campaigns: [],
  bugs: [],
  rejectBursts: [],
  segments: [
    {
      platform: "ios",
      launch: "2024-09-02",
      rampDays: 0,
      baseDau: 2_600,
      devDau: 8,
      growthPerYear: -0.45,
      newShare: [0.06, 0.09],
      sessionsPerUser: 2.6,
      avgDurationSec: 340,
      installsExtra: 0.04,
      weekly: WEEKLY_MOBILE,
      hourly: HOURLY_MOBILE,
      retention: { d1: 0.34, d30: 0.065, w1: 0.38, w12: 0.09 },
      dims: iosDims({
        versions: [
          ["1.9.2", "2024-09-02"],
          ["1.10.0", "2025-02-11"],
          ["1.11.3", "2025-05-27"],
          ["1.12.0", "2025-08-19"],
        ],
        halfDays: 6,
        laggards: 0.1,
        lagDays: 90,
      }),
    },
  ],
};

export const APP_MODELS: Readonly<Record<string, AppModel>> = {
  [DEMO_APP_IDS.notes]: NOTES,
  [DEMO_APP_IDS.site]: SITE,
  [DEMO_APP_IDS.bot]: BOT,
  [DEMO_APP_IDS.oldGame]: OLD_GAME,
};
