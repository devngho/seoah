export type Stone = 0 | 1 | 2;
export type Board = Stone[][];

interface BestMoveResponse {
  y: number;
  x: number;
  score: number;
  noMove?: boolean;
  error?: string;
}

export async function getBestMove(
  board: Board,
  player: 1 | 2
): Promise<BestMoveResponse> {
  const res = await fetch("http://localhost:8090/api/best-move", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ board, player }),
  });

  if (!res.ok) {
    const err = await res.json().catch(() => ({}));
    throw new Error(err.error ?? `서버 에러: ${res.status}`);
  }

  return res.json();
}