// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0
import { afterEach, describe, expect, it, vi } from "vitest";
import { screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import i18n from "@/lib/i18n";
import { meWith, viewer } from "@/test/library";
import { copy } from "@/test/published";
import { mockApi, renderPage } from "@/test/render";
import { HomePage } from "../HomePage";
import { PublishedListPage } from "./PublishedListPage";
import { PublishedPage } from "./PublishedPage";

afterEach(() => vi.unstubAllGlobals());
void i18n.changeLanguage("en");

const view = (me = viewer) => renderPage("/published/:id", "/published/l1", <PublishedPage />, me);
const published = (body = copy()) => ({ "GET /liturgies/l1/published": { status: 200, body } });

// WT-P-002
describe("PublishedPage", () => {
  it("shows the stored copy: header, items, reading, songs in sequence, team, credits", async () => {
    mockApi(published());
    view();
    expect(await screen.findByRole("heading", { level: 1, name: "Ibadah Umum" })).toBeInTheDocument();
    expect(screen.getByText("GKY Uji")).toBeInTheDocument();
    expect(screen.getByText(/Version 2, published .* by Admin\./)).toBeInTheDocument();
    // Duty with the people on it; the prayer text keeps its line break.
    const prayer = screen.getByRole("region", { name: "Doa Pembuka" });
    expect(within(prayer).getByText("Liturgis: Ruth")).toBeInTheDocument();
    expect(within(prayer).getByText(/terima kasih/)).toHaveClass("whitespace-pre-line");
    // Reading with its attribution.
    const reading = screen.getByRole("region", { name: "Pembacaan" });
    expect(within(reading).getByText("Yoh 3:16 (TB)")).toBeInTheDocument();
    expect(within(reading).getByText("© LAI")).toBeInTheDocument();
    // The song: key in the church's display form, the chorus twice, the part, the key change.
    const pujian = screen.getByRole("region", { name: "Pujian" });
    expect(within(pujian).getByRole("heading", { level: 3 })).toHaveTextContent("Besar Setia-Mu — KJ 12 · Do = G");
    expect(within(pujian).getAllByText("Setiap pagi")).toHaveLength(2);
    expect(within(pujian).getByText("Refren · Jemaat · change to Do = A")).toBeInTheDocument();
    expect(within(pujian).getByText("Public domain · CCLI 123")).toBeInTheDocument();
    // A duty without items is listed under Team; the licence footer closes the page.
    expect(within(screen.getByRole("region", { name: "Team" })).getByText("Kolektan: Pak Budi")).toBeInTheDocument();
    expect(screen.getByText("CCLI License #1234567")).toBeInTheDocument();
    expect(document.querySelector("[lang='id']")).not.toBeNull();
  });

  it("shows keys as letters, and no credits, as the church setting says", async () => {
    mockApi(published(copy({ render: { ...copy().render, key_display: "letter", show_credits: false } })));
    view();
    const pujian = await screen.findByRole("region", { name: "Pujian" });
    expect(within(pujian).getByRole("heading", { level: 3 })).toHaveTextContent("Besar Setia-Mu — KJ 12 · G");
    expect(screen.queryByText(/Public domain/)).toBeNull();
    expect(screen.queryByText("CCLI License #1234567")).toBeNull();
  });

  it("never takes a text as HTML", async () => {
    const c = copy();
    (c.content.items ?? [])[0].text = "<img src=x onerror=alert(1)> <b>tebal</b>";
    mockApi(published(c));
    view();
    expect(await screen.findByText("<img src=x onerror=alert(1)> <b>tebal</b>")).toBeInTheDocument();
    expect(document.querySelector("img")).toBeNull();
  });

  it("says when the liturgy is being revised or archived", async () => {
    mockApi(published(copy({ revising: true, archived: true })));
    view();
    expect(await screen.findByText(/is being revised\. You are reading the version published on/)).toBeInTheDocument();
    expect(screen.getByText("This liturgy is archived.")).toBeInTheDocument();
  });

  it("asks for an update when the copy is of a newer format", async () => {
    const c = copy();
    c.content.format = 2;
    mockApi(published(c));
    view();
    expect(await screen.findByText("Update the app to read this liturgy.")).toBeInTheDocument();
    expect(screen.queryByRole("heading", { level: 1 })).toBeNull();
  });

  it("explains a liturgy that has no published copy", async () => {
    mockApi({ "GET /liturgies/l1/published": { status: 404, body: { code: "not_found" } } });
    view();
    expect(await screen.findByText("This liturgy has not been published.")).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "All published liturgies" })).toHaveAttribute("href", "/published");
  });

  it("saves the text size from A, A+ and A++", async () => {
    const calls = mockApi({
      ...published(),
      "PATCH /me": (body) => ({ status: 200, body: { id: "u1", name: "Ruth", email: "r@example.org", preferences: (body as { preferences: object }).preferences } }),
    });
    view();
    const group = await screen.findByRole("group", { name: "Text size" });
    expect(within(group).getByRole("button", { name: "Normal" })).toHaveAttribute("aria-pressed", "true");
    await userEvent.click(within(group).getByRole("button", { name: "Larger" }));
    expect(calls.find((c) => c.route === "PATCH /me")?.body).toEqual({ preferences: { text_size: "larger", ui_language: null } });
  });
});

const row = (changes = {}) => ({ id: "l1", date: "2026-10-11", time: "07:00", service_name: "Ibadah Umum", language: "id", number: 2,
  published_at: "2026-10-09T03:00:00Z", revising: false, archived: false, ...changes });

describe("PublishedListPage", () => {
  const list = () => renderPage("/published", "/published", <PublishedListPage />, viewer);

  it("lists the liturgies as links, with tags", async () => {
    mockApi({ "GET /published": { status: 200, body: { items: [row({ revising: true }), row({ id: "l2", service_name: "Ibadah Sore", archived: true })], total: 2 } } });
    list();
    const link = await screen.findByRole("link", { name: /Ibadah Umum/ });
    expect(link).toHaveAttribute("href", "/published/l1");
    expect(within(link).getByText("Being revised")).toBeInTheDocument();
    expect(within(screen.getByRole("link", { name: /Ibadah Sore/ })).getByText("Archived")).toBeInTheDocument();
  });

  it("asks for archived liturgies only when the switch is on", async () => {
    const calls = mockApi({ "GET /published": { status: 200, body: { items: [], total: 0 } } });
    list();
    expect(await screen.findByText(/Nothing has been published yet/)).toBeInTheDocument();
    await userEvent.click(screen.getByRole("checkbox", { name: "Show archived" }));
    await screen.findByRole("checkbox", { checked: true });
    expect(calls.filter((c) => c.route === "GET /published")).toHaveLength(2);
  });
});

// WT-P-003
describe("HomePage (my assignments)", () => {
  const home = (me = viewer) => renderPage("/", "/", <HomePage />, me);
  const card = { liturgy: { id: "l1", date: "2026-10-11", time: "07:00", service_name: "Ibadah Umum", language: "id", number: 2, revising: false },
    duties: [{ id: "d1", name: "Liturgis" }], items: [{ id: "i1", title: "Doa Pembuka", type: "prayer" }] };

  it("shows a card per liturgy with the duties and items, linking to the view", async () => {
    mockApi({ "GET /me/assignments": { status: 200, body: { items: [card, { ...card, liturgy: { ...card.liturgy, id: "l2", revising: true } }], more: false } } });
    home();
    expect(await screen.findByRole("heading", { level: 1, name: "Welcome, Ruth" })).toBeInTheDocument();
    const links = await screen.findAllByRole("link", { name: "Open the liturgy" });
    expect(links[0]).toHaveAttribute("href", "/published/l1");
    expect(screen.getAllByText("Doa Pembuka")).toHaveLength(2);
    expect(screen.getByText("Being revised")).toBeInTheDocument();
  });

  it("explains an empty list and the home-screen steps, and says when there are more", async () => {
    mockApi({ "GET /me/assignments": { status: 200, body: { items: [], more: false } } });
    home(meWith([]));
    expect(await screen.findByText(/You have no duties in upcoming liturgies/)).toBeInTheDocument();
    expect(screen.getByText("Add this app to your home screen")).toBeInTheDocument();
    expect(screen.getByText(/Add to Home Screen/)).toBeInTheDocument();
  });

  it("notes that only the next 50 are shown", async () => {
    mockApi({ "GET /me/assignments": { status: 200, body: { items: [card], more: true } } });
    home();
    expect(await screen.findByText(/Showing the next 50/)).toBeInTheDocument();
  });
});
