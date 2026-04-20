// Mirrors Go API types from internal/httpapi/types.go and internal/game.

export type Letter = string; // single character "A".."Z" or "?"

export type Status = "waiting" | "active" | "completed";
export type TurnType = "play" | "exchange" | "pass";

export interface PlacedTile {
  letter: Letter;
  blank: boolean;
}

export interface Board {
  squares: Array<Array<PlacedTile | null>>;
}

export interface Placement {
  row: number;
  col: number;
  letter: Letter;
  blank: boolean;
}

export interface ScoredWord {
  word: string;
  score: number;
}

export interface PlayerView {
  userId: string;
  name: string;
  score: number;
  rack?: Letter[];
  rackSize: number;
}

export interface TurnRecord {
  playerIdx: number;
  type: TurnType;
  placements?: Placement[];
  words?: ScoredWord[];
  score: number;
  bingo?: boolean;
  exchanged?: number;
  at: string;
}

export interface GameView {
  id: string;
  creatorId: string;
  inviteCode: string;
  status: Status;
  numPlayers: number;
  openSeats: number;
  createdAt: string;
  startedAt?: string;
  endedAt?: string;
  players: PlayerView[];
  turn: number;
  currentIdx: number;
  board: Board;
  bagSize: number;
  history: TurnRecord[];
  winners?: number[];
  yourPlayerIdx: number;
  lastPlay?: TurnRecord;
}

export interface CreateGameRequest {
  numPlayers: number;
}

export interface CreateGameResponse {
  gameId: string;
  inviteCode: string;
}

export interface JoinRequest {
  inviteCode: string;
}

export interface PlayRequest {
  type: TurnType;
  placements?: Placement[];
  exchange?: Letter[];
}

export interface PlayResult {
  words: ScoredWord[];
  score: number;
  bingo: boolean;
  usedRack: Letter[];
}

export interface PlayResponse {
  result?: PlayResult;
  game: GameView;
}

export interface ValidateRequest {
  placements: Placement[];
}

export interface ValidateResponse {
  valid: boolean;
  words?: ScoredWord[];
  score: number;
  bingo: boolean;
  error?: string;
  invalidWords?: string[];
}

export interface ErrorResponse {
  error: string;
  invalidWords?: string[];
}

export interface UserSummary {
  userId: string;
  name: string;
  email?: string;
}

export interface GameSummary {
  id: string;
  status: Status;
  createdAt: string;
  playerNames: string[];
  yourTurn: boolean;
}
