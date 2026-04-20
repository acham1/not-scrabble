import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import {
  DndContext,
  DragEndEvent,
  PointerSensor,
  TouchSensor,
  useSensor,
  useSensors,
} from "@dnd-kit/core";
import { api, ApiError } from "../api/client";
import type { Placement, ValidateResponse } from "../api/types";
import { useGame } from "../state/useGame";
import { BoardGrid } from "../components/BoardGrid";
import { RackStrip } from "../components/RackStrip";
import { TurnLog } from "../components/TurnLog";
import { BlankPicker } from "../components/BlankPicker";
import { previewScore } from "../state/scoring";

export interface PendingPlacement {
  row: number;
  col: number;
  letter: string;
  blank: boolean;
  rackIdx: number;
}

export function GameBoardView({
  gameId,
  onBack,
}: {
  gameId: string;
  onBack: () => void;
}) {
  const { game, error, loading, refresh, setGame } = useGame(gameId);
  const [pending, setPending] = useState<PendingPlacement[]>([]);
  const [blankPrompt, setBlankPrompt] = useState<{ row: number; col: number; rackIdx: number } | null>(null);
  const [submitError, setSubmitError] = useState<string | null>(null);
  const [invalidWords, setInvalidWords] = useState<string[]>([]);
  const [exchangeMode, setExchangeMode] = useState(false);
  const [exchangeSelection, setExchangeSelection] = useState<Set<number>>(new Set());
  const [busy, setBusy] = useState(false);
  const [selectedRackIdx, setSelectedRackIdx] = useState<number | null>(null);
  const [copied, setCopied] = useState(false);
  const [rackOrder, setRackOrder] = useState<number[]>([]);
  const [validation, setValidation] = useState<ValidateResponse | null>(null);
  const validateTimer = useRef<number | null>(null);
  const lastTurn = useRef<number>(-1);

  const sensors = useSensors(
    useSensor(PointerSensor, { activationConstraint: { distance: 4 } }),
    useSensor(TouchSensor, { activationConstraint: { delay: 120, tolerance: 8 } }),
  );

  // rackUsed tracks *server* rack indices that are placed on the board
  const serverRackUsed = useMemo(() => {
    const used = new Set<number>();
    for (const p of pending) used.add(p.rackIdx);
    return used;
  }, [pending]);

  // rackUsed in *display* indices for the RackStrip
  const rackUsed = useMemo(() => {
    const used = new Set<number>();
    for (let d = 0; d < rackOrder.length; d++) {
      if (serverRackUsed.has(rackOrder[d])) used.add(d);
    }
    return used;
  }, [serverRackUsed, rackOrder]);

  const scorePreview = useMemo(
    () => (game ? previewScore(game.board, pending) : null),
    [game, pending],
  );

  const isYour = game != null && game.yourPlayerIdx >= 0 && game.currentIdx === game.yourPlayerIdx && game.status === "active";
  const myPlayer = game != null && game.yourPlayerIdx >= 0 ? game.players[game.yourPlayerIdx] : null;
  const serverRack = myPlayer?.rack ?? [];

  // Clear staged tiles when the game state changes (opponent played, or after
  // our own play). Tracked by turn number.
  useEffect(() => {
    if (!game) return;
    if (lastTurn.current >= 0 && game.turn !== lastTurn.current) {
      setPending([]);
      setValidation(null);
      setSubmitError(null);
      setInvalidWords([]);
      setExchangeMode(false);
      setExchangeSelection(new Set());
    }
    lastTurn.current = game.turn;
  }, [game?.turn]);

  // Keep rackOrder in sync with server rack length; preserve order on refresh
  // if the rack hasn't changed, reset on length change (post-play refill).
  useEffect(() => {
    setRackOrder((prev) => {
      if (prev.length === serverRack.length) return prev;
      return serverRack.map((_, i) => i);
    });
  }, [serverRack.length]);

  // Display rack in the user's chosen order
  const rack = rackOrder.length === serverRack.length
    ? rackOrder.map((i) => serverRack[i])
    : serverRack;
  // Map from display index to server rack index
  const displayToServer = useCallback((displayIdx: number): number =>
    rackOrder.length === serverRack.length ? rackOrder[displayIdx] : displayIdx,
  [rackOrder, serverRack.length]);

  const shuffleRack = useCallback(() => {
    setRackOrder((prev) => {
      const arr = [...prev];
      for (let i = arr.length - 1; i > 0; i--) {
        const j = Math.floor(Math.random() * (i + 1));
        [arr[i], arr[j]] = [arr[j], arr[i]];
      }
      return arr;
    });
  }, []);

  // Debounced server-side word validation when tiles are staged.
  useEffect(() => {
    if (validateTimer.current) {
      window.clearTimeout(validateTimer.current);
      validateTimer.current = null;
    }
    // Only validate when the client-side preview thinks the placement is valid
    // (tiles in line, connected, etc.)
    if (!game || pending.length === 0 || !scorePreview?.valid) {
      setValidation(null);
      return;
    }
    validateTimer.current = window.setTimeout(async () => {
      try {
        const placements: Placement[] = pending.map(({ rackIdx: _rackIdx, ...rest }) => rest);
        const resp = await api.validate(gameId, placements);
        setValidation(resp);
      } catch {
        // Validation is best-effort; don't break the UI
        setValidation(null);
      }
    }, 300);
    return () => {
      if (validateTimer.current) {
        window.clearTimeout(validateTimer.current);
        validateTimer.current = null;
      }
    };
  }, [game, gameId, pending, scorePreview?.valid]);

  // Tap-to-place: tap a rack tile to select, then tap an empty board cell.
  // rackIdx here is a *display* index.
  const handleRackTap = useCallback((displayIdx: number) => {
    if (exchangeMode) return;
    setSubmitError(null);
    setInvalidWords([]);
    setSelectedRackIdx((prev) => (prev === displayIdx ? null : displayIdx));
  }, [exchangeMode]);

  const handleCellTap = useCallback((row: number, col: number) => {
    if (selectedRackIdx === null) return;
    const serverIdx = displayToServer(selectedRackIdx);
    if (game!.board.squares[row][col] || pending.some((p) => p.row === row && p.col === col)) return;
    if (serverRackUsed.has(serverIdx)) { setSelectedRackIdx(null); return; }
    const letter = rack[selectedRackIdx];
    if (letter === "?") {
      setBlankPrompt({ row, col, rackIdx: serverIdx });
    } else {
      setPending((prev) => [...prev, { row, col, letter, blank: false, rackIdx: serverIdx }]);
    }
    setSelectedRackIdx(null);
    setSubmitError(null);
    setInvalidWords([]);
  }, [selectedRackIdx, game, pending, rack, serverRackUsed, displayToServer]);

  // Keyboard: Escape recalls last tile, Enter submits.
  useEffect(() => {
    const handler = (e: KeyboardEvent) => {
      if (e.key === "Escape") {
        if (pending.length > 0) {
          setPending((prev) => prev.slice(0, -1));
          setSubmitError(null);
          setInvalidWords([]);
        }
        setSelectedRackIdx(null);
      }
    };
    window.addEventListener("keydown", handler);
    return () => window.removeEventListener("keydown", handler);
  }, [pending.length]);

  if (loading && !game) return <div className="center muted">Loading game…</div>;
  if (!game) return <div className="center error">Failed to load: {error}</div>;

  const resetSubmitErrors = () => {
    setSubmitError(null);
    setInvalidWords([]);
  };

  const handleDragEnd = (e: DragEndEvent) => {
    resetSubmitErrors();
    if (!e.over) return;
    const overId = String(e.over.id);
    const activeId = String(e.active.id);

    if (activeId.startsWith("rack-")) {
      const displayIdx = Number(activeId.slice(5));
      const serverIdx = displayToServer(displayIdx);
      if (overId.startsWith("cell-")) {
        const [, r, c] = overId.split("-");
        const row = Number(r);
        const col = Number(c);
        // Don't drop onto occupied or already-pending cells.
        if (game.board.squares[row][col] || pending.some((p) => p.row === row && p.col === col)) {
          return;
        }
        const letter = rack[displayIdx];
        if (letter === "?") {
          setBlankPrompt({ row, col, rackIdx: serverIdx });
        } else {
          setPending((prev) => [...prev, { row, col, letter, blank: false, rackIdx: serverIdx }]);
        }
      }
      // Rack-to-rack reorder: drop a rack tile onto another rack tile
      if (overId.startsWith("rack-")) {
        const targetDisplayIdx = Number(overId.slice(5));
        if (displayIdx !== targetDisplayIdx) {
          setRackOrder((prev) => {
            const arr = [...prev];
            const [removed] = arr.splice(displayIdx, 1);
            arr.splice(targetDisplayIdx, 0, removed);
            return arr;
          });
        }
      }
      return;
    }

    if (activeId.startsWith("pending-")) {
      const [, r, c] = activeId.split("-");
      const fromRow = Number(r);
      const fromCol = Number(c);
      const existing = pending.find((p) => p.row === fromRow && p.col === fromCol);
      if (!existing) return;
      if (overId === "rack-zone") {
        setPending((prev) => prev.filter((p) => !(p.row === fromRow && p.col === fromCol)));
        return;
      }
      if (overId.startsWith("cell-")) {
        const [, nr, nc] = overId.split("-");
        const row = Number(nr);
        const col = Number(nc);
        if (game.board.squares[row][col] || pending.some((p) => p.row === row && p.col === col)) {
          return;
        }
        setPending((prev) =>
          prev.map((p) =>
            p.row === fromRow && p.col === fromCol ? { ...p, row, col } : p,
          ),
        );
      }
    }
  };

  const recall = () => {
    resetSubmitErrors();
    setPending([]);
  };

  const confirmBlank = (letter: string) => {
    if (!blankPrompt) return;
    setPending((prev) => [
      ...prev,
      {
        row: blankPrompt.row,
        col: blankPrompt.col,
        letter,
        blank: true,
        rackIdx: blankPrompt.rackIdx,
      },
    ]);
    setBlankPrompt(null);
  };

  const submitPlay = async () => {
    if (pending.length === 0) return;
    setBusy(true);
    resetSubmitErrors();
    try {
      const placements: Placement[] = pending.map(({ rackIdx: _rackIdx, ...rest }) => rest);
      const resp = await api.play(gameId, { type: "play", placements });
      setGame(resp.game);
      setPending([]);
    } catch (e) {
      if (e instanceof ApiError) {
        setSubmitError(e.message);
        if (e.invalidWords) setInvalidWords(e.invalidWords);
      } else {
        setSubmitError(String(e));
      }
    } finally {
      setBusy(false);
    }
  };

  const submitPass = async () => {
    setBusy(true);
    resetSubmitErrors();
    try {
      const resp = await api.play(gameId, { type: "pass" });
      setGame(resp.game);
      setPending([]);
    } catch (e) {
      setSubmitError(e instanceof Error ? e.message : String(e));
    } finally {
      setBusy(false);
    }
  };

  const toggleExchangeTile = (idx: number) => {
    setExchangeSelection((prev) => {
      const next = new Set(prev);
      if (next.has(idx)) next.delete(idx);
      else next.add(idx);
      return next;
    });
  };

  const submitExchange = async () => {
    if (exchangeSelection.size === 0) return;
    setBusy(true);
    resetSubmitErrors();
    try {
      const tiles = [...exchangeSelection].map((displayIdx) => rack[displayIdx]);
      const resp = await api.play(gameId, { type: "exchange", exchange: tiles });
      setGame(resp.game);
      setPending([]);
      setExchangeSelection(new Set());
      setExchangeMode(false);
    } catch (e) {
      setSubmitError(e instanceof Error ? e.message : String(e));
    } finally {
      setBusy(false);
    }
  };

  const inviteUrl = `${window.location.origin}/?invite=${encodeURIComponent(game.inviteCode)}`;
  const waitingForPlayer = game.status === "active" && game.players[game.currentIdx]?.userId === "";

  const copyInviteLink = async () => {
    try {
      await navigator.clipboard.writeText(inviteUrl);
      setCopied(true);
      setTimeout(() => setCopied(false), 2000);
    } catch {
      // Fallback: select the text
    }
  };

  return (
    <div className="game">
      <DndContext sensors={sensors} onDragEnd={handleDragEnd}>
        <nav className="game-nav">
          <button className="btn-link" onClick={onBack}>← Games</button>
          <span className="muted">
            {game.status === "completed"
              ? "Game over"
              : waitingForPlayer
                ? "Waiting for a player to join"
                : isYour
                  ? "Your turn"
                  : `${game.players[game.currentIdx]?.name ?? "?"}'s turn`}
          </span>
          <button onClick={refresh} className="btn-link" disabled={busy}>
            Refresh
          </button>
        </nav>

        <div className="scores">
          {game.players.map((p, i) => {
            if (!p.userId) return null;
            return (
              <div
                key={p.userId}
                className={`score-pill${i === game.currentIdx && game.status === "active" ? " active" : ""}${game.winners?.includes(i) ? " winner" : ""}`}
              >
                <span className="score-name">{p.name}</span>
                <span className="score-value">{p.score}</span>
                <span className="score-rack muted">({p.rackSize})</span>
              </div>
            );
          })}
          <div className="score-pill bag">
            <span className="score-name">Bag</span>
            <span className="score-value">{game.bagSize}</span>
          </div>
        </div>

        {game.openSeats > 0 && (
          <section className="panel">
            <h3>Invite players ({game.numPlayers - game.openSeats}/{game.numPlayers} joined)</h3>
            <p>
              Share this code: <code className="invite-code">{game.inviteCode}</code>
            </p>
            <div className="invite-link-row">
              <code className="invite-url">{inviteUrl}</code>
              <button className="btn-copy" onClick={copyInviteLink}>
                {copied ? "Copied!" : "Copy link"}
              </button>
            </div>
          </section>
        )}

        <BoardGrid board={game.board} pending={pending} onCellTap={handleCellTap} selectedRackIdx={selectedRackIdx} />

        {game.status === "active" && myPlayer && (
          <>
            <RackStrip
              rack={rack}
              rackUsed={rackUsed}
              exchangeMode={exchangeMode}
              exchangeSelection={exchangeSelection}
              onToggleExchange={toggleExchangeTile}
              selectedIdx={selectedRackIdx}
              onTileTap={handleRackTap}
              onShuffle={shuffleRack}
            />

            {exchangeMode && (
              <div className="exchange-hint muted">Tap tiles to select them for exchange</div>
            )}

            {!exchangeMode && pending.length > 0 && scorePreview && (
              <div className={`score-preview${scorePreview.valid ? "" : " invalid"}${validation && !validation.valid ? " invalid" : ""}`}>
                {scorePreview.valid ? (
                  <>
                    {(validation?.valid ? validation.words! : scorePreview.words).map((w, i) => {
                      const isInvalid = validation?.invalidWords?.includes(w.word);
                      return (
                        <span key={`${w.word}-${i}`} className={`score-preview-word${isInvalid ? " score-preview-invalid" : ""}`}>
                          <span className="score-preview-letters">{w.word}</span>
                          <span className="score-preview-points">{w.score}</span>
                        </span>
                      );
                    })}
                    {(validation?.bingo ?? scorePreview.bingo) && (
                      <span className="score-preview-word score-preview-bingo">
                        <span className="score-preview-letters">BINGO</span>
                        <span className="score-preview-points">+50</span>
                      </span>
                    )}
                    <span className="score-preview-total">= {validation?.valid ? validation.score : scorePreview.total} pts</span>
                    {validation && !validation.valid && validation.invalidWords && (
                      <span className="score-preview-error">Not in dictionary: {validation.invalidWords.join(", ")}</span>
                    )}
                  </>
                ) : (
                  <span className="muted">{scorePreview.reason ?? "—"}</span>
                )}
              </div>
            )}

            {submitError && (
              <div className="error">
                {submitError}
                {invalidWords.length > 0 && (
                  <div>Invalid word(s): {invalidWords.join(", ")}</div>
                )}
              </div>
            )}

            <div className="actions">
              {!exchangeMode ? (
                <>
                  <button
                    disabled={!isYour || pending.length === 0 || busy}
                    onClick={submitPlay}
                  >
                    Play ({pending.length})
                  </button>
                  <button
                    disabled={pending.length === 0 || busy}
                    onClick={recall}
                  >
                    Recall
                  </button>
                  <button
                    disabled={!isYour || busy}
                    onClick={() => {
                      setPending([]);
                      setExchangeMode(true);
                    }}
                  >
                    Exchange
                  </button>
                  <button disabled={!isYour || busy} onClick={submitPass}>
                    Pass
                  </button>
                </>
              ) : (
                <>
                  <button
                    disabled={exchangeSelection.size === 0 || busy}
                    onClick={submitExchange}
                  >
                    Swap {exchangeSelection.size} tile(s)
                  </button>
                  <button
                    onClick={() => {
                      setExchangeMode(false);
                      setExchangeSelection(new Set());
                    }}
                  >
                    Cancel
                  </button>
                </>
              )}
            </div>
          </>
        )}

        {game.status === "completed" && (
          <section className="panel end-game-summary">
            <h3>Game Over</h3>
            {game.winners && game.winners.length > 0 && (
              <p>
                Winner{game.winners.length > 1 ? "s" : ""}:{" "}
                <strong>{game.winners.map((i) => game.players[i]?.name).join(", ")}</strong>
              </p>
            )}
            <table className="end-game-table">
              <thead>
                <tr>
                  <th>Player</th>
                  <th>Score</th>
                  <th>Tiles left</th>
                </tr>
              </thead>
              <tbody>
                {game.players.map((p) => (
                  <tr key={p.userId}>
                    <td>{p.name}</td>
                    <td className="score-value">{p.score}</td>
                    <td className="muted">{p.rackSize > 0 ? `${p.rackSize} tile(s)` : "—"}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </section>
        )}

        <TurnLog game={game} />

        {blankPrompt && (
          <BlankPicker
            onConfirm={confirmBlank}
            onCancel={() => setBlankPrompt(null)}
          />
        )}
      </DndContext>
    </div>
  );
}
