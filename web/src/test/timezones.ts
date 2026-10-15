import { vi } from "vitest";

// Короткий список зон вместо полного IANA (≈ 420) для тестов, которые открывают TimezoneSelect. cmdk при
// размонтировании каждого элемента ищет выбранный через querySelector по всему списку: в jsdom фильтр
// «419 зон → 1» квадратичный и занимает ~0,8 с даже локально, а на раннере CI тест выходит за 5 с. Соседние
// зоны (Asia/Tomsk, America/North_Dakota/New_Salem) оставлены, чтобы поиску было что отсеивать. UTC в списке
// нет: его добавляет сам TimezoneSelect.
export const TEST_TIMEZONES = [
  "Africa/Cairo",
  "America/New_York",
  "America/North_Dakota/New_Salem",
  "America/Sao_Paulo",
  "Asia/Kolkata",
  "Asia/Tokyo",
  "Asia/Tomsk",
  "Europe/Berlin",
  "Europe/Moscow",
  "Pacific/Auckland",
];

// mockTimezones вызывается в beforeEach: restoreMocks снимает шпион после каждого теста, а TimezoneSelect
// кэширует список при первом открытии в файле.
export function mockTimezones() {
  vi.spyOn(Intl, "supportedValuesOf").mockReturnValue(TEST_TIMEZONES);
}
