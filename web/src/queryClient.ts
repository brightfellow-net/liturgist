// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0
import { MutationCache, QueryCache, QueryClient } from "@tanstack/react-query";
import { ApiError, isCode } from "./lib/errors";
import { clearOffline } from "./lib/offline";
import { router } from "./router";
import { loginWithNext, paths } from "./routes/paths";

// onError sends the user to the login page when the session has ended, and
// to the setup page before setup (05 §4). Queries on public pages opt out
// with meta: { public: true }.
function onError(err: unknown, isPublic: boolean) {
  if (isPublic) return;
  const here = router.state.location;
  if (isCode(err, "unauthenticated") && here.pathname !== paths.login) {
    void clearOffline();
    queryClient.clear();
    void router.navigate(loginWithNext(here.pathname + here.search), { replace: true });
  } else if (isCode(err, "not_set_up")) {
    void router.navigate(paths.setup, { replace: true });
  }
}

// Reads are retried twice, but not after a 4xx answer (05 §10), and not at
// all while the phone knows it is offline (13 §7).
function retry(failures: number, err: unknown) {
  return navigator.onLine !== false && failures < 2 && !(err instanceof ApiError && err.status < 500);
}

export const queryClient = new QueryClient({
  defaultOptions: { queries: { retry }, mutations: { retry: false } },
  queryCache: new QueryCache({ onError: (err, query) => onError(err, query.meta?.public === true) }),
  mutationCache: new MutationCache({ onError: (err, _v, _c, mutation) => onError(err, mutation.meta?.public === true) }),
});
