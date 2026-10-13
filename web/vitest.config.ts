import react from "@vitejs/plugin-react";
import { defineConfig } from "vitest/config";

export default defineConfig({
  plugins: [react()],
  resolve: { tsconfigPaths: true },
  test: {
    environment: "jsdom",
    globals: true,
    setupFiles: ["./src/test/setup.ts"],
    include: ["src/**/*.test.{ts,tsx}"],
    restoreMocks: true,
    coverage: {
      provider: "v8",
      reporter: ["text", "lcov"],
      include: ["src/lib/**", "src/components/reports/**", "src/components/charts/**"],
      exclude: ["src/lib/api/gen/**", "**/*.test.{ts,tsx}"],
      thresholds: { lines: 70, statements: 70, functions: 70, branches: 60 },
    },
  },
});
