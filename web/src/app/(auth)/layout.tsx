// AuthShell (14-ux-ui-spec.md §5): центрированная карточка 400 px на --bg-page для S-01 и S-02.
// Заголовок h1 страницы имеет id="auth-title" — на него ссылается <main>.
export default function AuthLayout({ children }: { children: React.ReactNode }) {
  return (
    <main
      aria-labelledby="auth-title"
      className="flex min-h-dvh items-center justify-center bg-page px-4 py-12"
    >
      <div className="flex w-full max-w-[400px] flex-col gap-4 rounded-xl border border-border bg-card p-8 shadow-md">
        <p className="text-heading-3">
          appmetrica<span className="text-fg-subtle">.online</span>
        </p>
        {children}
      </div>
    </main>
  );
}
