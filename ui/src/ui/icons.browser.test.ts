import { render } from "lit";
import { describe, expect, it } from "vitest";
import { icons } from "./icons.ts";

describe("navigation accent icons", () => {
  it("render SVG child nodes in the SVG namespace", () => {
    const container = document.createElement("div");
    render(icons.chatSpark, container);

    const svg = container.querySelector("svg");
    const path = svg?.querySelector("path");
    const circle = svg?.querySelector("circle");

    expect(svg).not.toBeNull();
    expect(path).not.toBeNull();
    expect(circle).not.toBeNull();
    expect(svg?.namespaceURI).toBe("http://www.w3.org/2000/svg");
    expect(path?.namespaceURI).toBe("http://www.w3.org/2000/svg");
    expect(circle?.namespaceURI).toBe("http://www.w3.org/2000/svg");
  });
});
