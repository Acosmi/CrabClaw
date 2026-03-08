import { render } from "lit";
import { describe, expect, it, vi } from "vitest";
import type { AppViewState } from "../app-view-state.ts";
import {
  renderDraftDetailModal,
  renderDraftEditModal,
  renderPublishDetailModal,
} from "./media-dashboard-modals.ts";

function createState(): AppViewState {
  return {
    requestUpdate: vi.fn(),
    mediaDraftEdit: {
      id: "draft-1",
      title: "Launch Plan",
      body: "Ship the new media workflow.",
      platform: "xiaohongshu",
      style: "clean",
      status: "draft",
      created_at: "2026-03-08T10:00:00.000Z",
      updated_at: "2026-03-08T12:00:00.000Z",
      tags: ["launch", "product"],
      images: [],
    },
    mediaDraftDetail: {
      id: "draft-2",
      title: "Weekly Recap",
      body: "A full recap for the week.",
      platform: "wechat",
      style: "briefing",
      status: "approved",
      created_at: "2026-03-07T10:00:00.000Z",
      updated_at: "2026-03-07T12:00:00.000Z",
      tags: ["weekly", "ops"],
      images: ["https://example.com/cover.png"],
    },
    mediaDraftDetailLoading: false,
    mediaPublishDetail: {
      id: "publish-1",
      draft_id: "draft-2",
      title: "Weekly Recap",
      platform: "wechat",
      post_id: "post-9",
      url: "https://example.com/post-9",
      status: "published",
      published_at: "2026-03-08T13:00:00.000Z",
    },
    mediaPublishDetailLoading: false,
  } as AppViewState;
}

describe("media dashboard modals", () => {
  it("keeps the draft edit footer outside the tags label", () => {
    const state = createState();
    const container = document.createElement("div");

    render(renderDraftEditModal(state), container);

    const fields = Array.from(container.querySelectorAll<HTMLElement>("label.media-dashboard-field"));
    const footer = container.querySelector<HTMLElement>("footer.media-dashboard-modal__footer");

    expect(fields).toHaveLength(4);
    expect(footer).not.toBeNull();
    expect(footer?.closest("label.media-dashboard-field")).toBeNull();
  });

  it("closes the draft edit modal and requests a rerender", () => {
    const state = createState();
    const container = document.createElement("div");

    render(renderDraftEditModal(state), container);

    const closeButton = container.querySelector<HTMLButtonElement>("button[aria-label='Close']");
    closeButton?.click();

    expect(state.mediaDraftEdit).toBeNull();
    expect(state.requestUpdate).toHaveBeenCalledTimes(1);
  });

  it("renders draft detail content with chips, tags, and images", () => {
    const state = createState();
    const container = document.createElement("div");

    render(renderDraftDetailModal(state), container);

    expect(container.textContent).toContain("Weekly Recap");
    expect(container.querySelectorAll(".media-dashboard-flow__step")).toHaveLength(4);
    expect(container.querySelectorAll(".media-dashboard-tag-list .pill")).toHaveLength(2);
    expect(container.querySelectorAll("img.media-dashboard-modal__image")).toHaveLength(1);
  });

  it("renders publish detail metadata and keeps the external link intact", () => {
    const state = createState();
    const container = document.createElement("div");

    render(renderPublishDetailModal(state), container);

    const link = container.querySelector<HTMLAnchorElement>("a.media-dashboard-row__link");

    expect(container.textContent).toContain("Post ID");
    expect(link?.getAttribute("href")).toBe("https://example.com/post-9");
    expect(container.textContent).toContain("draft-2");
  });
});
