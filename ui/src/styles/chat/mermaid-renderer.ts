let mermaidModule: typeof import("mermaid") | null = null;
let mermaidLoading: Promise<typeof import("mermaid")> | null = null;
let idCounter = 0;

function isDarkTheme(): boolean {
  return document.documentElement.getAttribute("data-theme") !== "light";
}

async function loadMermaid(): Promise<typeof import("mermaid")> {
  if (mermaidModule) return mermaidModule;
  if (mermaidLoading) return mermaidLoading;

  mermaidLoading = import("mermaid").then((mod) => {
    mermaidModule = mod;
    mod.default.initialize({
      startOnLoad: false,
      theme: isDarkTheme() ? "dark" : "default",
      securityLevel: "strict",
    });
    return mod;
  });

  return mermaidLoading;
}

export async function renderMermaidBlocks(container: HTMLElement) {
  const codeBlocks = container.querySelectorAll<HTMLElement>(
    'code.language-mermaid',
  );
  if (codeBlocks.length === 0) return;

  // Collect blocks that haven't been rendered yet
  const pending: Array<{ wrapper: HTMLElement; code: string }> = [];
  for (const codeEl of codeBlocks) {
    const pre = codeEl.closest("pre");
    const wrapper = pre?.closest(".code-block-wrapper") ?? pre;
    if (!wrapper) continue;
    if (wrapper.classList.contains("mermaid-rendered")) continue;

    const code = codeEl.textContent?.trim() ?? "";
    if (!code) continue;

    wrapper.classList.add("mermaid-rendered");
    pending.push({ wrapper: wrapper as HTMLElement, code });
  }

  if (pending.length === 0) return;

  // Show loading state
  for (const { wrapper } of pending) {
    const loading = document.createElement("div");
    loading.className = "mermaid-loading";
    loading.textContent = "Loading diagram...";
    wrapper.insertBefore(loading, wrapper.firstChild);
  }

  try {
    const mod = await loadMermaid();
    const mermaid = mod.default;

    // Re-initialize with current theme
    mermaid.initialize({
      startOnLoad: false,
      theme: isDarkTheme() ? "dark" : "default",
      securityLevel: "strict",
    });

    for (const { wrapper, code } of pending) {
      const loadingEl = wrapper.querySelector(".mermaid-loading");

      try {
        const id = `mermaid-${++idCounter}`;
        const { svg } = await mermaid.render(id, code);

        const container = document.createElement("div");
        container.className = "mermaid-container";
        container.innerHTML = svg;

        // Replace wrapper content with rendered SVG
        wrapper.replaceWith(container);
      } catch {
        // On error, remove loading and show error badge
        loadingEl?.remove();
        const badge = document.createElement("div");
        badge.className = "mermaid-error";
        badge.textContent = "Diagram render error";
        wrapper.insertBefore(badge, wrapper.firstChild);
      }
    }
  } catch {
    // Failed to load mermaid module
    for (const { wrapper } of pending) {
      const loadingEl = wrapper.querySelector(".mermaid-loading");
      if (loadingEl) {
        loadingEl.textContent = "Failed to load diagram renderer";
        loadingEl.classList.add("mermaid-error");
      }
    }
  }
}
