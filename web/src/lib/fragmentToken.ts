// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0

// readFragmentToken returns the token in "#t=…" and removes the fragment
// from the address bar and history (05 §5), or null if there is none.
export function readFragmentToken(): string | null {
  const params = new URLSearchParams(window.location.hash.slice(1));
  const token = params.get("t");
  if (window.location.hash !== "") {
    const { pathname, search } = window.location;
    window.history.replaceState(window.history.state, "", pathname + search);
  }
  return token ? token : null;
}
