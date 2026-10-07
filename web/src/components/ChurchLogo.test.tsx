// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0
import { describe, expect, it } from "vitest";
import { fireEvent, render } from "@testing-library/react";
import { ChurchLogo } from "./ChurchLogo";

const img = (c: HTMLElement) => c.querySelector("img");

// WT-L-002
describe("ChurchLogo", () => {
  it("shows the image with an empty alt text (the name is next to it)", () => {
    const { container } = render(<ChurchLogo url="/api/v1/church/logo?v=aaaa" className="h-8" />);
    expect(img(container)).toHaveAttribute("src", "/api/v1/church/logo?v=aaaa");
    expect(img(container)).toHaveAttribute("alt", "");
  });

  it("shows nothing without an address", () => {
    expect(img(render(<ChurchLogo url={null} className="h-8" />).container)).toBeNull();
    expect(img(render(<ChurchLogo className="h-8" />).container)).toBeNull();
  });

  it("disappears when the file cannot be loaded, and is tried again for a new address", () => {
    const { container, rerender } = render(<ChurchLogo url="/api/v1/church/logo?v=aaaa" className="h-8" />);
    fireEvent.error(img(container)!);
    expect(img(container)).toBeNull();
    rerender(<ChurchLogo url="/api/v1/church/logo?v=bbbb" className="h-8" />);
    expect(img(container)).toHaveAttribute("src", "/api/v1/church/logo?v=bbbb");
  });
});
