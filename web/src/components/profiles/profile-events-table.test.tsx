import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { HttpResponse, http } from "msw";
import { describe, expect, it } from "vitest";
import { errorBody, profileEvent } from "@/test/factories";
import { server } from "@/test/msw";
import { renderWithClient } from "@/test/render";
import { ProfileEventsTable } from "./profile-events-table";

const APP_ID = "0192aaaa-0000-7000-8000-000000000022";
const EVENTS_URL = `*/v1/apps/${APP_ID}/profiles/:personKey/events`;

function renderTable(options?: { locale?: "en" | "ru" }) {
  return renderWithClient(
    <ProfileEventsTable appId={APP_ID} personKey="u:42" timezone="Europe/Moscow" />,
    options,
  );
}

describe("ProfileEventsTable", () => {
  it("appends the next page on «Show more» and hides the button on the last page", async () => {
    const urls: URL[] = [];
    server.use(
      http.get(EVENTS_URL, ({ request }) => {
        const url = new URL(request.url);
        urls.push(url);
        return url.searchParams.get("before") === "cursor-1"
          ? HttpResponse.json({
              events: [profileEvent({ name: "second_page_event" })],
              next_before: null,
            })
          : HttpResponse.json({
              events: [profileEvent({ name: "first_page_event" })],
              next_before: "cursor-1",
            });
      }),
    );
    renderTable({ locale: "en" });

    expect(await screen.findByText("first_page_event")).toBeInTheDocument();
    expect(urls[0]?.searchParams.get("limit")).toBe("50");
    await userEvent.click(screen.getByRole("button", { name: "Show more" }));

    expect(await screen.findByText("second_page_event")).toBeInTheDocument();
    expect(screen.getByText("first_page_event")).toBeInTheDocument();
    expect(urls[1]?.searchParams.get("before")).toBe("cursor-1");
    expect(screen.queryByRole("button", { name: "Show more" })).not.toBeInTheDocument();
  });

  it("shows three properties and reveals the rest on «2 more»", async () => {
    server.use(
      http.get(EVENTS_URL, () =>
        HttpResponse.json({
          events: [
            profileEvent({
              properties: { a: "1", b: "2", c: "3", d: "4", e: "5" },
            }),
          ],
          next_before: null,
        }),
      ),
    );
    renderTable({ locale: "en" });

    expect(await screen.findByText("a: 1")).toBeInTheDocument();
    expect(screen.getByText("b: 2")).toBeInTheDocument();
    expect(screen.getByText("c: 3")).toBeInTheDocument();
    expect(screen.queryByText("d: 4")).not.toBeInTheDocument();
    expect(screen.queryByText("e: 5")).not.toBeInTheDocument();

    await userEvent.click(screen.getByRole("button", { name: "2 more" }));
    expect(screen.getByText("d: 4")).toBeInTheDocument();
    expect(screen.getByText("e: 5")).toBeInTheDocument();
  });

  it("uses the Russian plural for the properties toggle", async () => {
    server.use(
      http.get(EVENTS_URL, () =>
        HttpResponse.json({
          events: [
            profileEvent({
              properties: { a: "1", b: "2", c: "3", d: "4", e: "5", f: "6", g: "7", h: "8" },
            }),
          ],
          next_before: null,
        }),
      ),
    );
    renderTable();
    expect(await screen.findByRole("button", { name: "ещё 5" })).toBeInTheDocument();
  });

  it("renders name, platform and version of the event row", async () => {
    server.use(
      http.get(EVENTS_URL, () =>
        HttpResponse.json({
          events: [profileEvent({ name: "purchase", platform: "android", app_version: "3.0.0" })],
          next_before: null,
        }),
      ),
    );
    renderTable({ locale: "en" });
    const row = (await screen.findByText("purchase")).closest("tr");
    expect(row).not.toBeNull();
    expect(within(row as HTMLElement).getByText("Android")).toBeInTheDocument();
    expect(within(row as HTMLElement).getByText("3.0.0")).toBeInTheDocument();
  });

  it("shows the empty state when the person has no custom events", async () => {
    server.use(http.get(EVENTS_URL, () => HttpResponse.json({ events: [], next_before: null })));
    renderTable({ locale: "en" });
    expect(await screen.findByText("No custom events")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Show more" })).not.toBeInTheDocument();
  });

  it("shows an inline error with retry that reloads the events", async () => {
    let fail = true;
    server.use(
      http.get(EVENTS_URL, () =>
        fail
          ? HttpResponse.json(errorBody("storage_unavailable"), { status: 503 })
          : HttpResponse.json({
              events: [profileEvent({ name: "after_retry" })],
              next_before: null,
            }),
      ),
    );
    renderTable({ locale: "en" });
    const retry = await screen.findByRole("button", { name: /try again/i });
    fail = false;
    await userEvent.click(retry);
    await waitFor(() => expect(screen.getByText("after_retry")).toBeInTheDocument());
  });
});
