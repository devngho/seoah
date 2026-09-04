export const BOARD_SIZE = 15;
export type Stone = 'black' | 'white' | null;
export type Board = Stone[][];

export function createEmptyBoard(): Board {
  return Array.from({ length: BOARD_SIZE }, () => Array<Stone>(BOARD_SIZE).fill(null));
}

function inBounds(r: number, c: number) {
  return r >= 0 && r < BOARD_SIZE && c >= 0 && c < BOARD_SIZE;
}

function countDir(board: Board, row: number, col: number, dr: number, dc: number, color: Stone) {
  let r = row + dr;
  let c = col + dc;
  let count = 0;
  while (inBounds(r, c) && board[r][c] === color) {
    count++;
    r += dr;
    c += dc;
  }
  return count;
}

export function checkWin(board: Board, row: number, col: number, color: Stone): boolean {
  if (!color) return false;
  const directions: [number, number][] = [[0, 1], [1, 0], [1, 1], [1, -1]];
  for (const [dr, dc] of directions) {
    const count =
      1 + countDir(board, row, col, dr, dc, color) + countDir(board, row, col, -dr, -dc, color);
    if (count >= 5) return true;
  }
  return false;
}

export function isBoardFull(board: Board): boolean {
  return board.every((row) => row.every((cell) => cell !== null));
}

// 특정 방향으로 이어진 돌 개수와, 그 라인의 양 끝이 열려있는지(빈칸인지) 계산
function getLineInfo(board: Board, row: number, col: number, dr: number, dc: number, color: Stone) {
  let count = 1;
  let openEnds = 0;

  let r = row + dr, c = col + dc;
  while (inBounds(r, c) && board[r][c] === color) {
    count++;
    r += dr; c += dc;
  }
  if (inBounds(r, c) && board[r][c] === null) openEnds++;

  r = row - dr; c = col - dc;
  while (inBounds(r, c) && board[r][c] === color) {
    count++;
    r -= dr; c -= dc;
  }
  if (inBounds(r, c) && board[r][c] === null) openEnds++;

  return { count, openEnds };
}

function scoreFromLine(count: number, openEnds: number): number {
  if (count >= 5) return 1_000_000;
  if (count === 4) return openEnds >= 1 ? 100_000 : 100;
  if (count === 3) return openEnds === 2 ? 10_000 : openEnds === 1 ? 1_000 : 50;
  if (count === 2) return openEnds === 2 ? 500 : openEnds === 1 ? 100 : 10;
  return openEnds >= 1 ? 10 : 1;
}

function evaluatePoint(board: Board, row: number, col: number, color: Stone): number {
  const axes: [number, number][] = [[0, 1], [1, 0], [1, 1], [1, -1]];
  let score = 0;
  for (const [dr, dc] of axes) {
    const { count, openEnds } = getLineInfo(board, row, col, dr, dc, color);
    score += scoreFromLine(count, openEnds);
  }
  return score;
}

// 기존 돌 주변 2칸 이내의 빈 칸만 후보로 삼아 계산량을 줄임
function getCandidates(board: Board): { row: number; col: number }[] {
  const candidates: { row: number; col: number }[] = [];
  const seen = new Set<string>();
  let hasStone = false;

  for (let r = 0; r < BOARD_SIZE; r++) {
    for (let c = 0; c < BOARD_SIZE; c++) {
      if (board[r][c] === null) continue;
      hasStone = true;
      for (let dr = -2; dr <= 2; dr++) {
        for (let dc = -2; dc <= 2; dc++) {
          const nr = r + dr, nc = c + dc;
          if (!inBounds(nr, nc) || board[nr][nc] !== null) continue;
          const key = `${nr}-${nc}`;
          if (!seen.has(key)) {
            seen.add(key);
            candidates.push({ row: nr, col: nc });
          }
        }
      }
    }
  }

  if (!hasStone) {
    const mid = Math.floor(BOARD_SIZE / 2);
    return [{ row: mid, col: mid }];
  }
  return candidates;
}

export function bestMove(board: Board, aiColor: Stone, humanColor: Stone): { row: number; col: number } | null {
  const candidates = getCandidates(board);
  let best: { row: number; col: number } | null = null;
  let bestScore = -Infinity;

  for (const { row, col } of candidates) {
    board[row][col] = aiColor;
    const attackScore = evaluatePoint(board, row, col, aiColor);
    board[row][col] = humanColor;
    const defenseScore = evaluatePoint(board, row, col, humanColor);
    board[row][col] = null;

    const total = attackScore + defenseScore * 0.9;
    if (total > bestScore) {
      bestScore = total;
      best = { row, col };
    }
  }

  return best;
}