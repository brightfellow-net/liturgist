// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0
import { afterEach, describe, expect, it, vi } from "vitest";
import { screen } from "@testing-library/react";
import type { Me } from "@liturgist/api-client";
import i18n from "@/lib/i18n";
import { meWith } from "@/test/library";
import { copy } from "@/test/published";
import { mockApi, renderPage } from "@/test/render";
import { AppLayout } from "./AppLayout";
import { PrintPage } from "./published/PrintPage";
import { PublishedPage } from "./published/PublishedPage";

afterEach(() => vi.unstubAllGlobals());
void i18n.changeLanguage("en");

const url = "/api/v1/church/logo?v=0123456789abcdef";
const me = (logo_url: string | null): Me => {
  const m = meWith([]);
  return { ...m, church: { ...m.church!, logo_url } } as Me;
};
const logoOf = (name: string) => screen.getByText(name).parentElement!.querySelector("img");

// WT-L-002: the logo sits left of the church name in the app header, the published view and the print header.
describe("the church logo", () => {
  it("is in the app header, before the name", async () => {
    mockApi({ "GET /me": { status: 200, body: me(url) } });
    renderPage("/", "/", <AppLayout />);
    await screen.findByRole("navigation");
    const img = logoOf("GKY Uji");
    expect(img).toHaveAttribute("src", url);
    expect(img!.nextElementSibling).toHaveTextContent("GKY Uji");
  });

  it("is not in the header of a church without one", async () => {
    mockApi({ "GET /me": { status: 200, body: me(null) } });
    renderPage("/", "/", <AppLayout />);
    await screen.findByRole("navigation");
    expect(document.querySelector("header img")).toBeNull();
  });

  it("is in the published view", async () => {
    mockApi({ "GET /liturgies/l1/published": { status: 200, body: copy() } });
    renderPage("/published/:id", "/published/l1", <PublishedPage />, me(url));
    await screen.findByRole("heading", { level: 1, name: "Ibadah Umum" });
    expect(logoOf("GKY Uji")).toHaveAttribute("src", url);
  });

  it("is in the print header", async () => {
    mockApi({ "GET /liturgies/l1/published": { status: 200, body: copy() } });
    renderPage("/published/:id/print", "/published/l1/print", <PrintPage />, me(url));
    await screen.findByRole("heading", { level: 1, name: "Ibadah Umum" });
    expect(screen.getAllByText("GKY Uji").map((e) => e.parentElement!.querySelector("img")).filter(Boolean)).toHaveLength(1);
    expect(logoOf("GKY Uji")).toHaveAttribute("src", url);
  });

  it("is left out of a published view of a church without one, and of the same page without the app frame", async () => {
    mockApi({ "GET /liturgies/l1/published": { status: 200, body: copy() } });
    renderPage("/published/:id/print", "/published/l1/print", <PrintPage />);
    await screen.findByRole("heading", { level: 1, name: "Ibadah Umum" });
    expect(document.querySelector("img")).toBeNull();
  });
});
