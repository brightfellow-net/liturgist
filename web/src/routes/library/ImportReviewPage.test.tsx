// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0
import { afterEach, describe, expect, it, vi } from "vitest";
import { screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ImportCandidateView } from "@liturgist/api-client";
import i18n from "@/lib/i18n";
import { batch, candidate, editor } from "@/test/library";
import { mockApi, renderPage } from "@/test/render";
import { ImportReviewPage, withoutDuplicate } from "./ImportReviewPage";

afterEach(() => vi.unstubAllGlobals());

const dup = { id: "s9", title: "Besar Setia-Mu", hymnal_source: "KJ", hymnal_number: "12" };
const open = () => renderPage("/library/import/:id", "/library/import/b1", <ImportReviewPage />, editor);

describe("ImportReviewPage", () => {
  void i18n.changeLanguage("en");

  it("lists the songs with warnings in words and a link to the duplicate", async () => {
    mockApi({
      "GET /imports/b1": {
        status: 200,
        body: batch([
          candidate("c1", "Besar Setia-Mu", { duplicate_of: dup, warnings: ["chorus_guessed"] }),
          candidate("c2", "Amazing Grace", { warnings: ["title_from_file"] }),
        ]),
      },
    });
    open();
    const first = (await screen.findByRole("button", { name: "Besar Setia-Mu" })).closest("li")!;
    expect(within(first).getByRole("link", { name: "Besar Setia-Mu" })).toHaveAttribute("href", "/library/songs/s9");
    expect(within(first).getByText("A part that comes back several times was taken to be the chorus.")).toBeInTheDocument();
    expect(within(first).getByText("2 sections · Not chosen yet")).toBeInTheDocument();
    expect(screen.getByText("The title was taken from the file name.")).toBeInTheDocument();
    expect(screen.getByText("2 songs found")).toBeInTheDocument();
  });

  it("suggests merging for a duplicate but never preselects anything", async () => {
    mockApi({ "GET /imports/b1": { status: 200, body: batch([candidate("c1", "Besar Setia-Mu", { duplicate_of: dup })]) } });
    open();
    const group = await screen.findByRole("group", { name: "What should happen to this song?" });
    const radios = within(group).getAllByRole("radio");
    expect(radios.map((r) => (r as HTMLInputElement).labels?.[0].textContent)).toEqual(["Add as new song", "Merge into “Besar Setia-Mu”", "Skip"]);
    expect(radios.every((r) => !(r as HTMLInputElement).checked)).toBe(true);
    expect(screen.getByRole("button", { name: "Import chosen songs" })).toBeDisabled();
  });

  it("offers no merge for a song without a duplicate", async () => {
    mockApi({ "GET /imports/b1": { status: 200, body: batch([candidate("c1", "Baru")]) } });
    open();
    const group = await screen.findByRole("group", { name: "What should happen to this song?" });
    expect(within(group).queryByLabelText(/Merge into/)).toBeNull();
  });

  it("saves the choice Add as new song", async () => {
    const calls = mockApi({
      "GET /imports/b1": { status: 200, body: batch([candidate("c1", "Baru")]) },
      "PATCH /imports/b1/candidates": { status: 200, body: [] },
    });
    open();
    await userEvent.click(await screen.findByLabelText("Add as new song"));
    await vi.waitFor(() => expect(calls.find((c) => c.route === "PATCH /imports/b1/candidates")?.body).toEqual({ decisions: [{ id: "c1", decision: "accept" }] }));
  });

  it("adds all without duplicates and leaves the duplicates to the member", async () => {
    const list = [
      candidate("c1", "Baru"),
      candidate("c2", "Sudah ada", { duplicate_of: dup }),
      candidate("c3", "Kembar", { warnings: ["duplicate_in_batch"] }),
      candidate("c4", "Sudah dilewati", { decision: "skip" }),
      candidate("c5", "Lagi baru"),
    ];
    expect(withoutDuplicate(list).map((c) => c.id)).toEqual(["c1", "c5"]);
    const calls = mockApi({
      "GET /imports/b1": { status: 200, body: batch(list) },
      "PATCH /imports/b1/candidates": { status: 200, body: [] },
    });
    open();
    await userEvent.click(await screen.findByRole("button", { name: "Add all without duplicates" }));
    await vi.waitFor(() => expect(calls.find((c) => c.route === "PATCH /imports/b1/candidates")?.body).toEqual({
      decisions: [{ id: "c1", decision: "accept" }, { id: "c5", decision: "accept" }],
    }));
  });

  it("shows the merge preview, offers to remove unmatched sections and saves the version it showed", async () => {
    const calls = mockApi({
      "GET /imports/b1": { status: 200, body: batch([candidate("c1", "Besar Setia-Mu", { duplicate_of: dup })]) },
      "GET /imports/b1/candidates/c1/merge-preview": (): { status: number; body: unknown } => ({
        status: 200,
        body: {
          target_version: 4,
          sections: [
            { status: "updated", label: "Verse 1", old_text: "lama", new_text: "baru" },
            { status: "kept", label: "Verse 2", old_text: "tetap" },
          ],
        },
      }),
      "PATCH /imports/b1/candidates": { status: 200, body: [] },
    });
    open();
    await userEvent.click(await screen.findByLabelText(/Merge into/));
    expect(await screen.findByText("Merge preview")).toBeInTheDocument();
    expect(screen.getByText("lama")).toBeInTheDocument();
    expect(screen.getByText("baru")).toBeInTheDocument();
    const remove = screen.getByLabelText("Remove the 1 sections that are not in the import");
    expect(remove).not.toBeChecked();
    await userEvent.click(remove);
    await userEvent.click(screen.getByRole("button", { name: "Use this merge" }));
    await vi.waitFor(() => expect(calls.find((c) => c.route === "PATCH /imports/b1/candidates")?.body).toEqual({
      decisions: [{ id: "c1", decision: "merge", merge_into: "s9", merge_target_version: 4, remove_unmatched: true }],
    }));
  });

  it("applies the chosen songs, then lists what happened and retries the failures", async () => {
    let applied = 0;
    const chosen: ImportCandidateView[] = [
      candidate("c1", "Baru", { decision: "accept" }),
      candidate("c2", "Gagal", { decision: "merge", merge_into: "s9", outcome: "failed", error_code: "section_in_use", duplicate_of: dup }),
    ];
    const calls = mockApi({
      "GET /imports/b1": () => ({
        status: 200,
        body: applied === 0 ? batch(chosen) : batch([
          { ...chosen[0], outcome: "applied", applied_song_id: "s1" }, chosen[1],
        ]),
      }),
      "POST /imports/b1/apply": (): { status: number; body: unknown } => {
        applied++;
        return { status: 200, body: { created: 1, merged: 0, skipped: 0, status: "open", failed: [{ candidate_id: "c2", code: "section_in_use" }] } };
      },
    });
    open();
    await userEvent.click(await screen.findByRole("button", { name: "Import chosen songs" }));
    expect(await screen.findByText("1 added, 0 merged, 0 skipped.")).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Baru" })).toHaveAttribute("href", "/library/songs/s1");
    const failed = screen.getByText(/1 song could not be imported/).closest("div")!;
    expect(within(failed).getByText(/Gagal: A section of the song to merge into is used in a liturgy/)).toBeInTheDocument();
    await userEvent.click(within(failed).getByRole("button", { name: "Try again" }));
    await vi.waitFor(() => expect(calls.filter((c) => c.route === "POST /imports/b1/apply")).toHaveLength(2));
  });

  it("reopens the preview of a song that changed after the choice", async () => {
    mockApi({
      "GET /imports/b1": {
        status: 200,
        body: batch([candidate("c1", "Besar Setia-Mu", { duplicate_of: dup, decision: "merge", merge_into: "s9", merge_target_version: 3, outcome: "failed", error_code: "target_changed" })]),
      },
      "GET /imports/b1/candidates/c1/merge-preview": { status: 200, body: { target_version: 5, sections: [{ status: "new", label: "Verse 1", new_text: "baru" }] } },
    });
    open();
    expect(await screen.findByText("Merge preview")).toBeInTheDocument();
    expect(screen.getAllByText(/The song to merge into was changed after you chose/).length).toBeGreaterThan(0);
    expect(await screen.findByRole("button", { name: "Use this merge" })).toBeInTheDocument();
  });

  it("edits a song before it is imported", async () => {
    const calls = mockApi({
      "GET /imports/b1": { status: 200, body: batch([candidate("c1", "Besar Setia-Mu")]) },
      "PATCH /imports/b1/candidates/c1": { status: 200, body: candidate("c1", "Judul baru") },
    });
    open();
    await userEvent.click(await screen.findByRole("button", { name: "Edit this song" }));
    const title = screen.getByLabelText("Title");
    await userEvent.clear(title);
    await userEvent.type(title, "Judul baru");
    await userEvent.click(screen.getByRole("button", { name: "Save changes" }));
    await vi.waitFor(() => expect(calls.find((c) => c.route === "PATCH /imports/b1/candidates/c1")).toBeDefined());
    const body = calls.find((c) => c.route === "PATCH /imports/b1/candidates/c1")!.body as { draft: { title: string; default_arrangement: number[] } };
    expect(body.draft.title).toBe("Judul baru");
    expect(body.draft.default_arrangement).toEqual([0, 1, 0]);
  });

  it("says when the import no longer exists", async () => {
    mockApi({});
    open();
    expect(await screen.findByText("This import no longer exists. Start again.")).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Start a new import" })).toHaveAttribute("href", "/library/import");
  });
});
