// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0
import { afterEach, describe, expect, it, vi } from "vitest";
import { screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { LiturgyView } from "@liturgist/api-client";
import i18n from "@/lib/i18n";
import { duties, liturgy, noEdits, noParts } from "@/test/liturgy";
import { meWith } from "@/test/library";
import { mockApi, renderPage } from "@/test/render";
import { LiturgyPage } from "./LiturgyPage";

afterEach(() => vi.unstubAllGlobals());
void i18n.changeLanguage("en");

const author = { id: "u9", name: "Ruth" };
const comment = (id: string, body: string, changes: Record<string, unknown> = {}) => ({
  id, item_id: "i1", item_title: "Votum", author, body, resolved: false, created_at: "2026-10-02T09:00:00Z", ...changes,
});
const list = (...items: ReturnType<typeof comment>[]) => ({ status: 200, body: { items, open: items.filter((c) => !c.resolved).length } });

const base = (l: LiturgyView, comments = list()) => ({
  "GET /liturgies/l1": { status: 200, body: l },
  "GET /duties": duties,
  "GET /singing-parts": noParts,
  "GET /liturgies/l1/edits": noEdits,
  "GET /liturgies/l1/state-changes": { status: 200, body: { items: [], total: 0 } },
  "GET /liturgies/l1/comments": comments,
  "GET /liturgies/assignable": { status: 200, body: { items: [] } },
});
const page = () => renderPage("/liturgies/:id", "/liturgies/l1", <LiturgyPage />, meWith(["liturgy.edit", "liturgy.comment", "liturgy.approve"]));
const inReview = () => liturgy({ state: "in_review", actions: { edit: false, delete: false, submit: false, approve: true, request_changes: true, reopen: false, comment: true, publish: false, archive: false, unarchive: false } });

describe("WT-R-004 comments", () => {
  it("adds a comment on an item and on the whole liturgy", async () => {
    let seen = list();
    const calls = mockApi({
      ...base(liturgy()),
      "GET /liturgies/l1/comments": () => seen,
      "POST /liturgies/l1/comments": (body) => {
        const b = body as { item_id?: string; body: string };
        seen = list(...seen.body.items, comment("c" + (seen.body.items.length + 1), b.body, b.item_id ? {} : { item_id: null, item_title: "" }));
        return { status: 201, body: seen.body.items.at(-1) };
      },
    });
    page();
    await userEvent.click(await screen.findByRole("button", { name: "Comment on Votum" }));
    const form = screen.getByRole("group", { name: "Comment on Votum" });
    await userEvent.type(within(form).getByLabelText(/^Comment/), "Ganti kata ini");
    await userEvent.click(within(form).getByRole("button", { name: "Add comment" }));
    expect(await screen.findAllByText("Ganti kata ini")).not.toHaveLength(0);
    expect(calls.find((c) => c.route === "POST /liturgies/l1/comments")!.body).toEqual({ body: "Ganti kata ini", item_id: "i1" });
    expect(await screen.findByText("1 open comment")).toBeInTheDocument();

    const whole = await screen.findByRole("group", { name: "Whole liturgy" });
    await userEvent.type(within(whole).getByLabelText(/^Comment/), "Urutan baik");
    await userEvent.click(within(whole).getByRole("button", { name: "Add comment" }));
    await vi.waitFor(() => expect(calls.filter((c) => c.route === "POST /liturgies/l1/comments")).toHaveLength(2));
    expect(calls.filter((c) => c.route === "POST /liturgies/l1/comments")[1].body).toEqual({ body: "Urutan baik" });
  });

  it("resolves and reopens, and folds the resolved ones", async () => {
    let seen = list(comment("c1", "Cek lagi"));
    const calls = mockApi({
      ...base(inReview()),
      "GET /liturgies/l1/comments": () => seen,
      "PUT /liturgies/l1/comments/c1/resolved": (body) => {
        const resolved = (body as { resolved: boolean }).resolved;
        seen = list(comment("c1", "Cek lagi", resolved ? { resolved: true, resolved_by: author } : {}));
        return { status: 200, body: seen.body.items[0] };
      },
    });
    page();
    const row = (await screen.findAllByText("Cek lagi"))[0].closest("li")!;
    await userEvent.click(within(row).getByRole("button", { name: "Resolve" }));
    expect(await screen.findByText("1 resolved comment")).toBeInTheDocument();
    expect(calls.find((c) => c.route.startsWith("PUT"))!.body).toEqual({ resolved: true });
    await userEvent.click(screen.getByText("1 resolved comment"));
    const again = screen.getAllByText("Cek lagi")[0].closest("li")!;
    expect(within(again).getByText(/Resolved by Ruth/)).toBeInTheDocument();
    await userEvent.click(within(again).getByRole("button", { name: "Reopen" }));
    expect(await screen.findByText("1 open comment")).toBeInTheDocument();
  });

  it("works while the liturgy is in review, where the cards are read-only", async () => {
    mockApi(base(inReview(), list(comment("c1", "Dalam tinjauan"))));
    page();
    expect((await screen.findAllByText("Dalam tinjauan")).length).toBeGreaterThan(0);
    expect(screen.queryByRole("button", { name: "Save item" })).toBeNull();
    expect(screen.getByRole("button", { name: "Comment on Votum" })).toBeInTheDocument();
  });

  it("shows a comment on a removed item with its kept title, and filters in the panel", async () => {
    mockApi(base(liturgy(), list(comment("c1", "Hilang", { item_id: "gone", item_title: "Doa Lama" }), comment("c2", "Selesai", { resolved: true, resolved_by: author }))));
    page();
    expect(await screen.findByText("On a removed item: Doa Lama")).toBeInTheDocument();
    await userEvent.click(screen.getByRole("button", { name: "Resolved" }));
    expect((await screen.findAllByText("Selesai")).length).toBeGreaterThan(0);
    expect(screen.queryByText("Hilang")).toBeNull();
    await userEvent.click(screen.getByRole("button", { name: "All" }));
    expect(await screen.findByText("Hilang")).toBeInTheDocument();
  });

  it("offers no comment buttons without the scope, and nothing without review data", async () => {
    const l = liturgy({ actions: { ...liturgy().actions, comment: false } });
    mockApi(base(l, list(comment("c1", "Dilihat"))));
    renderPage("/liturgies/:id", "/liturgies/l1", <LiturgyPage />, meWith(["liturgy.edit"]));
    expect((await screen.findAllByText("Dilihat")).length).toBeGreaterThan(0);
    expect(screen.queryByRole("button", { name: /^Comment on/ })).toBeNull();
    expect(screen.queryByRole("button", { name: "Resolve" })).toBeNull();
  });

  it("explains the limit", async () => {
    mockApi({ ...base(liturgy()), "POST /liturgies/l1/comments": { status: 422, body: { code: "comment_limit" } } });
    page();
    const whole = await screen.findByRole("group", { name: "Whole liturgy" });
    await userEvent.type(within(whole).getByLabelText(/^Comment/), "x");
    await userEvent.click(within(whole).getByRole("button", { name: "Add comment" }));
    expect(await screen.findByText("This liturgy has reached its limit of 500 comments.")).toBeInTheDocument();
  });
});
