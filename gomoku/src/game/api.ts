export type Stone = 0 | 1 | 2;
export type ApiBoard = Stone[][];

export interface RenjuConfig {
  enabled: boolean;
  forbidDoubleThree: boolean;
  forbidDoubleFour: boolean;
  forbidOverline: boolean;
}

export type GameStatus = "playing" | "win" | "draw";

export interface MoveRecord {
  row: number;
  col: number;
  color: Stone;
}

export interface GameState {
  gameId: string;
  board: ApiBoard;
  turn: Stone;
  status: GameStatus;
  winner?: Stone;
  history: MoveRecord[];
  forbiddenMoves: [number, number][];
}

export interface BestMoveResult extends GameState {
  move?: { y: number; x: number; score: number };
}

const BASE_URL = `${location.origin}/api/games`;

async function handleResponse<T>(res: Response): Promise<T> {
  if (!res.ok) {
    const err = await res.json().catch(() => ({}));
    throw new Error(err.error ?? `서버 에러: ${res.status}`);
  }
  return res.json();
}

export async function createGame(options?: {
  renju?: RenjuConfig;
  depth?: number;
  maxNodes?: number;
}): Promise<GameState> {
  const res = await fetch(BASE_URL, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(options ?? {}),
  });
  return handleResponse<GameState>(res);
}

export async function getGameState(gameId: string): Promise<GameState> {
  const res = await fetch(`${BASE_URL}/${gameId}`);
  return handleResponse<GameState>(res);
}

export async function placeMove(
  gameId: string,
  row: number,
  col: number,
): Promise<GameState> {
  const res = await fetch(`${BASE_URL}/${gameId}/move`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ row, col }),
  });
  return handleResponse<GameState>(res);
}

export async function requestBestMove(
  gameId: string,
  place: boolean,
): Promise<BestMoveResult> {
  const res = await fetch(`${BASE_URL}/${gameId}/best-move`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ place }),
  });
  return handleResponse<BestMoveResult>(res);
}