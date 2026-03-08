import { html, nothing } from "lit";
import { t } from "../i18n.ts";
import { ref as litRef } from "lit/directives/ref.js";
import { unsafeHTML } from "lit/directives/unsafe-html.js";
import type { AssistantIdentity } from "../assistant-identity.ts";
import type { MessageGroup } from "../types/chat-types.ts";
import { formatTimeShort } from "../format.ts";
import { toSanitizedMarkdownHtml } from "../markdown.ts";
import { renderCopyAsMarkdownButton } from "./copy-as-markdown.ts";
import {
  extractTextCached,
  extractThinkingCached,
  formatReasoningMarkdown,
} from "./message-extract.ts";
import { isToolResultMessage, normalizeRoleForGrouping } from "./message-normalizer.ts";
import { extractToolCards, renderToolCardSidebar } from "./tool-cards.ts";
import { openLightbox } from "./image-lightbox.ts";
import { renderMermaidBlocks } from "./mermaid-renderer.ts";

type ImageBlock = {
  url: string;
  alt?: string;
};

function extractImages(message: unknown): ImageBlock[] {
  const m = message as Record<string, unknown>;
  const content = m.content;
  const images: ImageBlock[] = [];

  if (Array.isArray(content)) {
    for (const block of content) {
      if (typeof block !== "object" || block === null) {
        continue;
      }
      const b = block as Record<string, unknown>;

      if (b.type === "image") {
        // Handle source object format (from sendChatMessage)
        const source = b.source as Record<string, unknown> | undefined;
        if (source?.type === "base64" && typeof source.data === "string") {
          const data = source.data;
          const mediaType = (source.media_type as string) || "image/png";
          // If data is already a data URL, use it directly
          const url = data.startsWith("data:") ? data : `data:${mediaType};base64,${data}`;
          images.push({ url });
        } else if (typeof b.url === "string") {
          images.push({ url: b.url });
        }
      } else if (b.type === "image_url") {
        // OpenAI format
        const imageUrl = b.image_url as Record<string, unknown> | undefined;
        if (typeof imageUrl?.url === "string") {
          images.push({ url: imageUrl.url });
        }
      }
    }
  }

  return images;
}

type DocumentBlock = {
  fileName: string;
  fileSize?: number;
  mimeType?: string;
  url?: string;
};

type AudioBlock = {
  fileName: string;
  fileSize?: number;
  url?: string;
};

function extractDocuments(message: unknown): DocumentBlock[] {
  const m = message as Record<string, unknown>;
  const content = m.content;
  const docs: DocumentBlock[] = [];
  if (Array.isArray(content)) {
    for (const block of content) {
      if (typeof block !== "object" || block === null) continue;
      const b = block as Record<string, unknown>;
      if (b.type === "document") {
        docs.push({
          fileName: (b.fileName as string) ?? "document",
          fileSize: typeof b.fileSize === "number" ? b.fileSize : undefined,
          mimeType: (b.source as Record<string, unknown>)?.media_type as string | undefined,
          url: typeof b.url === "string" ? b.url : undefined,
        });
      }
    }
  }
  return docs;
}

type VideoBlock = {
  fileName: string;
  fileSize?: number;
  url?: string;
};

function extractVideoBlocks(message: unknown): VideoBlock[] {
  const m = message as Record<string, unknown>;
  const content = m.content;
  const blocks: VideoBlock[] = [];
  if (Array.isArray(content)) {
    for (const block of content) {
      if (typeof block !== "object" || block === null) continue;
      const b = block as Record<string, unknown>;
      if (b.type === "video") {
        const source = b.source as Record<string, unknown> | undefined;
        let url: string | undefined;
        if (source?.type === "base64" && typeof source.data === "string") {
          const data = source.data as string;
          const mediaType = (source.media_type as string) || "video/mp4";
          url = data.startsWith("data:") ? data : `data:${mediaType};base64,${data}`;
        } else if (typeof b.url === "string") {
          url = b.url;
        }
        blocks.push({
          fileName: (b.fileName as string) ?? "video",
          fileSize: typeof b.fileSize === "number" ? b.fileSize : undefined,
          url,
        });
      }
    }
  }
  return blocks;
}

function extractAudioBlocks(message: unknown): AudioBlock[] {
  const m = message as Record<string, unknown>;
  const content = m.content;
  const blocks: AudioBlock[] = [];
  if (Array.isArray(content)) {
    for (const block of content) {
      if (typeof block !== "object" || block === null) continue;
      const b = block as Record<string, unknown>;
      if (b.type === "audio") {
        const source = b.source as Record<string, unknown> | undefined;
        let url: string | undefined;
        if (source?.type === "base64" && typeof source.data === "string") {
          const data = source.data as string;
          const mediaType = (source.media_type as string) || "audio/webm";
          url = data.startsWith("data:") ? data : `data:${mediaType};base64,${data}`;
        } else if (typeof b.url === "string") {
          url = b.url;
        }
        blocks.push({
          fileName: (b.fileName as string) ?? "audio",
          fileSize: typeof b.fileSize === "number" ? b.fileSize : undefined,
          url,
        });
      }
    }
  }
  return blocks;
}

export function renderReadingIndicatorGroup(assistant?: AssistantIdentity) {
  return html`
    <div class="chat-group assistant">
      ${renderAvatar("assistant", assistant)}
      <div class="chat-group-messages">
        <div class="chat-bubble chat-reading-indicator" aria-hidden="true">
          <span class="chat-reading-indicator__dots">
            <span></span><span></span><span></span>
          </span>
        </div>
      </div>
    </div>
  `;
}

export function renderStreamingGroup(
  text: string,
  startedAt: number,
  onOpenSidebar?: (content: string) => void,
  assistant?: AssistantIdentity,
) {
  const timestamp = formatTimeShort(startedAt);
  const name = assistant?.name ?? t("chat.assistantName");

  return html`
    <div class="chat-group assistant">
      ${renderAvatar("assistant", assistant)}
      <div class="chat-group-messages">
        ${renderGroupedMessage(
    {
      role: "assistant",
      content: [{ type: "text", text }],
      timestamp: startedAt,
    },
    { isStreaming: true, showReasoning: false },
    onOpenSidebar,
  )}
        <div class="chat-group-footer">
          <span class="chat-sender-name">${name}</span>
          <span class="chat-group-timestamp">${timestamp}</span>
        </div>
      </div>
    </div>
  `;
}

export function renderMessageGroup(
  group: MessageGroup,
  opts: {
    onOpenSidebar?: (content: string) => void;
    showReasoning: boolean;
    assistantName?: string;
    assistantAvatar?: string | null;
  },
) {
  const normalizedRole = normalizeRoleForGrouping(group.role);
  const assistantName = opts.assistantName ?? t("chat.assistantName");
  const who =
    normalizedRole === "user"
      ? t("chat.you")
      : normalizedRole === "assistant"
        ? assistantName
        : normalizedRole;
  const roleClass =
    normalizedRole === "user" ? "user" : normalizedRole === "assistant" ? "assistant" : "other";
  const timestamp = formatTimeShort(group.timestamp);

  return html`
    <div class="chat-group ${roleClass}">
      ${renderAvatar(group.role, {
    name: assistantName,
    avatar: opts.assistantAvatar ?? null,
  })}
      <div class="chat-group-messages">
        ${group.messages.map((item, index) =>
    renderGroupedMessage(
      item.message,
      {
        isStreaming: group.isStreaming && index === group.messages.length - 1,
        showReasoning: opts.showReasoning,
      },
      opts.onOpenSidebar,
    ),
  )}
        <div class="chat-group-footer">
          <span class="chat-sender-name">${who}</span>
          <span class="chat-group-timestamp">${timestamp}</span>
        </div>
      </div>
    </div>
  `;
}

function renderAvatar(_role: string, _assistant?: Pick<AssistantIdentity, "name" | "avatar">) {
  return nothing;
}

function renderMessageImages(images: ImageBlock[]) {
  if (images.length === 0) {
    return nothing;
  }

  return html`
    <div class="chat-message-images">
      ${images.map(
    (img, index) => html`
          <img
            src=${img.url}
            alt=${img.alt ?? t("chat.attachedImage")}
            class="chat-message-image"
            @click=${() => openLightbox(images, index)}
          />
        `,
  )}
    </div>
  `;
}

function renderMessageDocuments(docs: DocumentBlock[]) {
  if (docs.length === 0) return nothing;
  return html`
    <div class="chat-message-documents">
      ${docs.map(
    (doc) => html`
          <div class="chat-message-document">
            <span class="chat-message-document__icon" aria-hidden="true">📄</span>
            <span class="chat-message-document__name">${doc.fileName}</span>
            ${doc.fileSize != null
        ? html`<span class="chat-message-document__size">${formatDocSize(doc.fileSize)}</span>`
        : nothing}
          </div>
        `,
  )}
    </div>
  `;
}

function renderMessageAudioBlocks(blocks: AudioBlock[]) {
  if (blocks.length === 0) return nothing;
  return html`
    <div class="chat-message-audio-list">
      ${blocks.map(
    (block) => html`
          <div class="chat-message-audio">
            <span class="chat-message-audio__icon" aria-hidden="true">🎤</span>
            <span class="chat-message-audio__name">${block.fileName}</span>
            ${block.url
        ? html`<audio controls preload="metadata" class="chat-message-audio__player"><source src="${block.url}" /></audio>`
        : nothing}
          </div>
        `,
  )}
    </div>
  `;
}

function renderMessageVideoBlocks(blocks: VideoBlock[]) {
  if (blocks.length === 0) return nothing;
  return html`
    <div class="chat-message-video-list">
      ${blocks.map(
    (block) => html`
          <div class="chat-message-video">
            ${block.url
        ? html`<video controls preload="metadata" class="chat-message-video__player"><source src="${block.url}" /></video>`
        : html`<span class="chat-message-video__name">${block.fileName}</span>`}
          </div>
        `,
  )}
    </div>
  `;
}

/** Format bytes (duplicated here to avoid cross-module dep, same logic) */
function formatDocSize(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`;
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`;
  return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
}

function renderGroupedMessage(
  message: unknown,
  opts: { isStreaming: boolean; showReasoning: boolean },
  onOpenSidebar?: (content: string) => void,
) {
  const m = message as Record<string, unknown>;
  const role = typeof m.role === "string" ? m.role : "unknown";
  const isToolResult =
    isToolResultMessage(message) ||
    role.toLowerCase() === "toolresult" ||
    role.toLowerCase() === "tool_result" ||
    typeof m.toolCallId === "string" ||
    typeof m.tool_call_id === "string";

  const toolCards = extractToolCards(message);
  const hasToolCards = toolCards.length > 0;
  const images = extractImages(message);
  const hasImages = images.length > 0;
  const documents = extractDocuments(message);
  const hasDocuments = documents.length > 0;
  const audioBlocks = extractAudioBlocks(message);
  const hasAudio = audioBlocks.length > 0;
  const videoBlocks = extractVideoBlocks(message);
  const hasVideo = videoBlocks.length > 0;

  const extractedText = extractTextCached(message);
  const extractedThinking =
    opts.showReasoning && role === "assistant" ? extractThinkingCached(message) : null;
  const markdownBase = extractedText?.trim() ? extractedText : null;
  const reasoningMarkdown = extractedThinking ? formatReasoningMarkdown(extractedThinking) : null;
  const markdown = markdownBase;
  const canCopyMarkdown = role === "assistant" && Boolean(markdown?.trim());

  const bubbleClasses = [
    "chat-bubble",
    canCopyMarkdown ? "has-copy" : "",
    opts.isStreaming ? "streaming" : "",
    "fade-in",
  ]
    .filter(Boolean)
    .join(" ");

  if (!markdown && hasToolCards && isToolResult) {
    return html`${toolCards.map((card) => renderToolCardSidebar(card, onOpenSidebar))}`;
  }

  if (!markdown && !hasToolCards && !hasImages && !hasDocuments && !hasAudio && !hasVideo) {
    return nothing;
  }

  return html`
    <div class="${bubbleClasses}">
      ${canCopyMarkdown ? renderCopyAsMarkdownButton(markdown!) : nothing}
      ${renderMessageImages(images)}
      ${renderMessageDocuments(documents)}
      ${renderMessageAudioBlocks(audioBlocks)}
      ${renderMessageVideoBlocks(videoBlocks)}
      ${reasoningMarkdown
      ? html`<div class="chat-thinking">${unsafeHTML(
        toSanitizedMarkdownHtml(reasoningMarkdown),
      )}</div>`
      : nothing
    }
      ${markdown
      ? html`<div class="chat-text" ${litRef((el) => {
        if (el) {
          requestAnimationFrame(() => renderMermaidBlocks(el as HTMLElement));
        }
      })}>${unsafeHTML(toSanitizedMarkdownHtml(markdown))}</div>`
      : nothing
    }
      ${toolCards.map((card) => renderToolCardSidebar(card, onOpenSidebar))}
    </div>
  `;
}

/** Format elapsed milliseconds to "M:SS" display string */
export function formatElapsed(ms: number): string {
  const totalSeconds = Math.max(0, Math.floor(ms / 1000));
  const minutes = Math.floor(totalSeconds / 60);
  const seconds = totalSeconds % 60;
  return `${minutes}:${seconds.toString().padStart(2, "0")}`;
}

/**
 * Render a processing card that wraps stream output or reading indicator.
 * Inspired by Gemini's task processing UX — shows a shimmer dot, elapsed time,
 * and the streaming content (or 3-dot animation when idle).
 */
export function renderProcessingCard(
  stream: string | null,
  startedAt: number,
  onOpenSidebar?: (content: string) => void,
  assistant?: AssistantIdentity,
) {
  const elapsed = Date.now() - startedAt;
  const elapsedStr = formatElapsed(elapsed);
  const hasContent = stream !== null && stream.trim().length > 0;

  return html`
    <div class="chat-group assistant">
      ${renderAvatar("assistant", assistant)}
      <div class="chat-group-messages">
        <div class="chat-processing-card ${!hasContent ? 'chat-processing-card--waiting' : ''}">
          <div class="chat-processing-card__content">
            <div class="chat-processing-card__icon-wrap" aria-hidden="true">
              <svg class="chat-processing-card__anim-icon" width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
                <circle cx="12" cy="12" r="7"></circle>
                <path d="M12 2v2M12 20v2M4.93 4.93l1.41 1.41M17.66 17.66l1.41 1.41M2 12h2M20 12h2M4.93 19.07l1.41-1.41M17.66 6.34l1.41-1.41"></path>
              </svg>
            </div>
            <div class="chat-processing-card__text-wrap">
              <span class="chat-processing-card__title">${t("chat.processing")}</span>
              <span class="chat-processing-card__timer">${t("chat.processingTime", { time: elapsedStr })}</span>
            </div>
            <div class="chat-processing-card__pulse-dots" aria-hidden="true">
              <span></span>
              <span></span>
              <span></span>
            </div>
          </div>
          ${hasContent
      ? html`
              <div class="chat-processing-card__stream">
                ${renderGroupedMessage(
        {
          role: "assistant",
          content: [{ type: "text", text: stream! }],
          timestamp: startedAt,
        },
        { isStreaming: true, showReasoning: false },
        onOpenSidebar,
      )}
              </div>
            `
      : nothing
    }
        </div>
      </div>
    </div>
  `;
}
