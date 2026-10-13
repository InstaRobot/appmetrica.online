// Русское склонение по числу: forms = [одно, несколько, много] — «событие», «события», «событий».
// Дробные числа (категория CLDR "other") согласуются как «несколько»: «1,5 события».
type Forms = readonly [one: string, few: string, many: string];

const rules = new Intl.PluralRules("ru-RU");

export function plural(n: number, forms: Forms): string {
  switch (rules.select(Math.abs(n))) {
    case "one":
      return forms[0];
    case "few":
    case "other":
      return forms[1];
    default:
      return forms[2];
  }
}
