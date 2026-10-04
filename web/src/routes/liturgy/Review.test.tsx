// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0
import { afterEach, describe, expect, it, vi } from "vitest";
import { screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { LiturgyView } from "@liturgist/api-client";
import i18n from "@/lib/i18n";
import { duties, liturgy, noEdits, noParts } from "@/test/liturgy";
import { meWith } from "@/test/library";
import { mockApi, renderPage } from "@/test/render";
import { LiturgyPage } from "./LiturgyPage";

afterEach(() => vi.unstubAllGlobals());
void i18n.changeLanguage("en");

const none = { edit: false, delete: false, submit: false, approve: false, request_changes: false, reopen: false, comment: false };
const user = { id: "u9", name: "Ruth" };
const change = (to: string, from: string, note = "") => ({ id: "c-" + to, from_state: from, to_state: to, user, note, edit_seq: 7, created_at: "2026-10-02T09:00:00Z" });
const history = (...items: ReturnType<typeof change>[]) => ({ status: 200, body: { items, total: items.length } });

const base = (l: LiturgyView, h = history()) => ({
  "GET /liturgies/l1": { status: 200, body: l },
  "GET /duties": duties,
  "GET /singing-parts": noParts,
  "GET /liturgies/l1/edits": noEdits,
  "GET /liturgies/l1/state-changes": h,
  "GET /liturgies/l1/comments": { status: 200, body: { items: [], open: 0 } },
  "GET /liturgies/assignable": { status: 200, body: { items: [] } },
});
const page = (scopes = ["liturgy.edit", "liturgy.approve"]) => renderPage("/liturgies/:id", "/liturgies/l1", <LiturgyPage />, meWith(scopes));
const states = {
  draft: liturgy(),
  in_review: liturgy({ state: "in_review", actions: { ...none, approve: true, request_changes: true } }),
  needs_revision: liturgy({ state: "needs_revision", actions: { ...none, edit: true, submit: true }, last_change: change("needs_revision", "in_review", "Ganti doa") as never }),
  approved: liturgy({ state: "approved", actions: { ...none, reopen: true } }),
};
const buttons = () => screen.queryAllByRole("button").map((b) => b.textContent).filter((x) => /^(Submit for review|Approve|Request changes|Reopen as draft)$/.test(x ?? ""));

describe("WT-R-001 review bar", () => {
  it.each([
    ["draft", ["Submit for review"], null],
    ["in_review", ["Approve", "Request changes"], /being reviewed/],
    ["needs_revision", ["Submit for review"], /Changes were requested/],
    ["approved", ["Reopen as draft"], /Approved\. Reopen it/],
  ] as const)("shows exactly the buttons the actions allow in %s", async (state, want, banner) => {
    mockApi(base(states[state]));
    page();
    await screen.findByRole("heading", { name: "Review" });
    expect(buttons()).toEqual(want);
    if (banner) expect(screen.getByText(banner)).toBeInTheDocument();
  });

  it("shows the reviewer's note in needs_revision", async () => {
    mockApi(base(states.needs_revision));
    page();
    expect(await screen.findByText("Ruth wrote: Ganti doa")).toBeInTheDocument();
  });

  it("shows no buttons and no history to someone without a liturgy scope", async () => {
    const l = liturgy({ state: "published", actions: none });
    delete (l as { open_comments?: number }).open_comments;
    const calls = mockApi(base(l));
    page([]);
    expect(await screen.findByText("This liturgy is final and can't be edited.")).toBeInTheDocument();
    expect(buttons()).toEqual([]);
    expect(screen.queryByText("Review history")).toBeNull();
    expect(calls.some((c) => c.route.endsWith("/state-changes"))).toBe(false);
  });

  it("lists the review history", async () => {
    mockApi(base(states.in_review, history(change("in_review", "draft", "siap"))));
    page();
    expect(await screen.findByText("Ruth: Draft → In review")).toBeInTheDocument();
    expect(screen.getByText("siap")).toBeInTheDocument();
  });
});

describe("WT-R-002 approve", () => {
  it("sends the edit_seq it displayed and the note, then shows the new state", async () => {
    const calls = mockApi({
      ...base(states.in_review),
      "POST /liturgies/l1/approve": { status: 200, body: { ...states.approved, edit_seq: 7 } },
    });
    page();
    await userEvent.click(await screen.findByRole("button", { name: "Approve" }));
    await userEvent.type(screen.getByLabelText(/Note/), "Baik");
    await userEvent.click(screen.getByRole("button", { name: "Send" }));
    expect(await screen.findByText(/Approved\. Reopen it/)).toBeInTheDocument();
    expect(calls.find((c) => c.route === "POST /liturgies/l1/approve")!.body).toEqual({ note: "Baik", edit_seq: 7 });
    expect(screen.getByRole("button", { name: "Reopen as draft" })).toBeInTheDocument();
  });

  it("asks once more when comments are open", async () => {
    const calls = mockApi({
      ...base({ ...states.in_review, open_comments: 2 } as LiturgyView),
      "POST /liturgies/l1/approve": { status: 200, body: states.approved },
    });
    page();
    await userEvent.click(await screen.findByRole("button", { name: "Approve" }));
    await userEvent.click(screen.getByRole("button", { name: "Send" }));
    expect(await screen.findByText("2 comments are still open.")).toBeInTheDocument();
    expect(calls.some((c) => c.route === "POST /liturgies/l1/approve")).toBe(false);
    await userEvent.click(screen.getByRole("button", { name: "Approve anyway" }));
    expect(await screen.findByText(/Approved\. Reopen it/)).toBeInTheDocument();
  });

  it("reloads the liturgy on review_stale", async () => {
    let loads = 0;
    const calls = mockApi({
      ...base(states.in_review),
      "GET /liturgies/l1": () => { loads++; return { status: 200, body: states.in_review }; },
      "POST /liturgies/l1/approve": { status: 409, body: { code: "review_stale" } },
    });
    page();
    await userEvent.click(await screen.findByRole("button", { name: "Approve" }));
    await userEvent.click(screen.getByRole("button", { name: "Send" }));
    expect(await screen.findByText("The liturgy changed after you opened it. Read it again.")).toBeInTheDocument();
    await vi.waitFor(() => expect(loads).toBe(2));
    expect(calls.filter((c) => c.route === "POST /liturgies/l1/approve")).toHaveLength(1);
  });
});

describe("WT-R-003 submit", () => {
  it("lists the unfinished items with links when the server refuses", async () => {
    mockApi({
      ...base(states.draft),
      "POST /liturgies/l1/submit": { status: 422, body: { code: "has_problems", problems: [{ code: "song_missing", item_id: "i2" }] } },
    });
    page();
    await userEvent.click(await screen.findByRole("button", { name: "Submit for review" }));
    const link = await screen.findByRole("link", { name: "Item 2 (Pujian) has no song yet." });
    expect(link).toHaveAttribute("href", "#item-i2");
    expect(screen.getByText("Finish these items before submitting:")).toBeInTheDocument();
  });

  it("says so when there are no items", async () => {
    mockApi({ ...base(states.draft), "POST /liturgies/l1/submit": { status: 422, body: { code: "empty_liturgy" } } });
    page();
    await userEvent.click(await screen.findByRole("button", { name: "Submit for review" }));
    expect(await screen.findByText("Add at least one item before submitting.")).toBeInTheDocument();
  });

  it("submits at once and announces it", async () => {
    const calls = mockApi({ ...base(states.draft), "POST /liturgies/l1/submit": { status: 200, body: states.in_review } });
    page();
    await userEvent.click(await screen.findByRole("button", { name: "Submit for review" }));
    expect(await screen.findByText(/being reviewed/)).toBeInTheDocument();
    expect(calls.find((c) => c.route === "POST /liturgies/l1/submit")!.body).toEqual({});
    expect(screen.getAllByText("Submitted for review.").length).toBeGreaterThan(0);
    expect(screen.queryByRole("button", { name: "Save item" })).toBeNull();
  });
});

describe("WT-R-005 Indonesian", () => {
  it("uses the review.* texts of id", async () => {
    await i18n.changeLanguage("id");
    try {
      mockApi(base(states.in_review));
      page();
      expect(await screen.findByRole("button", { name: "Setujui" })).toBeInTheDocument();
      expect(screen.getByRole("button", { name: "Minta perubahan" })).toBeInTheDocument();
    } finally {
      await i18n.changeLanguage("en");
    }
  });
});
