// Заглушка для красного теста T-036; реализация — в следующем коммите.
export function ErrorState(_props: {
  error: { code: string; message: string; request_id?: string; status?: number } | Error;
  onRetry?: () => void;
  correlationId?: string;
  variant?: "inline" | "full-page";
}) {
  return null;
}
