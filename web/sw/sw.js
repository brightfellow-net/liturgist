// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0

// The service worker of 13 §7. The build (vite.config.ts) fills in the two
// placeholders below, each of which appears once, and writes the result to
// dist/sw.js.
//
// Two caches, both named after the build, so a new build starts clean:
//   shell-<build>  the app's static files, precached; cache first.
//   pub-<build>    published views and "my assignments"; network first.
// Nothing else is cached: the lists, /me, /church and every other route go to
// the network untouched.

const BUILD = "__BUILD__";
const SHELL = "shell-" + BUILD;
const PUB = "pub-" + BUILD;
const SHELL_FILES = __SHELL__;

const NETWORK_TIMEOUT_MS = 4000;
const MAX_LITURGIES = 5; // [Q-5.4]

// Only these two answers are kept, and only without a query string.
const LITURGY = /\/api\/v1\/liturgies\/[^/]+\/published$/;
const ASSIGNMENTS = /\/api\/v1\/me\/assignments$/;

// Every clear (logout, 401, another user) increments the generation. A fetch
// that started before a clear must not store its answer afterwards.
let generation = 0;

self.addEventListener("install", (event) => {
  event.waitUntil(caches.open(SHELL).then((cache) => cache.addAll(SHELL_FILES)).then(() => self.skipWaiting()));
});

self.addEventListener("activate", (event) => {
  event.waitUntil(
    caches
      .keys()
      .then((names) => Promise.all(names.filter((n) => n !== SHELL && n !== PUB).map((n) => caches.delete(n))))
      .then(() => self.clients.claim()),
  );
});

self.addEventListener("message", (event) => {
  if (!event.data || event.data.type !== "clear") return;
  generation++;
  const done = clearPublished();
  event.waitUntil(done);
  done.then(() => event.ports && event.ports[0] && event.ports[0].postMessage("cleared"));
});

// clearPublished deletes every published-view cache, of any build. The shell
// holds only the app's public files and stays, so the login page still opens
// offline afterwards.
async function clearPublished() {
  const names = await caches.keys();
  await Promise.all(names.filter((n) => n.startsWith("pub-")).map((n) => caches.delete(n)));
}

self.addEventListener("fetch", (event) => {
  const request = event.request;
  if (request.method !== "GET") return;
  const url = new URL(request.url);
  if (url.origin !== self.location.origin) return;
  if (url.search === "" && (LITURGY.test(url.pathname) || ASSIGNMENTS.test(url.pathname))) {
    event.respondWith(publishedAnswer(event, request, url));
    return;
  }
  if (url.pathname.startsWith("/api/")) return;
  if (request.mode === "navigate") {
    event.respondWith(navigation(request));
    return;
  }
  if (SHELL_FILES.includes(url.pathname)) {
    event.respondWith(caches.open(SHELL).then((c) => c.match(url.pathname)).then((hit) => hit || fetch(request)));
  }
});

// navigation tries the network, so a new build is picked up, and falls back
// to the cached index page, which boots the app offline.
async function navigation(request) {
  try {
    return await fetch(request);
  } catch (err) {
    const hit = await (await caches.open(SHELL)).match("/");
    if (hit) return hit;
    throw err;
  }
}

// publishedAnswer is NetworkFirst with a timeout: the network answer when it
// comes within 4 seconds, the saved copy otherwise. A saved copy is marked
// with X-From-Cache: 1 so the page can say so (a cached answer is otherwise an
// ordinary 200).
async function publishedAnswer(event, request, url) {
  const started = generation;
  const key = url.pathname;
  const network = fetch(request).then(async (response) => {
    if (response.ok && started === generation) {
      // The cache is opened only now: one deleted by a clear in the meantime
      // must not be written to, and not be created again.
      const cache = await caches.open(PUB);
      await cache.delete(key); // so that a re-saved copy is the newest
      await cache.put(key, response.clone());
      await trim(cache);
    }
    return response;
  });
  const timeout = new Promise((resolve) => setTimeout(() => resolve(null), NETWORK_TIMEOUT_MS));
  try {
    const first = await Promise.race([network, timeout]);
    if (first) return first;
    const saved = await savedCopy(key);
    if (saved) {
      event.waitUntil(network.catch(() => undefined));
      return marked(saved);
    }
    return await network;
  } catch (err) {
    const saved = await savedCopy(key);
    if (saved) return marked(saved);
    throw err;
  }
}

async function savedCopy(key) {
  // caches.match would create nothing; open() on a missing cache would.
  if (!(await caches.keys()).includes(PUB)) return undefined;
  return (await caches.open(PUB)).match(key);
}

function marked(response) {
  const headers = new Headers(response.headers);
  headers.set("X-From-Cache", "1");
  return new Response(response.body, { status: response.status, statusText: response.statusText, headers });
}

// trim keeps the newest MAX_LITURGIES liturgies; "my assignments" does not count.
async function trim(cache) {
  const keys = (await cache.keys()).filter((r) => LITURGY.test(new URL(r.url).pathname));
  for (const old of keys.slice(0, Math.max(0, keys.length - MAX_LITURGIES))) await cache.delete(old);
}
