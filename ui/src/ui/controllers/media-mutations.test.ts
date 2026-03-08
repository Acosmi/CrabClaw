import { describe, expect, it, vi } from "vitest";
import { runMediaMutation } from "./media-mutations.ts";

function createState() {
  return {
    client: null,
    connected: true,
    requestUpdate: vi.fn(),
  };
}

describe("runMediaMutation", () => {
  it("runs the mutation, invalidates dependent data, and commits a refresh", async () => {
    const request = vi.fn().mockResolvedValue({ ok: true });
    const invalidate = vi.fn().mockResolvedValue(undefined);
    const onSuccess = vi.fn();
    const state = createState();
    state.client = { request } as any;

    const result = await runMediaMutation(state as any, {
      label: "testMutation",
      run: (client) => client.request("media.test.update", { id: "draft-1" }),
      invalidate: [invalidate],
      onSuccess,
    });

    expect(result).toBe(true);
    expect(request).toHaveBeenCalledWith("media.test.update", { id: "draft-1" });
    expect(onSuccess).toHaveBeenCalledWith(state, { ok: true });
    expect(invalidate).toHaveBeenCalledWith(state, { ok: true });
    expect(state.requestUpdate).toHaveBeenCalledTimes(1);
  });

  it("returns false and still refreshes when the mutation fails", async () => {
    const request = vi.fn().mockRejectedValue(new Error("boom"));
    const onError = vi.fn();
    const consoleError = vi.spyOn(console, "error").mockImplementation(() => {});
    const state = createState();
    state.client = { request } as any;

    const result = await runMediaMutation(state as any, {
      label: "failingMutation",
      run: (client) => client.request("media.test.update", {}),
      onError,
    });

    expect(result).toBe(false);
    expect(onError).toHaveBeenCalled();
    expect(state.requestUpdate).toHaveBeenCalledTimes(1);
    consoleError.mockRestore();
  });

  it("returns false without mutating when the client is unavailable", async () => {
    const state = createState();
    state.client = null;

    const result = await runMediaMutation(state as any, {
      label: "missingClient",
      run: async () => {
        throw new Error("should not run");
      },
    });

    expect(result).toBe(false);
    expect(state.requestUpdate).not.toHaveBeenCalled();
  });
});
