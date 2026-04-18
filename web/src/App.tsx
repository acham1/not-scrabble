import { useEffect, useState } from "react";
import { api, ApiError, setupPushSubscription } from "./api/client";
import type { UserSummary } from "./api/types";
import { LoginView } from "./views/LoginView";
import { LobbyView } from "./views/LobbyView";
import { GameBoardView } from "./views/GameBoardView";

type Route =
  | { kind: "lobby"; invite?: string }
  | { kind: "game"; id: string };

function parseRoute(): Route {
  const params = new URLSearchParams(window.location.search);
  const id = params.get("game");
  if (id) return { kind: "game", id };
  const invite = params.get("invite");
  if (invite) return { kind: "lobby", invite };
  return { kind: "lobby" };
}

export function App() {
  const [user, setUser] = useState<UserSummary | null | "loading">("loading");
  const [route, setRoute] = useState<Route>(parseRoute());

  useEffect(() => {
    api
      .me()
      .then((u) => {
        setUser(u);
        setupPushSubscription();
      })
      .catch((e: unknown) => {
        if (e instanceof ApiError && e.status === 401) setUser(null);
        else {
          console.error(e);
          setUser(null);
        }
      });
  }, []);

  useEffect(() => {
    const onPop = () => setRoute(parseRoute());
    window.addEventListener("popstate", onPop);
    return () => window.removeEventListener("popstate", onPop);
  }, []);

  const navigate = (r: Route) => {
    const url =
      r.kind === "game" ? `?game=${encodeURIComponent(r.id)}` : window.location.pathname;
    window.history.pushState({}, "", url);
    setRoute(r);
  };

  if (user === "loading") return <div className="center muted">Loading…</div>;
  if (user === null) return <LoginView onLogin={setUser} />;

  return (
    <div className="app">
      <header className="topbar">
        <h1 className="logo">crossletters</h1>
        <div className="topbar-right">
          <span className="muted">{user.name}</span>
          <button
            className="btn-link"
            onClick={async () => {
              try { await api.googleLogout(); } catch {}
              try { await api.devLogout(); } catch {}
              setUser(null);
            }}
          >
            Log out
          </button>
        </div>
      </header>
      {route.kind === "lobby" && (
        <LobbyView
          inviteCode={route.invite}
          onOpenGame={(id) => navigate({ kind: "game", id })}
        />
      )}
      {route.kind === "game" && (
        <GameBoardView
          gameId={route.id}
          onBack={() => navigate({ kind: "lobby" })}
        />
      )}
    </div>
  );
}
