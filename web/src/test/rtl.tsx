import {
  render as pureRender,
  renderHook as pureRenderHook,
  type RenderHookOptions,
  type RenderOptions,
} from "@testing-library/react/pure";
import type { ComponentType, ReactElement, ReactNode } from "react";
import { MessagesProvider } from "@/lib/i18n/client";
import type { Locale } from "@/lib/i18n/locale";
import { getMessages } from "@/messages";

// Vitest подменяет «@testing-library/react» этим модулем (алиас в vitest.config.ts): render и renderHook
// оборачивают компонент в русский словарь — язык, на котором написаны проверки тестов. Английский —
// renderWithLocale(ui, "en"); собственный wrapper теста оказывается внутри словаря.
export * from "@testing-library/react/pure";

function withLocale(locale: Locale, inner?: ComponentType<{ children: ReactNode }>) {
  const Inner = inner;
  return function I18nWrapper({ children }: { children: ReactNode }) {
    return (
      <MessagesProvider locale={locale} messages={getMessages(locale)}>
        {Inner ? <Inner>{children}</Inner> : children}
      </MessagesProvider>
    );
  };
}

export function render(ui: ReactElement, options: RenderOptions = {}) {
  return pureRender(ui, { ...options, wrapper: withLocale("ru", options.wrapper) });
}

export function renderHook<Result, Props>(
  hook: (props: Props) => Result,
  options: RenderHookOptions<Props> = {},
) {
  return pureRenderHook(hook, { ...options, wrapper: withLocale("ru", options.wrapper) });
}

export function renderWithLocale(ui: ReactElement, locale: Locale, options: RenderOptions = {}) {
  return pureRender(ui, { ...options, wrapper: withLocale(locale, options.wrapper) });
}
