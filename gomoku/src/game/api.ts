export type Stone = 0 | 1 | 2;
export type Board = Stone[][];

export interface RenjuConfig {
  enabled: boolean;
  forbidDoubleThree: boolean;
  forbidDoubleFour: boolean;
  forbidOverline: boolean;
}

interface BestMoveRequest {
  board: Board;
  player: 1 | 2;
  renju?: RenjuConfig;
}

interface BestMoveResponse {
  y: number;
  x: number;
  score: number;
  noMove?: boolean;
  nodesVisited: number;
  error?: string;
}

export async function getBestMove(
  board: Board,
  player: 1 | 2,
  options?: { renju?: RenjuConfig; },
  lastMove?: { row: number; col: number },
): Promise<BestMoveResponse> {
  console.log(player, " request")
  const res = await fetch(`${location.origin}/api/best-move`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ board, player, options, lastMove }),
  });

  if (!res.ok) {
    const err = await res.json().catch(() => ({}));
    throw new Error(err.error ?? `서버 에러: ${res.status}`);
  }

  return res.json();
}
