// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0
import { afterEach, describe, expect, it, vi } from "vitest";
import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import i18n from "@/lib/i18n";
import { meWith } from "@/test/library";
import { copy } from "@/test/published";
import { mockApi, renderPage } from "@/test/render";
import { MessagesPage } from "./MessagesPage";
import { PublishedPage } from "./PublishedPage";

afterEach(() => vi.unstubAllGlobals());
void i18n.changeLanguage("en");

const base = {
  number: 2, url: "https://liturgi.example.org/published/l1", key_display: "do",
  liturgy: { date: "2026-10-11", time: "07:00", service_name: "Ibadah Umum", language: "id", church_name: "GKY Uji" },
  items: [
    { title: "Doa Pembuka", type: "prayer", duty: { id: "d1", name: "Liturgis" }, songs: [] },
    { title: "Pujian", type: "song", songs: [{ title: "Besar Setia-Mu", hymnal_source: "KJ", hymnal_number: "12", key: "G" }] },
    { title: "Pembacaan", type: "reading", songs: [], reading: { reference_display: "Yoh 3:16", translation_code: "TB" } },
  ],
  assignments: [{ duty: { id: "d1", name: "Liturgis" }, names: ["Ruth", "Pak Budi"] }],
  recipients: [
    { name: "Ruth", duties: ["Liturgis"], phone: "+62 812-0000-1111", member: true },
    { name: "Sari", duties: ["Liturgis"], member: true },
    { name: "Pak Budi", duties: ["Liturgis"], member: false },
  ],
};
const approver = meWith(["liturgy.approve"]);
const open = (body: object = base, me = approver) => {
  mockApi({ "GET /liturgies/l1/published/summary": { status: 200, body } });
  return renderPage("/published/:id/messages", "/published/l1/messages", <MessagesPage />, me);
};

// WT-P-006
describe("MessagesPage", () => {
  it("shows the team text in the liturgy's language, whatever the app's language", async () => {
    open();
    const team = await screen.findByRole("textbox", { name: "Text for Team summary" });
    expect((team as HTMLTextAreaElement).value).toContain("*Liturgi Ibadah Umum*\nMinggu, 11 Oktober 2026 · 07:00");
    expect((team as HTMLTextAreaElement).value).toContain("1. KJ 12 · Besar Setia-Mu · Do = G");
    expect((team as HTMLTextAreaElement).value).toContain("*Bacaan:* Yoh 3:16 (TB)");
    expect(screen.getByRole("heading", { level: 1, name: "Send to the team" })).toBeInTheDocument();
  });

  it("follows the checkboxes", async () => {
    open();
    const team = (await screen.findByRole("textbox", { name: "Text for Team summary" })) as HTMLTextAreaElement;
    await userEvent.click(screen.getByRole("checkbox", { name: "Keys" }));
    expect(team.value).toContain("1. KJ 12 · Besar Setia-Mu\n");
    await userEvent.click(screen.getByRole("checkbox", { name: "Readings" }));
    expect(team.value).not.toContain("Yoh 3:16");
    await userEvent.click(screen.getByRole("checkbox", { name: "Songs" }));
    expect(team.value).not.toContain("Besar");
  });

  it("opens WhatsApp with the encoded text: the picker for the team, the chat for a person", async () => {
    open();
    const share = await screen.findByRole("link", { name: "Share to WhatsApp" });
    const team = (screen.getByRole("textbox", { name: "Text for Team summary" }) as HTMLTextAreaElement).value;
    expect(share).toHaveAttribute("href", `https://wa.me/?text=${encodeURIComponent(team)}`);
    expect(share).toHaveAttribute("target", "_blank");
    expect(share.getAttribute("rel")).toContain("noopener");
    const ruth = screen.getByRole("link", { name: "Send to Ruth via WhatsApp" });
    expect(ruth.getAttribute("href")).toMatch(/^https:\/\/wa\.me\/6281200001111\?text=Shalom%20Ruth/);
  });

  it("gives a member with no phone a Copy button only, and a name without an account nothing", async () => {
    open();
    await screen.findByRole("heading", { name: "Personal messages" });
    expect(screen.queryByRole("link", { name: /Send to Sari/ })).toBeNull();
    expect(screen.getByRole("button", { name: "Copy for Sari" })).toBeInTheDocument();
    expect(screen.getByText("No phone number")).toBeInTheDocument();
    const budi = screen.getByRole("heading", { name: /Pak Budi/ }).closest("li")!;
    expect(within(budi).queryByRole("button")).toBeNull();
    expect(within(budi).getByText("Name only, no account")).toBeInTheDocument();
  });

  it("copies the text, and selects it for copying by hand when the browser refuses", async () => {
    const writeText = vi.fn().mockResolvedValue(undefined);
    Object.defineProperty(navigator, "clipboard", { value: { writeText }, configurable: true });
    open();
    const team = (await screen.findByRole("textbox", { name: "Text for Team summary" })) as HTMLTextAreaElement;
    await userEvent.click(screen.getAllByRole("button", { name: "Copy" })[0]);
    expect(writeText).toHaveBeenCalledWith(team.value);
    expect(await screen.findByText("Copied.")).toBeInTheDocument();

    writeText.mockRejectedValue(new Error("denied"));
    await userEvent.click(screen.getAllByRole("button", { name: "Copy" })[0]);
    expect(await screen.findByText(/Copying did not work/)).toBeInTheDocument();
    expect(team).toHaveFocus();
  });

  it("never carries lyrics or Bible text, and warns about a link that points at this computer", async () => {
    open({ ...base, url: "http://localhost:8080/published/l1" });
    await screen.findByRole("textbox", { name: "Text for Team summary" });
    expect(screen.getByText(/points to this computer/)).toBeInTheDocument();
    for (const box of screen.getAllByRole("textbox")) {
      expect((box as HTMLTextAreaElement).value).not.toMatch(/Karena begitu besar|Setiap pagi/);
    }
  });

  it("says there is no comparison for a first version", async () => {
    open();
    expect(await screen.findByText(/first version, so there is nothing to compare/)).toBeInTheDocument();
  });

  it("offers the change summary for a later version", async () => {
    open({ ...base, changes: { items: [{ kind: "added", item_id: "x", title: "Penutup" }], songs: [], reading: [], assignments: [] } });
    const box = await screen.findByRole("textbox", { name: "Text for Change summary" });
    expect((box as HTMLTextAreaElement).value).toContain("+ Penutup ditambahkan");
  });

  it("says nothing changed when the changes are empty", async () => {
    open({ ...base, changes: { items: [], songs: [], reading: [], assignments: [] } });
    expect(await screen.findByText("Nothing the team needs to know changed.")).toBeInTheDocument();
    expect(screen.queryByRole("textbox", { name: "Text for Change summary" })).toBeNull();
  });

  it("uses the church's language for a liturgy language the app has no texts for", async () => {
    open({ ...base, liturgy: { ...base.liturgy, language: "zh-Hans" } }, { ...approver, church: { ...approver.church!, default_ui_language: "en" } });
    const team = (await screen.findByRole("textbox", { name: "Text for Team summary" })) as HTMLTextAreaElement;
    expect(team.value).toContain("*Liturgy Ibadah Umum*");
  });

  it("shows the refusal to a member without liturgy.approve", async () => {
    mockApi({ "GET /liturgies/l1/published/summary": { status: 403, body: { code: "forbidden", status: 403 } } });
    renderPage("/published/:id/messages", "/published/l1/messages", <MessagesPage />, meWith([]));
    expect(await screen.findByRole("alert")).toBeInTheDocument();
    expect(screen.queryByRole("textbox")).toBeNull();
  });
});

describe("the link to the messages", () => {
  const view = (me: ReturnType<typeof meWith>) => {
    mockApi({ "GET /liturgies/l1/published": { status: 200, body: copy() } });
    renderPage("/published/:id", "/published/l1", <PublishedPage />, me);
  };
  it("is shown to a member who can approve", async () => {
    view(approver);
    await waitFor(() => expect(screen.getByRole("link", { name: "Send to the team" })).toHaveAttribute("href", "/published/l1/messages"));
  });
  it("is not shown to anybody else", async () => {
    view(meWith(["liturgy.edit"]));
    await screen.findByRole("link", { name: "Print" });
    expect(screen.queryByRole("link", { name: "Send to the team" })).toBeNull();
  });
});
