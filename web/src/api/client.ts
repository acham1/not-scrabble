import type {
  CreateGameRequest,
  CreateGameResponse,
  ErrorResponse,
  GameSummary,
  GameView,
  JoinRequest,
  PlayRequest,
  PlayResponse,
  UserSummary,
} from "./types";

export class ApiError extends Error {
  status: number;
  invalidWords?: string[];
  constructor(status: number, message: string, invalidWords?: string[]) {
    super(message);
    this.status = status;
    this.invalidWords = invalidWords;
  }
}

async function request<T>(method: string, path: string, body?: unknown): Promise<T> {
  const init: RequestInit = {
    method,
    credentials: "include",
    headers: body ? { "Content-Type": "application/json" } : undefined,
    body: body ? JSON.stringify(body) : undefined,
  };
  const resp = await fetch(path, init);
  const text = await resp.text();
  const data = text ? JSON.parse(text) : null;
  if (!resp.ok) {
    const err = data as ErrorResponse | null;
    throw new ApiError(resp.status, err?.error ?? resp.statusText, err?.invalidWords);
  }
  return data as T;
}

export interface AuthConfig {
  devLogin: boolean;
  googleClientId?: string;
}

export const api = {
  authConfig: () => request<AuthConfig>("GET", "/api/auth/config"),
  devLogin: (userId: string, name: string) =>
    request<UserSummary>("POST", "/api/auth/dev/login", { userId, name }),
  devLogout: () => request<void>("POST", "/api/auth/dev/logout"),
  googleCallback: (credential: string) =>
    request<UserSummary>("POST", "/api/auth/google/callback", { credential }),
  googleLogout: () => request<void>("POST", "/api/auth/google/logout"),
  me: () => request<UserSummary>("GET", "/api/users/me"),
  myGames: () => request<GameSummary[]>("GET", "/api/users/me/games"),
  createGame: (req: CreateGameRequest) => request<CreateGameResponse>("POST", "/api/games", req),
  joinGame: (req: JoinRequest) => request<GameView>("POST", "/api/games/join", req),
  getGame: (id: string) => request<GameView>("GET", `/api/games/${id}`),
  play: (id: string, req: PlayRequest) =>
    request<PlayResponse>("POST", `/api/games/${id}/plays`, req),
  pushVapidKey: () => request<{ key: string }>("GET", "/api/push/vapid-key"),
  pushSubscribe: (sub: PushSubscriptionJSON) =>
    request<void>("POST", "/api/push/subscribe", sub),
};

/** Register service worker and subscribe to Web Push if VAPID key is available. */
export async function setupPushSubscription(): Promise<void> {
  if (!("serviceWorker" in navigator) || !("PushManager" in window)) return;
  try {
    // Fetch VAPID key directly to avoid noisy console errors when push isn't configured.
    const resp = await fetch("/api/push/vapid-key", { credentials: "include" });
    if (!resp.ok) return;
    const { key } = (await resp.json()) as { key: string };
    if (!key) return;
    const reg = await navigator.serviceWorker.register("/sw.js");
    await navigator.serviceWorker.ready;
    const existing = await reg.pushManager.getSubscription();
    if (existing) {
      await api.pushSubscribe(existing.toJSON());
      return;
    }
    const permission = await Notification.requestPermission();
    if (permission !== "granted") return;
    const sub = await reg.pushManager.subscribe({
      userVisibleOnly: true,
      applicationServerKey: urlBase64ToUint8Array(key),
    });
    await api.pushSubscribe(sub.toJSON());
  } catch (err) {
    console.warn("Push subscription failed:", err);
  }
}

function urlBase64ToUint8Array(base64String: string): Uint8Array<ArrayBuffer> {
  const padding = "=".repeat((4 - (base64String.length % 4)) % 4);
  const base64 = (base64String + padding).replace(/-/g, "+").replace(/_/g, "/");
  const raw = atob(base64);
  const arr = new Uint8Array(raw.length);
  for (let i = 0; i < raw.length; i++) arr[i] = raw.charCodeAt(i);
  return arr;
}
