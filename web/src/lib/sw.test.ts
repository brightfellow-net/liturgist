// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0
import { readFileSync } from "node:fs";
import { resolve } from "node:path";
import { runInNewContext } from "node:vm";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

// WT-P-007 (worker part): the service worker of 13 §7 is run with a fake
// cache store, fake fetch and fake events.
const source = readFileSync(resolve(process.cwd(), "sw/sw.js"), "utf8")
  .replace("__BUILD__", "b1")
  .replace("__SHELL__", JSON.stringify(["/", "/assets/app-1.js"]));

const origin = "https://church.test";
type Listener = (event: unknown) => void;

function setup(fetchImpl: (req: Request) => Promise<Response>) {
  const listeners: Record<string, Listener> = {};
  const store = new Map<string, Map<string, Response>>();
  const pathOf = (k: string | { url: string }) => (typeof k === "string" ? k : new URL(k.url).pathname);
  const cacheOf = (name: string) => {
    if (!store.has(name)) store.set(name, new Map());
    const m = store.get(name)!;
    return {
      put: async (k: string, r: Response) => void m.set(pathOf(k), r),
      match: async (k: string) => m.get(pathOf(k))?.clone(),
      delete: async (k: string | { url: string }) => m.delete(pathOf(k)),
      keys: async () => [...m.keys()].map((p) => ({ url: origin + p })),
      addAll: async (urls: string[]) => urls.forEach((u) => m.set(u, new Response("shell " + u))),
    };
  };
  const caches = {
    open: async (n: string) => cacheOf(n),
    keys: async () => [...store.keys()],
    delete: async (n: string) => store.delete(n),
  };
  const self = {
    location: { origin },
    addEventListener: (type: string, fn: Listener) => void (listeners[type] = fn),
    skipWaiting: () => Promise.resolve(),
    clients: { claim: () => Promise.resolve() },
  };
  runInNewContext(source, { self, caches, fetch: fetchImpl, Response, Headers, Request, URL, setTimeout, Promise });
  const fetched = (path: string, init: { method?: string; mode?: string } = {}) => {
    const out: { response?: Promise<Response> } = {};
    const event = {
      request: { method: init.method ?? "GET", url: origin + path, mode: init.mode ?? "cors" },
      respondWith: (p: Promise<Response>) => void (out.response = p),
      waitUntil: () => undefined,
    };
    listeners.fetch(event);
    return out;
  };
  const pub = (name = "pub-b1") => store.get(name);
  return { listeners, store, fetched, pub };
}

const answer = (body: unknown, status = 200) => new Response(JSON.stringify(body), { status, headers: { "Content-Type": "application/json" } });
const lit = (id: string) => `/api/v1/liturgies/${id}/published`;

beforeEach(() => vi.useRealTimers());
afterEach(() => vi.useRealTimers());

describe("service worker", () => {
  it("keeps only the published view of a liturgy and my assignments", async () => {
    const w = setup(async () => answer({ ok: 1 }));
    for (const path of ["/api/v1/published", "/api/v1/me", "/api/v1/church", "/api/v1/published?archived=all", lit("a") + "?x=1", "/api/v1/liturgies/a"]) {
      const out = w.fetched(path);
      expect(out.response, path).toBeUndefined();
    }
    expect(w.fetched("/api/v1/me/assignments", { method: "POST" }).response).toBeUndefined();
    await w.fetched(lit("a")).response;
    await w.fetched("/api/v1/me/assignments").response;
    expect([...w.pub()!.keys()].sort()).toEqual(["/api/v1/liturgies/a/published", "/api/v1/me/assignments"]);
  });

  it("answers from the network when it can, and from the saved copy marked when it cannot", async () => {
    let online = true;
    const w = setup(async () => {
      if (!online) throw new TypeError("offline");
      return answer({ number: 3 });
    });
    const live = await w.fetched(lit("a")).response!;
    expect(live.headers.get("X-From-Cache")).toBeNull();
    online = false;
    const saved = await w.fetched(lit("a")).response!;
    expect(saved.status).toBe(200);
    expect(saved.headers.get("X-From-Cache")).toBe("1");
    expect(await saved.json()).toEqual({ number: 3 });
    // Nothing saved for this liturgy: the failure is not hidden.
    await expect(w.fetched(lit("b")).response!).rejects.toThrow("offline");
  });

  it("falls back to the saved copy after 4 seconds", async () => {
    vi.useFakeTimers();
    let slow = false;
    const w = setup(() => (slow ? new Promise<Response>(() => undefined) : Promise.resolve(answer({ number: 1 }))));
    await w.fetched(lit("a")).response;
    slow = true;
    const pending = w.fetched(lit("a")).response!;
    await vi.advanceTimersByTimeAsync(3900);
    let settled = false;
    void pending.then(() => (settled = true));
    await vi.advanceTimersByTimeAsync(0);
    expect(settled).toBe(false);
    await vi.advanceTimersByTimeAsync(200);
    expect((await pending).headers.get("X-From-Cache")).toBe("1");
  });

  it("does not save a refusal, and returns it as it is", async () => {
    const w = setup(async () => answer({ code: "unauthenticated" }, 401));
    const res = await w.fetched(lit("a")).response!;
    expect(res.status).toBe(401);
    expect(w.pub()?.size ?? 0).toBe(0);
  });

  it("keeps the newest five liturgies, and my assignments besides", async () => {
    const w = setup(async () => answer({}));
    await w.fetched("/api/v1/me/assignments").response;
    for (const id of ["a", "b", "c", "d", "e", "f"]) await w.fetched(lit(id)).response;
    await w.fetched(lit("b")).response; // read again: now the newest
    await w.fetched(lit("g")).response;
    expect([...w.pub()!.keys()].sort()).toEqual(
      ["/api/v1/liturgies/b/published", "/api/v1/liturgies/d/published", "/api/v1/liturgies/e/published", "/api/v1/liturgies/f/published", "/api/v1/liturgies/g/published", "/api/v1/me/assignments"].sort(),
    );
  });

  it("deletes the saved copies on 'clear' but not the shell, and ignores a late answer", async () => {
    let release: (r: Response) => void = () => undefined;
    let first = true;
    const w = setup(() => {
      if (first) {
        first = false;
        return Promise.resolve(answer({ n: 1 }));
      }
      return new Promise<Response>((resolve) => void (release = resolve));
    });
    await w.listeners.install({ waitUntil: (p: Promise<unknown>) => p });
    await w.fetched(lit("a")).response;
    expect(w.pub()!.size).toBe(1);
    const late = w.fetched(lit("b")).response!; // in flight when the user logs out
    await vi.waitFor(() => expect(release).not.toBe(undefined));
    let done: Promise<unknown> = Promise.resolve();
    w.listeners.message({ data: { type: "clear" }, ports: [], waitUntil: (p: Promise<unknown>) => (done = p) });
    await done;
    expect(w.store.has("pub-b1")).toBe(false);
    expect(w.store.has("shell-b1")).toBe(true);
    release(answer({ n: 2 }));
    await late;
    await vi.waitFor(() => undefined);
    expect(w.store.get("pub-b1")?.size ?? 0).toBe(0);
  });

  it("drops the caches of an older build when it starts", async () => {
    const w = setup(async () => answer({}));
    w.store.set("shell-b0", new Map());
    w.store.set("pub-b0", new Map());
    await w.listeners.install({ waitUntil: (p: Promise<unknown>) => p });
    let done: Promise<unknown> = Promise.resolve();
    w.listeners.activate({ waitUntil: (p: Promise<unknown>) => (done = p) });
    await done;
    expect([...w.store.keys()]).toEqual(["shell-b1"]);
  });

  it("opens a page offline from the cached index, and serves app files from the shell", async () => {
    let online = true;
    const w = setup(async () => {
      if (!online) throw new TypeError("offline");
      return new Response("network");
    });
    await w.listeners.install({ waitUntil: (p: Promise<unknown>) => p });
    expect(await (await w.fetched("/published/x", { mode: "navigate" }).response!).text()).toBe("network");
    online = false;
    expect(await (await w.fetched("/published/x", { mode: "navigate" }).response!).text()).toBe("shell /");
    expect(await (await w.fetched("/assets/app-1.js").response!).text()).toBe("shell /assets/app-1.js");
    expect(w.fetched("/assets/other.js").response).toBeUndefined();
  });
});
