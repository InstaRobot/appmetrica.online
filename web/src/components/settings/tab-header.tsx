// Заголовок вкладки S-12: название раздела слева, действие («Добавить passkey», «Создать ключ») справа.
export function TabHeader({
  id,
  title,
  action,
}: {
  id: string;
  title: string;
  action?: React.ReactNode;
}) {
  return (
    <div className="flex items-center justify-between gap-4">
      <h2 id={id} className="text-heading-3">
        {title}
      </h2>
      {action}
    </div>
  );
}
