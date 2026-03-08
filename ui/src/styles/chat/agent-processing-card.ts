import { LitElement, html, css } from "lit";
import { customElement, property, state } from "lit/decorators.js";
import { t } from "../i18n.ts";
import { formatElapsed } from "./grouped-render.ts";
import { ref } from "lit/directives/ref.js";

@customElement("agent-processing-card")
export class AgentProcessingCard extends LitElement {
    @property({ type: String }) stream: string | null = null;
    @property({ type: Number }) startedAt: number = Date.now();

    @state() private elapsedStr: string = "0:00";

    private _timer: number | null = null;

    // Since we use global styles for markdown and chat components,
    // we do not use shadow DOM here so it can inherit components.css
    createRenderRoot() {
        return this;
    }

    connectedCallback() {
        super.connectedCallback();
        this.updateElapsed();
        this._timer = window.setInterval(() => {
            this.updateElapsed();
        }, 1000);
    }

    disconnectedCallback() {
        super.disconnectedCallback();
        if (this._timer !== null) {
            window.clearInterval(this._timer);
            this._timer = null;
        }
    }

    private updateElapsed() {
        const elapsed = Date.now() - this.startedAt;
        this.elapsedStr = formatElapsed(elapsed);
    }

    render() {
        const hasContent = this.stream !== null && this.stream.trim().length > 0;

        let processingTitle = t("chat.processing") || "任务处理中";
        if (hasContent) {
            if (this.stream!.includes("read_url") || this.stream!.includes("search")) processingTitle = "正在搜索...";
            else if (this.stream!.includes("run_command")) processingTitle = "正在执行命令...";
            else if (this.stream!.includes("code")) processingTitle = "正在编写代码...";
            else processingTitle = "正在思考...";
        }

        return html`
      <div class="chat-processing-card gemini-style">
        <div class="chat-processing-card__content">
          <div class="chat-processing-card__icon-wrap">
            <svg class="chat-processing-card__anim-icon" viewBox="0 0 24 24" width="24" height="24" stroke="currentColor" stroke-width="2" fill="none" stroke-linecap="round" stroke-linejoin="round">
              <path d="M12 2v4m0 12v4M4.93 4.93l2.83 2.83m8.48 8.48l2.83 2.83M2 12h4m12 0h4M4.93 19.07l2.83-2.83m8.48-8.48l2.83-2.83" />
            </svg>
          </div>
          <div class="chat-processing-card__text-wrap">
            <div class="chat-processing-card__title">${processingTitle}</div>
            <div class="chat-processing-card__subtitle">这一过程可能需要一些时间 (${this.elapsedStr})</div>
          </div>
        </div>
        ${hasContent
                ? html`
              <div class="chat-processing-card__stream">
                <div class="chat-bubble">
                  ${this.stream}
                </div>
              </div>
            `
                : ""}
      </div>
    `;
    }
}

declare global {
    interface HTMLElementTagNameMap {
        "agent-processing-card": AgentProcessingCard;
    }
}
