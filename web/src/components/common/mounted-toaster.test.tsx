import { render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { MountedToaster } from "./mounted-toaster";

// Тостер стоит в корневом layout вне MessagesProvider: язык подписей — из <html lang>.
describe("MountedToaster", () => {
  afterEach(() => {
    document.documentElement.lang = "";
  });

  it("labels the notifications region in Russian on Russian pages", async () => {
    document.documentElement.lang = "ru";
    render(<MountedToaster />);
    expect(await screen.findByRole("region", { name: /^Уведомления/ })).toBeInTheDocument();
  });

  it("labels it in English otherwise", async () => {
    document.documentElement.lang = "en";
    render(<MountedToaster />);
    expect(await screen.findByRole("region", { name: /^Notifications/ })).toBeInTheDocument();
  });
});
