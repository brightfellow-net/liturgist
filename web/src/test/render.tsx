// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0
import type { ReactElement } from "react";
import { render } from "@testing-library/react";
import { createMemoryRouter, Outlet } from "react-router";
import { RouterProvider } from "react-router/dom";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { vi } from "vitest";
import "@/lib/i18n";

type Reply = { status: number; body?: unknown; headers?: Record<string, string> };
type Handler = (body: unknown) => Reply;

// mockApi replaces fetch with fixed answers keyed by "METHOD /path" (without
// /api/v1) and returns the requests made, with their JSON bodies.
export function mockApi(routes: Record<string, Reply | Handler>) {
  const calls: { route: string; body: unknown }[] = [];
  vi.stubGlobal("fetch", async (request: Request) => {
    const route = request.method + " " + new URL(request.url).pathname.replace(/^\/api\/v1/, "");
    const text = await request.text();
    const body: unknown = text ? JSON.parse(text) : undefined;
    calls.push({ route, body });
    const r = routes[route];
    const reply = typeof r === "function" ? r(body) : r ?? { status: 404, body: { code: "not_found" } };
    return new Response(reply.body === undefined ? null : JSON.stringify(reply.body), {
      status: reply.status,
      headers: { "Content-Type": "application/json", ...reply.headers },
    });
  });
  return calls;
}

// renderPage renders one page at url, inside an outlet that provides
// context (as AppLayout provides /me), with a fresh query cache. The real
// address bar is set too, so pages can read "#t=" from it.
export function renderPage(path: string, url: string, page: ReactElement, context?: unknown) {
  window.history.replaceState(null, "", url);
  const router = createMemoryRouter(
    [
      { element: <Outlet context={context} />, children: [{ path, element: page }] },
      { path: "*", element: <p>other page</p> },
    ],
    { initialEntries: [url] },
  );
  const client = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
  render(<QueryClientProvider client={client}><RouterProvider router={router} /></QueryClientProvider>);
  return router;
}
