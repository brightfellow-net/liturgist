// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0
import type { Middleware } from "openapi-fetch";

// Offline use (13 §7). The service worker keeps the last published views and
// "my assignments" of one user on this phone; this file holds the client side:
// the user marker, clearing, the "from the saved copy" flag, and registration.

const markerKey = "liturgist.user";

// userMarker is the ID of the user whose copies are on this phone, or null.
// localStorage can throw (private windows, blocked site data); then there is
// no marker and the app simply needs the network.
export function userMarker(): string | null {
  try {
    return localStorage.getItem(markerKey);
  } catch {
    return null;
  }
}

function setMarker(id: string | null) {
  try {
    if (id === null) localStorage.removeItem(markerKey);
    else localStorage.setItem(markerKey, id);
  } catch {
    // Nothing to do: the app works without the marker.
  }
}

const clearWaitMs = 1000;

// clearOffline deletes the saved copies and the marker. It never fails and
// never waits long, so logging out works offline too. The worker is told first
// (it bumps its generation, so a fetch still in flight is not stored), and the
// caches are deleted here as well, which also covers a page with no worker.
export async function clearOffline(): Promise<void> {
  setMarker(null);
  try {
    const worker = navigator.serviceWorker?.controller;
    if (worker) {
      await new Promise<void>((resolve) => {
        const channel = new MessageChannel();
        channel.port1.onmessage = () => resolve();
        setTimeout(resolve, clearWaitMs);
        worker.postMessage({ type: "clear" }, [channel.port2]);
      });
    }
  } catch {
    // Fall through to deleting the caches directly.
  }
  try {
    const names = await caches.keys();
    await Promise.all(names.filter((n) => n.startsWith("pub-")).map((n) => caches.delete(n)));
  } catch {
    // No Cache API here.
  }
}

// rememberUser records who is logged in. Copies saved for a different user are
// deleted first.
export async function rememberUser(id: string): Promise<void> {
  const before = userMarker();
  if (before === id) return;
  if (before !== null) await clearOffline();
  setMarker(id);
}

// savedCopies lists the paths whose last answer came from the saved copy.
const savedCopies = new Set<string>();

// fromSavedCopy tells whether the last answer for a path was a saved copy.
export function fromSavedCopy(path: string): boolean {
  return savedCopies.has(path);
}

// trackSavedCopies notes which answers carry X-From-Cache (set by the worker).
export const trackSavedCopies: Middleware = {
  onResponse({ request, response }) {
    const path = new URL(request.url, window.location.href).pathname.replace(/^\/api\/v1/, "");
    if (response.headers.get("X-From-Cache") === "1") savedCopies.add(path);
    else savedCopies.delete(path);
    return undefined;
  },
};

// registerWorker starts the service worker. Only in a production build: in
// development a worker would serve stale files.
export function registerWorker() {
  if (!import.meta.env.PROD || !("serviceWorker" in navigator)) return;
  window.addEventListener("load", () => {
    navigator.serviceWorker.register("/sw.js").catch(() => undefined);
  });
}
