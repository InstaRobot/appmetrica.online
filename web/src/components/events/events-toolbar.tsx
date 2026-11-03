"use client";

import { Copy } from "lucide-react";
import { EnvironmentSwitch } from "@/components/reports/environment-switch";
import { FilterBar } from "@/components/reports/filter-bar";
import { PeriodPicker } from "@/components/reports/period-picker";
import { PlatformSwitcher } from "@/components/reports/platform-switcher";
import { TOUCH_BUTTON } from "@/components/reports/responsive";
import { UnitBadge } from "@/components/reports/unit-badge";
import { Button } from "@/components/ui/button";
import { useFilterOptions } from "@/hooks/use-filter-options";
import { useObservedPlatforms } from "@/hooks/use-observed-platforms";
import type { App } from "@/lib/api/gen/model";
import { useMessages } from "@/lib/i18n/client";
import {
  type EventsSearchPatch,
  type EventsState,
  filtersSearch,
  periodSearch,
  REPORT_FILTER_DIMENSIONS,
  reportParams,
} from "@/lib/url-state";
import { cn } from "@/lib/utils";

type EventsToolbarProps = {
  app: App;
  state: EventsState;
  update: (patch: EventsSearchPatch) => unknown;
  /** Ошибка периода (range_too_large) под выбором периода. */
  rangeError: string | undefined;
  /** «Скопировать запрос»; без обработчика кнопки нет. */
  onCopy?: () => void;
  /** Элементы первой строки между единицей и кнопкой запроса (детализация графика). */
  children?: React.ReactNode;
};

// Панель отчёта вкладки «События» и страницы события, как на S-05: период и запрос — первая строка,
// платформа, фильтры и окружение — вторая.
export function EventsToolbar({
  app,
  state,
  update,
  rangeError,
  onCopy,
  children,
}: EventsToolbarProps) {
  const messages = useMessages();
  const params = reportParams(state);
  const observedPlatforms = useObservedPlatforms(app.id, params);
  const filterOptions = useFilterOptions(app.id, params, REPORT_FILTER_DIMENSIONS);

  return (
    <div className="flex flex-col gap-3">
      <div className="flex flex-wrap items-start gap-3">
        <PeriodPicker
          value={state.period}
          onChange={(period) => void update(periodSearch(period))}
          timezone={app.timezone}
          error={rangeError}
        />
        {children}
        <UnitBadge unit={state.unit} />
        {onCopy ? (
          <Button
            variant="outline"
            size="sm"
            className={cn(TOUCH_BUTTON, "md:ml-auto")}
            onClick={onCopy}
          >
            <Copy aria-hidden="true" />
            {messages.reports.audience.copyRequest}
          </Button>
        ) : null}
      </div>
      <div className="flex flex-wrap items-center gap-3">
        <PlatformSwitcher
          appPlatforms={app.platforms}
          observed={observedPlatforms}
          value={state.filters.platform?.[0] ?? null}
          onChange={(platform) => void update({ platform: platform ? [platform] : null })}
        />
        <FilterBar
          dims={REPORT_FILTER_DIMENSIONS}
          value={state.filters}
          onChange={(filters) => void update(filtersSearch(filters))}
          options={filterOptions.options}
          onOpenChange={filterOptions.onOpenChange}
          loading={filterOptions.loading}
        />
        <div className="w-full md:ml-auto md:w-auto">
          <EnvironmentSwitch value={state.env} onChange={(env) => void update({ env })} />
        </div>
      </div>
    </div>
  );
}
