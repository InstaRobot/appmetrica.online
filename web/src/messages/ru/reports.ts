import type { reports as en } from "../en/reports";

export const reports: typeof en = {
  environment: "Окружение",
  unitBadge: {
    label: "Установки",
    hint: "Сервисный режим: считаются установки, а не люди. Уберите ?unit=installs из адреса, чтобы вернуться.",
  },
};
