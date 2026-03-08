import type { AppViewState } from "../app-view-state.ts";
import type { GatewayBrowserClient } from "../gateway.ts";

export type MediaMutationInvalidator<TResult = unknown> = (
  state: AppViewState,
  result: TResult,
) => Promise<unknown>;

export type MediaMutationSpec<TResult = unknown> = {
  label: string;
  run: (client: GatewayBrowserClient, state: AppViewState) => Promise<TResult>;
  invalidate?: MediaMutationInvalidator<TResult>[];
  onSuccess?: (state: AppViewState, result: TResult) => void;
  onError?: (state: AppViewState, error: unknown) => void;
  onFinally?: (state: AppViewState) => void;
};

function resolveMediaClient(state: AppViewState): GatewayBrowserClient | null {
  if (!state.client || !state.connected) {
    return null;
  }
  return state.client;
}

export async function runMediaMutation<TResult>(
  state: AppViewState,
  spec: MediaMutationSpec<TResult>,
): Promise<boolean> {
  const client = resolveMediaClient(state);
  if (!client) {
    return false;
  }

  try {
    const result = await spec.run(client, state);
    spec.onSuccess?.(state, result);

    for (const invalidate of spec.invalidate ?? []) {
      await invalidate(state, result);
    }

    return true;
  } catch (error) {
    console.error(`${spec.label} failed`, error);
    spec.onError?.(state, error);
    return false;
  } finally {
    spec.onFinally?.(state);
    state.requestUpdate();
  }
}
