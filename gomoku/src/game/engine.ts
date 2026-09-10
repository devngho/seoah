export const BOARD_SIZE = 15;
export type Stone = 'black' | 'white' | null;
export type Board = Stone[][];

export function createEmptyBoard(): Board {
  return Array.from({ length: BOARD_SIZE }, () => Array<Stone>(BOARD_SIZE).fill(null));
}

export function cloneBoard(board: Board): Board {
  return board.map((row) => [...row]);
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
    if (color === 'black') {
      if (count === 5) return true;
    } else {
      if (count >= 5) return true;
    }
  }
  return false;
}

export function isBoardFull(board: Board): boolean {
  return board.every((row) => row.every((cell) => cell !== null));
}

function getLineLength(board: Board, row: number, col: number, dr: number, dc: number, color: Stone): number {
  return 1 + countDir(board, row, col, dr, dc, color) + countDir(board, row, col, -dr, -dc, color);
}

function hasExactFive(board: Board, row: number, col: number): boolean {
  const axes: [number, number][] = [[0, 1], [1, 0], [1, 1], [1, -1]];
  for (const [dr, dc] of axes) {
    if (getLineLength(board, row, col, dr, dc, 'black') === 5) return true;
  }
  return false;
}

function hasOverline(board: Board, row: number, col: number): boolean {
  const axes: [number, number][] = [[0, 1], [1, 0], [1, 1], [1, -1]];
  for (const [dr, dc] of axes) {
    if (getLineLength(board, row, col, dr, dc, 'black') > 5) return true;
  }
  return false;
}

function countFoursAll(board: Board, row: number, col: number): number {
  const axes: [number, number][] = [[0, 1], [1, 0], [1, 1], [1, -1]];
  let totalFours = 0;

  for (const [dr, dc] of axes) {
    const winningSpots: { r: number; c: number }[] = [];

    for (let i = -4; i <= 4; i++) {
      if (i === 0) continue;
      const r = row + dr * i;
      const c = col + dc * i;
      if (inBounds(r, c) && board[r][c] === null) {
        board[r][c] = 'black';
        if (getLineLength(board, r, c, dr, dc, 'black') === 5) {
          winningSpots.push({ r, c });
        }
        board[r][c] = null;
      }
    }

    if (winningSpots.length === 1) {
      totalFours += 1;
    } else if (winningSpots.length >= 2) {
      const isContiguousOpenFour = getLineLength(board, row, col, dr, dc, 'black') === 4;
      totalFours += isContiguousOpenFour ? 1 : winningSpots.length;
    }
  }

  return totalFours;
}

function countThreesAll(board: Board, row: number, col: number): number {
  const axes: [number, number][] = [[0, 1], [1, 0], [1, 1], [1, -1]];
  let openThreeCount = 0;

  for (const [dr, dc] of axes) {
    let hasOpenThreeOnAxis = false;

    for (let i = -4; i <= 4; i++) {
      if (i === 0) continue;
      const r = row + dr * i;
      const c = col + dc * i;
      if (inBounds(r, c) && board[r][c] === null) {
        board[r][c] = 'black';
        if (getLineLength(board, r, c, dr, dc, 'black') === 4) {
          let winSpotCount = 0;
          for (let j = -4; j <= 4; j++) {
            if (j === 0) continue;
            const r2 = r + dr * j;
            const c2 = c + dc * j;
            if (inBounds(r2, c2) && board[r2][c2] === null) {
              board[r2][c2] = 'black';
              if (getLineLength(board, r2, c2, dr, dc, 'black') === 5) {
                winSpotCount++;
              }
              board[r2][c2] = null;
            }
          }
          if (winSpotCount === 2 && !hasOverline(board, r, c)) {
            hasOpenThreeOnAxis = true;
          }
        }
        board[r][c] = null;
        if (hasOpenThreeOnAxis) break;
      }
    }

    if (hasOpenThreeOnAxis) openThreeCount++;
  }

  return openThreeCount;
}

export function isForbidden(board: Board, row: number, col: number): boolean {
  if (!inBounds(row, col) || board[row][col] !== null) return false;

  board[row][col] = 'black';

  if (hasExactFive(board, row, col)) {
    board[row][col] = null;
    return false;
  }

  if (hasOverline(board, row, col)) {
    board[row][col] = null;
    return true;
  }

  if (countFoursAll(board, row, col) >= 2) {
    board[row][col] = null;
    return true;
  }

  if (countThreesAll(board, row, col) >= 2) {
    board[row][col] = null;
    return true;
  }

  board[row][col] = null;
  return false;
}

export function getForbiddenMoves(board: Board): Set<string> {
  const set = new Set<string>();
  const cleanBoard = cloneBoard(board);

  for (let r = 0; r < BOARD_SIZE; r++) {
    for (let c = 0; c < BOARD_SIZE; c++) {
      if (cleanBoard[r][c] === null && isForbidden(cleanBoard, r, c)) {
        set.add(`${r}-${c}`);
      }
    }
  }
  return set;
}