type LightboxImage = {
  url: string;
  alt?: string;
};

let overlay: HTMLElement | null = null;
let currentImages: LightboxImage[] = [];
let currentIndex = 0;

function createOverlay(): HTMLElement {
  const el = document.createElement("div");
  el.className = "lightbox-overlay";
  el.setAttribute("role", "dialog");
  el.setAttribute("aria-modal", "true");
  el.setAttribute("aria-label", "Image viewer");

  el.innerHTML = `
    <button class="lightbox-close" aria-label="Close" title="Close">&times;</button>
    <button class="lightbox-nav lightbox-nav--prev" aria-label="Previous">&lsaquo;</button>
    <button class="lightbox-nav lightbox-nav--next" aria-label="Next">&rsaquo;</button>
    <div class="lightbox-counter"></div>
    <div class="lightbox-img-container">
      <img class="lightbox-img" alt="" />
    </div>
  `;

  return el;
}

function updateOverlay() {
  if (!overlay) return;
  const img = overlay.querySelector(".lightbox-img") as HTMLImageElement;
  const counter = overlay.querySelector(".lightbox-counter") as HTMLElement;
  const prevBtn = overlay.querySelector(".lightbox-nav--prev") as HTMLElement;
  const nextBtn = overlay.querySelector(".lightbox-nav--next") as HTMLElement;

  const current = currentImages[currentIndex];
  if (!current) return;

  img.src = current.url;
  img.alt = current.alt ?? "Image";

  const multi = currentImages.length > 1;
  counter.textContent = multi ? `${currentIndex + 1} / ${currentImages.length}` : "";
  counter.style.display = multi ? "" : "none";
  prevBtn.style.display = multi ? "" : "none";
  nextBtn.style.display = multi ? "" : "none";
}

function handleKeydown(e: KeyboardEvent) {
  if (e.key === "Escape") {
    closeLightbox();
  } else if (e.key === "ArrowLeft" && currentImages.length > 1) {
    currentIndex = (currentIndex - 1 + currentImages.length) % currentImages.length;
    updateOverlay();
  } else if (e.key === "ArrowRight" && currentImages.length > 1) {
    currentIndex = (currentIndex + 1) % currentImages.length;
    updateOverlay();
  }
}

function handleOverlayClick(e: MouseEvent) {
  const target = e.target as HTMLElement;
  if (target.classList.contains("lightbox-overlay") || target.classList.contains("lightbox-img-container")) {
    closeLightbox();
  } else if (target.classList.contains("lightbox-close")) {
    closeLightbox();
  } else if (target.classList.contains("lightbox-nav--prev")) {
    currentIndex = (currentIndex - 1 + currentImages.length) % currentImages.length;
    updateOverlay();
  } else if (target.classList.contains("lightbox-nav--next")) {
    currentIndex = (currentIndex + 1) % currentImages.length;
    updateOverlay();
  }
}

export function openLightbox(images: LightboxImage[], startIndex = 0) {
  if (images.length === 0) return;
  closeLightbox();

  currentImages = images;
  currentIndex = Math.max(0, Math.min(startIndex, images.length - 1));

  overlay = createOverlay();
  updateOverlay();

  document.body.appendChild(overlay);
  requestAnimationFrame(() => overlay?.classList.add("lightbox-overlay--visible"));

  document.addEventListener("keydown", handleKeydown);
  overlay.addEventListener("click", handleOverlayClick);
}

export function closeLightbox() {
  if (!overlay) return;
  document.removeEventListener("keydown", handleKeydown);
  overlay.classList.remove("lightbox-overlay--visible");
  const el = overlay;
  el.addEventListener("transitionend", () => el.remove(), { once: true });
  // Fallback removal if no transition fires
  setTimeout(() => { if (el.parentNode) el.remove(); }, 300);
  overlay = null;
  currentImages = [];
  currentIndex = 0;
}
