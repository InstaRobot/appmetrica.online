type PageHeaderProps = {
  title: string;
  /** Иконка платформы и бейджи справа от заголовка (S-05: PlatformIcon + UnitBadge). */
  adornment?: React.ReactNode;
  description?: string;
  actions?: React.ReactNode;
};

// Заголовок страницы (14-ux-ui-spec.md §16.1): единственный <h1> страницы и правые действия.
export function PageHeader({ title, adornment, description, actions }: PageHeaderProps) {
  return (
    <header className="mb-6 flex items-start justify-between gap-4">
      <div className="flex min-w-0 flex-col gap-1">
        <div className="flex items-center gap-2">
          <h1 className="truncate text-heading-1">{title}</h1>
          {adornment}
        </div>
        {description ? <p className="text-body-md text-muted-foreground">{description}</p> : null}
      </div>
      {actions ? <div className="flex shrink-0 items-center gap-3">{actions}</div> : null}
    </header>
  );
}
