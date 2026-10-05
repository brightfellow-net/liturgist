// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0
import { afterEach, describe, expect, it, vi } from "vitest";
import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import i18n from "@/lib/i18n";
import { viewer } from "@/test/library";
import { copy } from "@/test/published";
import { mockApi, renderPage } from "@/test/render";
import { PublishedPage } from "./PublishedPage";

afterEach(() => {
  vi.unstubAllGlobals();
  Reflect.deleteProperty(navigator, "wakeLock");
});
void i18n.changeLanguage("en");

const published = () => ({ "GET /liturgies/l1/published": { status: 200, body: copy() } });
const read = (me = viewer) => renderPage("/published/:id", "/published/l1?read=1", <PublishedPage />, me);

// The viewer is user u1, who is Liturgis (duty d1) in the copy.
// WT-P-005
describe("reading mode", () => {
  it("marks the viewer's part in words, and moves focus to it", async () => {
    mockApi(published());
    read();
    const mine = await screen.findByRole("region", { name: "Doa Pembuka" });
    expect(within(mine).getByText("Your part")).toBeInTheDocument();
    expect(screen.getByRole("region", { name: "Pembacaan" }).textContent).not.toContain("Your part");
    await userEvent.click(screen.getByRole("button", { name: "Go to my part" }));
    expect(screen.getByRole("heading", { name: "Doa Pembuka" })).toHaveFocus();
  });

  it("has no 'Go to my part' and no marker for a viewer with no duty", async () => {
    mockApi(published());
    read({ ...viewer, user: { ...viewer.user, id: "u7" } });
    await screen.findByRole("region", { name: "Doa Pembuka" });
    expect(screen.queryByText("Your part")).toBeNull();
    expect(screen.queryByRole("button", { name: "Go to my part" })).toBeNull();
  });

  it("is a link away from the normal view, and back", async () => {
    mockApi(published());
    const router = renderPage("/published/:id", "/published/l1", <PublishedPage />, viewer);
    await userEvent.click(await screen.findByRole("link", { name: "Reading mode" }));
    expect(router.state.location.search).toBe("?read=1");
    expect(await screen.findByRole("link", { name: "Leave reading mode" })).toBeInTheDocument();
    expect(screen.queryByText("Your part")).not.toBeNull();
    // Outside reading mode nothing is marked.
    await userEvent.click(screen.getByRole("link", { name: "Leave reading mode" }));
    await screen.findByRole("link", { name: "Reading mode" });
    expect(screen.queryByText("Your part")).toBeNull();
  });

  it("hides 'Keep screen on' when the browser has no Wake Lock", async () => {
    mockApi(published());
    read();
    await screen.findByRole("region", { name: "Doa Pembuka" });
    expect(screen.queryByRole("switch", { name: "Keep screen on" })).toBeNull();
  });

  it("holds the lock while the switch is on, lets go when hidden, asks again when shown, and releases when off", async () => {
    const release = vi.fn().mockResolvedValue(undefined);
    const request = vi.fn().mockImplementation(async () => ({ release }));
    Object.defineProperty(navigator, "wakeLock", { value: { request }, configurable: true });
    mockApi(published());
    read();
    const sw = await screen.findByRole("switch", { name: "Keep screen on" });
    expect(request).not.toHaveBeenCalled();
    await userEvent.click(sw);
    await waitFor(() => expect(request).toHaveBeenCalledTimes(1));
    expect(request).toHaveBeenCalledWith("screen");

    const visibility = (state: "hidden" | "visible") => {
      Object.defineProperty(document, "visibilityState", { value: state, configurable: true });
      document.dispatchEvent(new Event("visibilitychange"));
    };
    visibility("hidden");
    await waitFor(() => expect(release).toHaveBeenCalledTimes(1));
    visibility("visible");
    await waitFor(() => expect(request).toHaveBeenCalledTimes(2));

    await userEvent.click(sw);
    await waitFor(() => expect(release).toHaveBeenCalledTimes(2));
    visibility("visible");
    expect(request).toHaveBeenCalledTimes(2);
  });

  it("says so when the lock is refused", async () => {
    Object.defineProperty(navigator, "wakeLock", { value: { request: vi.fn().mockRejectedValue(new Error("battery")) }, configurable: true });
    mockApi(published());
    read();
    await userEvent.click(await screen.findByRole("switch", { name: "Keep screen on" }));
    expect(await screen.findByText(/could not be kept on/)).toBeInTheDocument();
  });
});
