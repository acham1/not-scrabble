import { useDraggable, useDroppable } from "@dnd-kit/core";
import { Tile } from "./Tile";

export function RackStrip({
  rack,
  rackUsed,
  exchangeMode,
  exchangeSelection,
  onToggleExchange,
  selectedIdx,
  onTileTap,
  onShuffle,
}: {
  rack: string[];
  rackUsed: Set<number>;
  exchangeMode: boolean;
  exchangeSelection: Set<number>;
  onToggleExchange: (idx: number) => void;
  selectedIdx?: number | null;
  onTileTap?: (idx: number) => void;
  onShuffle?: () => void;
}) {
  const { setNodeRef, isOver } = useDroppable({ id: "rack-zone" });
  return (
    <div
      ref={setNodeRef}
      className={`rack${isOver ? " rack-over" : ""}${exchangeMode ? " rack-exchange" : ""}`}
      aria-label="Your tiles"
    >
      {rack.map((letter, idx) => {
        const used = rackUsed.has(idx);
        const selected = exchangeSelection.has(idx);
        if (used) {
          return <div key={idx} className="rack-slot empty" />;
        }
        if (exchangeMode) {
          return (
            <button
              key={idx}
              type="button"
              className={`rack-slot exchange${selected ? " selected" : ""}`}
              onClick={() => onToggleExchange(idx)}
            >
              <Tile letter={letter === "?" ? "?" : letter} blank={letter === "?"} />
            </button>
          );
        }
        return (
          <DraggableRackTile
            key={idx}
            idx={idx}
            letter={letter}
            selected={selectedIdx === idx}
            onTap={onTileTap}
          />
        );
      })}
      {onShuffle && !exchangeMode && (
        <button type="button" className="rack-shuffle-btn" onClick={onShuffle} title="Shuffle rack">
          ⟳
        </button>
      )}
    </div>
  );
}

function DraggableRackTile({
  idx,
  letter,
  selected,
  onTap,
}: {
  idx: number;
  letter: string;
  selected?: boolean;
  onTap?: (idx: number) => void;
}) {
  const { attributes, listeners, setNodeRef: setDragRef, transform, isDragging } = useDraggable({
    id: `rack-${idx}`,
  });
  const { setNodeRef: setDropRef, isOver } = useDroppable({
    id: `rack-${idx}`,
  });
  const style = transform
    ? { transform: `translate3d(${transform.x}px, ${transform.y}px, 0)` }
    : undefined;
  const displayLetter = letter === "?" ? "?" : letter;
  return (
    <div
      ref={(node) => { setDragRef(node); setDropRef(node); }}
      style={style}
      {...listeners}
      {...attributes}
      onClick={() => onTap?.(idx)}
      className={`rack-slot tile-drag${isDragging ? " dragging" : ""}${selected ? " selected" : ""}${isOver ? " rack-slot-over" : ""}`}
    >
      <Tile letter={displayLetter} blank={letter === "?"} />
    </div>
  );
}
