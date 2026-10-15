type PageHeaderProps = {
  title: string;
  /** Иконка платформы и бейджи справа от заголовка (S-05: PlatformIcon + UnitBadge). */
  adornment?: React.ReactNode;
  description?: string;
  actions?: React.ReactNode;
};

// Заголовок страницы (14-ux-ui-spec.md §16.1): единственный <h1> страницы и правые действия.
// На телефоне бейджи переносятся под заголовок, а действия — строкой ниже; с md — в одну строку.
export function PageHeader({ title, adornment, description, actions }: PageHeaderProps) {
  return (
    <header className="mb-6 flex flex-col gap-3 md:flex-row md:items-start md:justify-between md:gap-4">
      <div className="flex min-w-0 flex-col gap-1">
        <div className="flex flex-wrap items-center gap-2 md:flex-nowrap">
          <h1 className="truncate text-heading-1">{title}</h1>
          {adornment}
        </div>
        {description ? <p className="text-body-md text-muted-foreground">{description}</p> : null}
      </div>
      {actions ? (
        <div className="flex flex-wrap items-center gap-3 md:shrink-0 md:flex-nowrap">
          {actions}
        </div>
      ) : null}
    </header>
  );
}
