// Параметры синтетической модели каждого демо-приложения: масштаб DAU, сезонность, retention и
// распределения значений девяти измерений breakdown. Веса относительные, нормируются в model.ts.
import type { BreakdownDimension } from "@/lib/api/gen/model";
import { DEMO_APP_IDS } from "./apps";
import type { Weighted } from "./random";

export type Weights = Weighted<string>;
export type ParentDim = "os_name" | "country";
// Измерение задано либо общими весами, либо весами внутри значения родителя (город внутри страны,
// версия ОС внутри ОС). Ключ "*" — значения для родителя без собственного списка.
export type DimSpec =
  | { readonly weights: Weights }
  | { readonly parent: ParentDim; readonly children: Readonly<Record<string, Weights>> };

export type AppModel = {
  baseDau: number;
  devDau: number;
  growthPerYear: number;
  // Уникальные за n дней ≈ средний DAU × n^stickiness.
  stickiness: number;
  newShare: readonly [number, number];
  sessionsPerUser: number;
  avgDurationSec: number;
  installsExtra: number;
  weekly: readonly number[];
  hourly: readonly number[];
  retention: { d1: number; d30: number; w1: number; w12: number };
  rejectRate: number;
  rejections: Weights;
  dims: Readonly<Record<BreakdownDimension, DimSpec>>;
};

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

function iosDims(appVersions: Weights): AppModel["dims"] {
  return {
    platform: { weights: [["ios", 1]] },
    os_name: { weights: IOS_OS },
    os_version: { parent: "os_name", children: IOS_OS_VERSIONS },
    app_version: { weights: appVersions },
    device_model: { parent: "os_name", children: IOS_DEVICES },
    device_type: { parent: "os_name", children: IOS_DEVICE_TYPES },
    country: {
      weights: [
        ["RU", 78],
        ["KZ", 6],
        ["BY", 5],
        ["UZ", 3],
        ["AM", 2],
        ["GE", 2],
        ["DE", 2],
        ["RS", 1],
        ["TR", 1],
      ],
    },
    city: { parent: "country", children: CITIES },
    locale: { parent: "country", children: LOCALES },
  };
}

const NOTES_IOS: AppModel = {
  baseDau: 12_000,
  devDau: 34,
  growthPerYear: 0.12,
  stickiness: 0.6,
  newShare: [0.08, 0.12],
  sessionsPerUser: 2.35,
  avgDurationSec: 262,
  installsExtra: 0.06,
  weekly: WEEKLY_MOBILE,
  hourly: HOURLY_MOBILE,
  retention: { d1: 0.36, d30: 0.07, w1: 0.42, w12: 0.12 },
  rejectRate: 0.0022,
  rejections: [
    ["duration_required", 62],
    ["event_too_old", 28],
    ["invalid_field", 10],
  ],
  dims: iosDims([
    ["3.4.0", 61],
    ["3.3.2", 23],
    ["3.3.1", 8],
    ["3.2.0", 5],
    ["3.1.4", 3],
  ]),
};

const NOTES_ANDROID: AppModel = {
  baseDau: 8_900,
  devDau: 26,
  growthPerYear: 0.15,
  stickiness: 0.6,
  newShare: [0.08, 0.12],
  sessionsPerUser: 2.3,
  avgDurationSec: 236,
  installsExtra: 0.05,
  weekly: WEEKLY_MOBILE,
  hourly: HOURLY_MOBILE,
  retention: { d1: 0.33, d30: 0.062, w1: 0.39, w12: 0.1 },
  rejectRate: 0.003,
  rejections: [
    ["event_too_old", 55],
    ["install_rate_limited", 30],
    ["missing_field", 15],
  ],
  dims: {
    platform: { weights: [["android", 1]] },
    os_name: { weights: [["Android", 1]] },
    os_version: {
      parent: "os_name",
      children: {
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
    },
    app_version: {
      weights: [
        ["3.4.0", 52],
        ["3.3.2", 27],
        ["3.3.1", 10],
        ["3.2.0", 7],
        ["3.1.4", 4],
      ],
    },
    device_model: {
      parent: "os_name",
      children: {
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
    },
    device_type: {
      parent: "os_name",
      children: {
        Android: [
          ["phone", 95],
          ["tablet", 5],
        ],
      },
    },
    country: {
      weights: [
        ["RU", 74],
        ["KZ", 8],
        ["BY", 6],
        ["UZ", 6],
        ["AM", 2],
        ["GE", 1],
        ["TR", 1],
        ["DE", 1],
        ["RS", 1],
      ],
    },
    city: { parent: "country", children: CITIES },
    locale: { parent: "country", children: LOCALES },
  },
};

const DESKTOP: Weights = [["desktop", 1]];

const SITE: AppModel = {
  baseDau: 3_100,
  devDau: 14,
  growthPerYear: 0.08,
  stickiness: 0.68,
  newShare: [0.09, 0.12],
  sessionsPerUser: 2.15,
  avgDurationSec: 198,
  installsExtra: 0.07,
  weekly: WEEKLY_WEB,
  hourly: HOURLY_WEB,
  retention: { d1: 0.2, d30: 0.03, w1: 0.24, w12: 0.05 },
  rejectRate: 0.004,
  rejections: [
    ["user_id_looks_like_pii", 58],
    ["invalid_field", 42],
  ],
  dims: {
    platform: { weights: [["web", 1]] },
    os_name: {
      weights: [
        ["Windows", 38],
        ["Android", 27],
        ["iOS", 17],
        ["macOS", 12],
        ["Linux", 4],
        ["", 2],
      ],
    },
    os_version: {
      parent: "os_name",
      children: {
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
    },
    app_version: {
      weights: [
        ["2026.10.2", 64],
        ["2026.10.1", 24],
        ["2026.09.4", 9],
        ["2026.09.3", 3],
      ],
    },
    device_model: {
      parent: "os_name",
      children: {
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
    },
    device_type: {
      parent: "os_name",
      children: {
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
    },
    country: {
      weights: [
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
    city: { parent: "country", children: CITIES },
    locale: { parent: "country", children: LOCALES },
  },
};

const BOT: AppModel = {
  baseDau: 1_000,
  devDau: 12,
  growthPerYear: 0.2,
  stickiness: 0.55,
  newShare: [0.08, 0.12],
  sessionsPerUser: 2.5,
  avgDurationSec: 305,
  installsExtra: 0.04,
  weekly: WEEKLY_BOT,
  hourly: HOURLY_BOT,
  retention: { d1: 0.31, d30: 0.055, w1: 0.36, w12: 0.09 },
  rejectRate: 0.0035,
  rejections: [
    ["duration_required", 70],
    ["unknown_event_name", 30],
  ],
  dims: {
    platform: { weights: [["bot", 1]] },
    os_name: {
      weights: [
        ["Android", 52],
        ["iOS", 31],
        ["Windows", 10],
        ["macOS", 5],
        ["Linux", 2],
      ],
    },
    os_version: { parent: "os_name", children: { "*": UNKNOWN } },
    app_version: {
      weights: [
        ["1.8.0", 68],
        ["1.7.3", 24],
        ["1.7.0", 8],
      ],
    },
    device_model: { parent: "os_name", children: { "*": UNKNOWN } },
    device_type: {
      parent: "os_name",
      children: { Windows: DESKTOP, macOS: DESKTOP, Linux: DESKTOP, "*": [["phone", 1]] },
    },
    country: {
      weights: [
        ["RU", 88],
        ["KZ", 5],
        ["BY", 4],
        ["UZ", 2],
        ["AM", 1],
      ],
    },
    city: { parent: "country", children: CITIES },
    locale: { parent: "country", children: BOT_LOCALES },
  },
};

const OLD_GAME: AppModel = {
  baseDau: 2_600,
  devDau: 8,
  growthPerYear: -0.45,
  stickiness: 0.6,
  newShare: [0.06, 0.09],
  sessionsPerUser: 2.6,
  avgDurationSec: 340,
  installsExtra: 0.04,
  weekly: WEEKLY_MOBILE,
  hourly: HOURLY_MOBILE,
  retention: { d1: 0.34, d30: 0.065, w1: 0.38, w12: 0.09 },
  rejectRate: 0,
  rejections: [["app_archived", 1]],
  dims: iosDims([
    ["1.12.0", 71],
    ["1.11.3", 22],
    ["1.10.0", 7],
  ]),
};

export const APP_MODELS: Readonly<Record<string, AppModel>> = {
  [DEMO_APP_IDS.notesIos]: NOTES_IOS,
  [DEMO_APP_IDS.notesAndroid]: NOTES_ANDROID,
  [DEMO_APP_IDS.site]: SITE,
  [DEMO_APP_IDS.bot]: BOT,
  [DEMO_APP_IDS.oldGame]: OLD_GAME,
};
