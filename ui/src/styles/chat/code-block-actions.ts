const COPIED_RESET_MS = 1500;

async function copyCodeToClipboard(text: string): Promise<boolean> {
  if (!text) {
    return false;
  }
  try {
    await navigator.clipboard.writeText(text);
    return true;
  } catch {
    return false;
  }
}

function handleCodeBlockClick(e: Event) {
  const target = e.target as HTMLElement | null;
  if (!target) {
    return;
  }
  const trigger = target.closest(".code-block-copy-trigger") as HTMLElement | null;
  if (!trigger) {
    return;
  }
  if (trigger.dataset.copying === "1") {
    return;
  }
  const wrapper = trigger.closest(".code-block-wrapper");
  if (!wrapper) {
    return;
  }
  const codeEl = wrapper.querySelector("pre code");
  if (!codeEl) {
    return;
  }
  const rawCode = codeEl.textContent ?? "";

  trigger.dataset.copying = "1";
  copyCodeToClipboard(rawCode).then((ok) => {
    if (!trigger.isConnected) {
      return;
    }
    delete trigger.dataset.copying;

    if (ok) {
      trigger.dataset.copied = "1";
      trigger.textContent = "Copied!";
    } else {
      trigger.dataset.copied = "0";
      trigger.textContent = "Failed";
    }
    window.setTimeout(() => {
      if (!trigger.isConnected) {
        return;
      }
      delete trigger.dataset.copied;
      trigger.textContent = "\u2398"; // ⎘
    }, COPIED_RESET_MS);
  });
}

export function initCodeBlockCopyListeners(container: HTMLElement) {
  container.addEventListener("click", handleCodeBlockClick);
}
