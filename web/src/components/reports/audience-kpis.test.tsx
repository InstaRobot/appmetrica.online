import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import type { AudienceSummary } from "@/lib/api/gen/model";
import { renderWithLocale } from "@/test/rtl";
import { AudienceKpis } from "./audience-kpis";

const summary: AudienceSummary = {
  from: "2026-10-06",
  to: "2026-10-12",
  active: 12480,
  new: 7905,
  sessions: 198304,
  avg_duration_ms: 252000,
  median_duration_ms: 125000,
  compare: {
    from: "2026-09-29",
    to: "2026-10-05",
    active: 11988,
    new: 8150,
    sessions: 198304,
    avg_duration_ms: null,
    median_duration_ms: null,
    delta_pct: {
      active: 4.1,
      new: -3,
      sessions: 0,
      avg_duration_ms: null,
      median_duration_ms: null,
    },
  },
};

const sparklines = {
  active: [
    { ts: "2026-10-06T00:00:00+03:00", value: 10 },
    { ts: "2026-10-07T00:00:00+03:00", value: 20 },
  ],
};

describe("AudienceKpis", () => {
  it("renders KPIs with accessible deltas", () => {
    render(
      <AudienceKpis
        summary={summary}
        sparklines={sparklines}
        selected="active"
        onSelect={vi.fn()}
      />,
    );

    const region = screen.getByRole("region", { name: "Ключевые метрики" });
    const cards = within(region)
      .getAllByRole("button")
      .filter((b) => b.hasAttribute("aria-pressed"));
    expect(cards.map((c) => c.querySelector("[data-kpi-label]")?.textContent)).toEqual([
      "Активные",
      "Новые",
      "Сессии",
      "Ср. длительность",
    ]);

    expect(
      within(region).getByRole("img", { name: /^рост на 4,1\s% к предыдущему периоду$/ }),
    ).toBeInTheDocument();
    expect(
      within(region).getByRole("img", { name: /^снижение на 3\s% к предыдущему периоду$/ }),
    ).toBeInTheDocument();
    expect(
      within(region).getByRole("img", { name: "без изменений к предыдущему периоду" }),
    ).toBeInTheDocument();
    expect(within(region).getAllByRole("img")).toHaveLength(3);

    const active = within(region).getByRole("button", { name: /^Активные/ });
    expect(active).toHaveAttribute("aria-pressed", "true");
    expect(active).toHaveTextContent(/12\s480/);
    expect(within(region).getByRole("button", { name: /^Ср\. длительность/ })).toHaveTextContent(
      /4\sмин\s12\sс.*медиана 2\sмин\s5\sс/,
    );
  });

  it("hides deltas without comparison and reports the selected metric", async () => {
    const onSelect = vi.fn();
    const user = userEvent.setup();
    const { compare: _, ...plain } = summary;
    render(<AudienceKpis summary={plain} selected="active" onSelect={onSelect} />);

    expect(screen.queryByRole("img")).not.toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: /^Сессии/ }));
    expect(onSelect).toHaveBeenCalledWith("sessions");
  });

  it("shows skeleton cards while the summary loads", () => {
    render(<AudienceKpis summary={undefined} loading selected="active" onSelect={vi.fn()} />);
    expect(screen.getAllByRole("status", { name: "Загрузка…" })).toHaveLength(4);
  });

  it("names and formats the KPIs in English", () => {
    renderWithLocale(<AudienceKpis summary={summary} selected="active" onSelect={vi.fn()} />, "en");

    const region = screen.getByRole("region", { name: "Key metrics" });
    const cards = within(region)
      .getAllByRole("button")
      .filter((b) => b.hasAttribute("aria-pressed"));
    expect(cards.map((c) => c.querySelector("[data-kpi-label]")?.textContent)).toEqual([
      "Active users",
      "New users",
      "Sessions",
      "Avg. duration",
    ]);
    expect(within(region).getByRole("button", { name: /^Active users/ })).toHaveTextContent(
      "12,480",
    );
    expect(
      within(region).getByRole("img", { name: "up 4.1% vs. the previous period" }),
    ).toBeInTheDocument();
    expect(within(region).getByRole("button", { name: /^Avg\. duration/ })).toHaveTextContent(
      "4m 12smedian 2m 5s",
    );
    expect(within(region).getByRole("button", { name: "What is “Sessions”?" })).toBeInTheDocument();
  });
});
