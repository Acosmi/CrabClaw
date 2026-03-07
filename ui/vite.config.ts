import path from "node:path";
import { fileURLToPath } from "node:url";
import { defineConfig } from "vite";

const here = path.dirname(fileURLToPath(import.meta.url));

function normalizeBase(input: string): string {
  const trimmed = input.trim();
  if (!trimmed) {
    return "/";
  }
  if (trimmed === "./") {
    return "./";
  }
  if (trimmed.endsWith("/")) {
    return trimmed;
  }
  return `${trimmed}/`;
}

export default defineConfig(() => {
  const envBase = process.env.OPENACOSMI_CONTROL_UI_BASE_PATH?.trim();
  const base = envBase ? normalizeBase(envBase) : "./";
  return {
    base,
    publicDir: path.resolve(here, "public"),
    optimizeDeps: {
      include: ["lit/directives/repeat.js"],
    },
    build: {
      outDir: path.resolve(here, "../dist/control-ui"),
      emptyOutDir: true,
      sourcemap: true,
    },
    server: {
      host: true,
      port: 26222,
      strictPort: true,
      proxy: {
        "/ws": {
          target: "ws://localhost:19001",
          ws: true,
          // eslint-disable-next-line @typescript-eslint/no-explicit-any
          configure: (proxy: any) => {
            proxy.on("error", (err: { code?: string; message?: string }) => {
              if (err.code === "ECONNREFUSED") {
                // Gateway 停止期间静默 ECONNREFUSED，重启后自动恢复
                return;
              }
              console.error("[vite-proxy]", err.message);
            });
          },
        },
        "/browser-extension": {
          target: "http://localhost:19001",
          // eslint-disable-next-line @typescript-eslint/no-explicit-any
          configure: (proxy: any) => {
            proxy.on("error", (err: { code?: string; message?: string }) => {
              if (err.code === "ECONNREFUSED") return;
              console.error("[vite-proxy]", err.message);
            });
          },
        },
      },
    },
  };
});
