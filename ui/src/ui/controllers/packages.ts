import type { GatewayBrowserClient } from "../gateway.ts";
import type { PackageCatalogBrowseResult, PackageCatalogItem, PackageKind } from "../types.ts";

const PAGE_SIZE = 20;

export type PackagesState = {
  client: GatewayBrowserClient | null;
  connected: boolean;
  packagesLoading: boolean;
  packagesItems: PackageCatalogItem[];
  packagesTotal: number;
  packagesError: string | null;
  packagesKindFilter: PackageKind | "all";
  packagesKeyword: string;
  packagesBusyId: string | null;
};

function getErrorMessage(err: unknown): string {
  if (err instanceof Error) return err.message;
  return String(err);
}

export async function browsePackages(state: PackagesState) {
  if (!state.client || !state.connected) return;
  if (state.packagesLoading) return;
  state.packagesLoading = true;
  state.packagesError = null;
  state.packagesItems = [];
  state.packagesTotal = 0;
  try {
    const params: Record<string, unknown> = { page: 1, pageSize: PAGE_SIZE };
    if (state.packagesKindFilter !== "all") params.kind = state.packagesKindFilter;
    if (state.packagesKeyword.trim()) params.keyword = state.packagesKeyword.trim();
    const res = await state.client.request<PackageCatalogBrowseResult>("packages.catalog.browse", params);
    if (res) {
      state.packagesItems = res.items ?? [];
      state.packagesTotal = res.total ?? 0;
    }
  } catch (err) {
    state.packagesError = getErrorMessage(err);
  } finally {
    state.packagesLoading = false;
  }
}

export async function loadMorePackages(state: PackagesState) {
  if (!state.client || !state.connected) return;
  if (state.packagesLoading) return;
  const nextPage = Math.floor(state.packagesItems.length / PAGE_SIZE) + 1;
  state.packagesLoading = true;
  state.packagesError = null;
  try {
    const params: Record<string, unknown> = { page: nextPage, pageSize: PAGE_SIZE };
    if (state.packagesKindFilter !== "all") params.kind = state.packagesKindFilter;
    if (state.packagesKeyword.trim()) params.keyword = state.packagesKeyword.trim();
    const res = await state.client.request<PackageCatalogBrowseResult>("packages.catalog.browse", params);
    if (res) {
      state.packagesItems = [...state.packagesItems, ...(res.items ?? [])];
      state.packagesTotal = res.total ?? 0;
    }
  } catch (err) {
    state.packagesError = getErrorMessage(err);
  } finally {
    state.packagesLoading = false;
  }
}

export async function installPackage(state: PackagesState, id: string) {
  if (!state.client || !state.connected) return;
  state.packagesBusyId = id;
  state.packagesError = null;
  try {
    await state.client.request("packages.install", { id });
    state.packagesItems = state.packagesItems.map((item) =>
      item.id === id ? { ...item, isInstalled: true } : item,
    );
  } catch (err) {
    state.packagesError = getErrorMessage(err);
  } finally {
    state.packagesBusyId = null;
  }
}

export async function removePackage(state: PackagesState, id: string) {
  if (!state.client || !state.connected) return;
  state.packagesBusyId = id;
  state.packagesError = null;
  try {
    await state.client.request("packages.remove", { id });
    state.packagesItems = state.packagesItems.map((item) =>
      item.id === id ? { ...item, isInstalled: false } : item,
    );
  } catch (err) {
    state.packagesError = getErrorMessage(err);
  } finally {
    state.packagesBusyId = null;
  }
}
