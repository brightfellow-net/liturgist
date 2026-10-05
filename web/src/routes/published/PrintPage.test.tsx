// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0
import { afterEach, describe, expect, it, vi } from "vitest";
import { screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import i18n from "@/lib/i18n";
import { parsePrint, printParams } from "@/lib/print";
import { viewer } from "@/test/library";
import { copy } from "@/test/published";
import { mockApi, renderPage } from "@/test/render";
import { PrintPage } from "./PrintPage";

afterEach(() => vi.unstubAllGlobals());
void i18n.changeLanguage("en");

const view = (query = "", body = copy()) => {
  mockApi({ "GET /liturgies/l1/published": { status: 200, body } });
  return renderPage("/published/:id/print", `/published/l1/print${query}`, <PrintPage />, viewer);
};

// WT-P-004
describe("PrintPage", () => {
  it("prints the team sheet from the church defaults, with @page for A4", async () => {
    view();
    expect(await screen.findByRole("heading", { level: 1, name: "Ibadah Umum" })).toBeInTheDocument();
    expect(screen.getByText("Liturgis: Ruth")).toBeInTheDocument();
    expect(screen.getAllByText("Setiap pagi")).toHaveLength(2); // full lyrics, the chorus twice
    expect(screen.getByText("Karena begitu besar kasih Allah")).toBeInTheDocument();
    expect(screen.getByText("Public domain · CCLI 123")).toBeInTheDocument();
    expect(screen.getByText("CCLI License #1234567")).toBeInTheDocument();
    expect(document.querySelector("style")?.textContent).toContain("size: A4");
    // The controls and the browser frame stay off the paper.
    expect(screen.getByRole("button", { name: "Print / Save as PDF" }).parentElement).toHaveClass("print:hidden");
  });

  it("keeps a song heading with its first row, and a row whole", async () => {
    view();
    await screen.findByRole("heading", { level: 1 });
    const heading = screen.getByRole("heading", { level: 3 });
    expect(heading).toHaveClass("break-after-avoid");
    expect(heading.parentElement).toHaveClass("break-inside-avoid");
    expect(document.querySelectorAll(".break-inside-avoid").length).toBeGreaterThan(2);
  });

  it("follows the options in the address: F4, first lines, no readings, no keys, no assignments, no notes", async () => {
    view("?paper=f4&lyrics=first_lines&readings=0&keys=0&assignments=0&notes=0&size=large");
    await screen.findByRole("heading", { level: 1 });
    expect(document.querySelector("style")?.textContent).toContain("215mm 330mm");
    expect(screen.getAllByText("Setiap pagi")).toHaveLength(2); // a one-line section: its first line is all of it
    expect(screen.getByText("Yoh 3:16 (TB)")).toBeInTheDocument();
    expect(screen.queryByText("Karena begitu besar kasih Allah")).toBeNull();
    expect(screen.queryByText("Liturgis: Ruth")).toBeNull();
    expect(screen.queryByText("Kolektan: Pak Budi")).toBeNull();
    expect(screen.queryByText("pelan")).toBeNull();
    expect(screen.getByRole("heading", { level: 3 })).toHaveTextContent("Besar Setia-Mu — KJ 12");
    expect(screen.queryByText(/change to/)).toBeNull();
    expect(document.querySelector("article")).toHaveClass("text-xl");
  });

  it("first lines prints the first line of a long section, full lyrics print all of it", async () => {
    const body = copy();
    const song = body.content.items![2].songs![0];
    song.sections![0].text = "Besar setia-Mu\n\nTak berkesudahan";
    view("?lyrics=first_lines", body);
    await screen.findByRole("heading", { level: 1 });
    expect(screen.getByText("Besar setia-Mu")).toBeInTheDocument();
    expect(screen.queryByText(/Tak berkesudahan/)).toBeNull();
  });

  it("full lyrics print every line of a section", async () => {
    const body = copy();
    body.content.items![2].songs![0].sections![0].text = "Besar setia-Mu\nTak berkesudahan";
    view("", body);
    expect(await screen.findByText(/Tak berkesudahan/)).toBeInTheDocument();
  });

  it("an invalid option falls back to the church default", async () => {
    const body = copy();
    body.render.print = { ...body.render.print, paper: "f4", size: "large", keys: false };
    view("?paper=a3&size=huge&keys=maybe&variant=other", body);
    await screen.findByRole("heading", { level: 1 });
    expect(document.querySelector("style")?.textContent).toContain("215mm 330mm");
    expect(document.querySelector("article")).toHaveClass("text-xl");
    expect(screen.getByRole("heading", { level: 3 })).toHaveTextContent("Besar Setia-Mu — KJ 12");
    expect(document.querySelector("article")).toHaveAttribute("data-variant", "team");
  });

  it("the musician sheet: songs only, first lines, keys and parts, no credits", async () => {
    view("?variant=musician");
    await screen.findByRole("heading", { level: 1 });
    expect(screen.queryByText("Doa Pembuka")).toBeNull();
    expect(screen.queryByText("Karena begitu besar kasih Allah")).toBeNull();
    expect(screen.queryByText("Public domain · CCLI 123")).toBeNull();
    expect(screen.getByRole("heading", { level: 3 })).toHaveTextContent("Besar Setia-Mu — KJ 12 · Do = G");
    expect(screen.getByText("Refren · Jemaat · change to Do = A")).toBeInTheDocument();
    expect(screen.getByText("pelan")).toBeInTheDocument();
    expect(document.querySelector("article")).toHaveAttribute("data-variant", "musician");
    expect(screen.queryByLabelText("Lyrics")).toBeNull(); // the musician sheet always has first lines
  });

  it("show_credits off drops the credits and the licence line", async () => {
    const body = copy();
    body.render.show_credits = false;
    view("", body);
    await screen.findByRole("heading", { level: 1 });
    expect(screen.queryByText("Public domain · CCLI 123")).toBeNull();
    expect(screen.queryByText("CCLI License #1234567")).toBeNull();
  });

  it("choosing an option changes the page and the address, never the church", async () => {
    const calls = mockApi({ "GET /liturgies/l1/published": { status: 200, body: copy() } });
    renderPage("/published/:id/print", "/published/l1/print", <PrintPage />, viewer);
    await userEvent.selectOptions(await screen.findByLabelText("Paper"), "f4");
    expect(document.querySelector("style")?.textContent).toContain("215mm 330mm");
    await userEvent.click(screen.getByLabelText("Show readings"));
    expect(screen.queryByText("Karena begitu besar kasih Allah")).toBeNull();
    expect(calls.every((c) => c.route.startsWith("GET"))).toBe(true);
  });

  it("calls window.print", async () => {
    const print = vi.fn();
    vi.stubGlobal("print", print);
    view();
    await userEvent.click(await screen.findByRole("button", { name: "Print / Save as PDF" }));
    expect(print).toHaveBeenCalledOnce();
  });

  it("a liturgy with no copy says so", async () => {
    mockApi({ "GET /liturgies/l1/published": { status: 404, body: { code: "not_found", title: "Not found", status: 404 } } });
    renderPage("/published/:id/print", "/published/l1/print", <PrintPage />, viewer);
    expect(await screen.findByText(/./, { selector: "[role=alert], [role=status]" })).toBeInTheDocument();
  });

  it("refuses a copy of a newer format", async () => {
    const body = copy();
    body.content.format = 2;
    view("", body);
    expect(await screen.findByText(/update the app/i)).toBeInTheDocument();
  });
});

describe("print options", () => {
  const d = copy().render.print;
  it("round-trips through the address", () => {
    const o = { ...parsePrint(new URLSearchParams(), d), variant: "musician" as const, paper: "f4" as const, keys: false };
    expect(parsePrint(printParams(o), d)).toEqual(o);
  });
});
