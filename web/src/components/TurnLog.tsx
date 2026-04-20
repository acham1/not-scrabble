import type { GameView } from "../api/types";

export function TurnLog({ game }: { game: GameView }) {
  if (game.history.length === 0) {
    return null;
  }
  return (
    <section className="turn-log">
      <h3>History</h3>
      <ol reversed start={game.history.length}>
        {[...game.history].reverse().map((t, i) => {
          const origIdx = game.history.length - 1 - i;
          const name = game.players[t.playerIdx]?.name ?? "?";
          let detail = "";
          if (t.type === "play") {
            const words = (t.words ?? []).map((w) => `${w.word} (${w.score})`).join(", ");
            detail = `played ${words || "a word"} for ${t.score}`;
            if (t.bingo) detail += " — BINGO!";
          } else if (t.type === "exchange") {
            detail = `exchanged ${t.exchanged} tile(s)`;
          } else {
            detail = "passed";
          }
          return (
            <li key={origIdx}>
              <strong>{name}</strong> {detail}
            </li>
          );
        })}
      </ol>
    </section>
  );
}
