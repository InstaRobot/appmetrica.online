// Заглушка для красного теста T-036; реализация — в следующем коммите.
export function EmptyState(_props: {
  icon: React.ComponentType<{ className?: string; "aria-hidden"?: boolean }>;
  title: string;
  description?: React.ReactNode;
  action?: { label: string; href?: string; onClick?: () => void };
}) {
  return null;
}
