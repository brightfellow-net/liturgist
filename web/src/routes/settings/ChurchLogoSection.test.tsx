// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0
import { afterEach, describe, expect, it, vi } from "vitest";
import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useQuery } from "@tanstack/react-query";
import { meQuery } from "@/lib/queries";
import i18n from "@/lib/i18n";
import { mockApi, renderPage } from "@/test/render";
import { ChurchSettingsPage } from "./ChurchSettingsPage";

const church = {
  id: "c1", name: "GKY Uji", default_ui_language: "id", default_language: "id", default_translation_code: "TB",
  time_zone: "Asia/Jakarta", key_display: "do", feedback_url: null, privacy_contact: null, actions: { edit: true },
  show_credits: true, licence_footer: "", logo_url: null,
  print: { paper: "a4", lyrics: "full", readings: true, assignments: true, keys: true, notes: true, size: "normal" },
};
const withLogo = { ...church, logo_url: "/api/v1/church/logo?v=0123456789abcdef" };
const translations = { "GET /translations": { status: 200, body: [{ code: "TB", name: "Terjemahan Baru", language: "id" }] } };

afterEach(() => vi.unstubAllGlobals());
void i18n.changeLanguage("en");

const png = (name = "logo.png", type = "image/png", size = 10) => new File([new Uint8Array(size).fill(7)], name, { type });
const page = () => renderPage("/settings/church", "/settings/church", <ChurchSettingsPage />);

// MeLogo stands for the app header, which reads the logo from GET /me.
function MeLogo() {
  const me = useQuery(meQuery);
  return <p data-testid="me-logo">{me.data?.church?.logo_url ?? "none"}</p>;
}

// WT-L-001
describe("ChurchLogoSection", () => {
  it("is shown only with actions.edit", async () => {
    mockApi({ "GET /church": { status: 200, body: { ...church, actions: { edit: false } } }, ...translations });
    page();
    expect(await screen.findByLabelText("Church name")).toBeDisabled();
    expect(screen.queryByText("Choose image")).toBeNull();
  });

  it("says there is no logo, then uploads one as base64 and shows it", async () => {
    const calls = mockApi({
      "GET /church": { status: 200, body: church }, ...translations,
      "PUT /church/logo": { status: 200, body: withLogo },
    });
    page();
    expect(await screen.findByText(/No logo yet/)).toBeInTheDocument();
    await userEvent.upload(screen.getByLabelText("Choose image"), png("logo.png", "image/png", 3));
    const shown = await screen.findByRole("img", { name: "The current logo" });
    expect(shown).toHaveAttribute("src", withLogo.logo_url);
    expect(calls.find((c) => c.route === "PUT /church/logo")?.body).toEqual({ image: btoa("\x07\x07\x07") });
    expect(screen.getByRole("button", { name: "Remove logo" })).toBeInTheDocument();
  });

  it("refuses a file over 2 MB or of another type before sending anything", async () => {
    const calls = mockApi({ "GET /church": { status: 200, body: church }, ...translations });
    page();
    await screen.findByText(/No logo yet/);
    await userEvent.upload(screen.getByLabelText("Choose image"), png("big.png", "image/png", 2 * 1024 * 1024 + 1));
    expect(await screen.findByText("The image is larger than 2 MB. Choose a smaller one.")).toBeInTheDocument();
    // The file chooser's own accept filter would hide a GIF from user-event, so switch it off here.
    await userEvent.setup({ applyAccept: false }).upload(screen.getByLabelText("Choose image"), png("a.gif", "image/gif"));
    expect(await screen.findByText("Choose a PNG, JPEG or WebP image.")).toBeInTheDocument();
    expect(calls.filter((c) => c.route === "PUT /church/logo")).toHaveLength(0);
  });

  it("explains an image the server refused, in words and not as a code", async () => {
    mockApi({
      "GET /church": { status: 200, body: church }, ...translations,
      "PUT /church/logo": { status: 422, body: { code: "validation_failed", errors: [{ location: "body.image", message: "Use a PNG, JPEG or WebP image." }] } },
    });
    page();
    await screen.findByText(/No logo yet/);
    await userEvent.upload(screen.getByLabelText("Choose image"), png());
    expect(await screen.findByText(/could not use this image/)).toBeInTheDocument();
    expect(screen.queryByRole("img", { name: "The current logo" })).toBeNull();
  });

  it("removes the logo", async () => {
    const calls = mockApi({
      "GET /church": { status: 200, body: withLogo }, ...translations,
      "DELETE /church/logo": { status: 200, body: church },
    });
    page();
    await userEvent.click(await screen.findByRole("button", { name: "Remove logo" }));
    await waitFor(() => expect(screen.getByText(/No logo yet/)).toBeInTheDocument());
    expect(calls.some((c) => c.route === "DELETE /church/logo")).toBe(true);
    expect(screen.queryByRole("button", { name: "Remove logo" })).toBeNull();
  });

  it("refreshes /me after a change, so that the header follows", async () => {
    let asked = 0;
    mockApi({
      "GET /church": { status: 200, body: church }, ...translations,
      "GET /me": () => ({ status: 200, body: { user: { id: "u1" }, church: { name: "GKY Uji", logo_url: asked++ === 0 ? null : withLogo.logo_url }, membership: null } }),
      "PUT /church/logo": { status: 200, body: withLogo },
    });
    renderPage("/settings/church", "/settings/church", <><ChurchSettingsPage /><MeLogo /></>);
    expect(await screen.findByTestId("me-logo")).toHaveTextContent("none");
    await userEvent.upload(await screen.findByLabelText("Choose image"), png());
    await waitFor(() => expect(screen.getByTestId("me-logo")).toHaveTextContent(withLogo.logo_url));
  });
});
