// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import i18n from "@/lib/i18n";
import { clearOffline, rememberUser, userMarker } from "@/lib/offline";
import { meWith, viewer } from "@/test/library";
import { copy } from "@/test/published";
import { mockApi, renderPage } from "@/test/render";
import { AppLayout } from "./AppLayout";
import { HomePage } from "./HomePage";
import { PublishedPage } from "./published/PublishedPage";

void i18n.changeLanguage("en");

const names = (store: Map<string, unknown>) => ({
  keys: async () => [...store.keys()],
  delete: async (n: string) => store.delete(n),
});

beforeEach(() => localStorage.clear());
afterEach(() => {
  vi.unstubAllGlobals();
  Reflect.deleteProperty(navigator.serviceWorker ?? {}, "controller");
});

// WT-P-007
describe("saved copies", () => {
  const fromCache = { "X-From-Cache": "1" };

  it("says so, with the date, when the worker answered from the saved copy", async () => {
    mockApi({ "GET /liturgies/l1/published": { status: 200, body: copy(), headers: fromCache } });
    renderPage("/published/:id", "/published/l1", <PublishedPage />, viewer);
    expect(await screen.findByText(/^Offline — showing the last saved copy, published .*2026/)).toBeInTheDocument();
  });

  it("says nothing for a live answer, and stops saying it once one arrives", async () => {
    mockApi({ "GET /liturgies/l1/published": { status: 200, body: copy() } });
    renderPage("/published/:id", "/published/l1", <PublishedPage />, viewer);
    await screen.findByRole("heading", { level: 1, name: "Ibadah Umum" });
    expect(screen.queryByText(/Offline/)).toBeNull();
  });

  it("marks 'my assignments' too, without a date", async () => {
    mockApi({ "GET /me/assignments": { status: 200, body: { items: [], more: false }, headers: fromCache } });
    renderPage("/", "/", <HomePage />, viewer);
    expect(await screen.findByText("Offline — showing the last saved copy.")).toBeInTheDocument();
  });

  it("clears the worker's copies and the marker, and does not fail without a worker or storage", async () => {
    const store = new Map<string, unknown>([["pub-b1", 1], ["pub-b0", 1], ["shell-b1", 1]]);
    vi.stubGlobal("caches", names(store));
    localStorage.setItem("liturgist.user", "u1");
    await clearOffline();
    expect([...store.keys()]).toEqual(["shell-b1"]);
    expect(userMarker()).toBeNull();

    vi.stubGlobal("caches", undefined);
    vi.spyOn(Storage.prototype, "getItem").mockImplementation(() => {
      throw new Error("blocked");
    });
    await expect(clearOffline()).resolves.toBeUndefined();
    expect(userMarker()).toBeNull();
    vi.restoreAllMocks();
  });

  it("tells the worker to clear, and waits for its answer", async () => {
    const posted: unknown[] = [];
    Object.defineProperty(navigator, "serviceWorker", {
      configurable: true,
      value: {
        controller: {
          postMessage: (m: unknown, ports: MessagePort[]) => {
            posted.push(m);
            ports[0].postMessage("cleared");
          },
        },
      },
    });
    vi.stubGlobal("caches", names(new Map()));
    await clearOffline();
    expect(posted).toEqual([{ type: "clear" }]);
    Reflect.deleteProperty(navigator, "serviceWorker");
  });

  it("clears when a different user logs in, and keeps the copies of the same user", async () => {
    const store = new Map<string, unknown>([["pub-b1", 1]]);
    vi.stubGlobal("caches", names(store));
    await rememberUser("u1");
    expect(userMarker()).toBe("u1");
    await rememberUser("u1");
    expect(store.size).toBe(1);
    await rememberUser("u2");
    expect(userMarker()).toBe("u2");
    expect(store.size).toBe(0);
  });
});

describe("AppLayout and the saved copies", () => {
  const me = meWith([]);

  it("logs out offline: the copies go first, whatever the server answers", async () => {
    const store = new Map<string, unknown>([["pub-b1", 1]]);
    vi.stubGlobal("caches", names(store));
    localStorage.setItem("liturgist.user", "u1");
    const seenAtLogout: (string | null)[] = [];
    vi.stubGlobal("fetch", async (request: Request) => {
      if (request.method === "POST") {
        seenAtLogout.push(userMarker());
        throw new TypeError("offline");
      }
      return new Response(JSON.stringify(me), { status: 200, headers: { "Content-Type": "application/json" } });
    });
    const router = renderPage("/", "/", <AppLayout />);
    await userEvent.click(await screen.findByRole("button", { name: "Log out" }));
    await waitFor(() => expect(router.state.location.pathname).toBe("/login"));
    expect(seenAtLogout).toEqual([null]);
    expect(store.size).toBe(0);
  });

  it("shows only 'My assignments' when the server cannot be reached and a user's copies are here", async () => {
    localStorage.setItem("liturgist.user", "u1");
    vi.stubGlobal("fetch", async (request: Request) => {
      if (new URL(request.url).pathname.endsWith("/me/assignments")) {
        return new Response(JSON.stringify({ items: [], more: false }), { status: 200, headers: { "Content-Type": "application/json", "X-From-Cache": "1" } });
      }
      throw new TypeError("offline");
    });
    renderPage("/", "/", <AppLayout />);
    expect(await screen.findByRole("link", { name: "My assignments" })).toBeInTheDocument();
    expect(screen.queryByRole("link", { name: "Library" })).toBeNull();
    expect(screen.getByRole("button", { name: "Log out" })).toBeInTheDocument();
  });

  it("shows the error, not saved copies, when the server cannot be reached and nobody has copies here", async () => {
    vi.stubGlobal("fetch", async () => {
      throw new TypeError("offline");
    });
    renderPage("/", "/", <AppLayout />);
    expect(await screen.findByRole("button", { name: "Try again" })).toBeInTheDocument();
    expect(screen.queryByRole("link", { name: "My assignments" })).toBeNull();
  });
});
