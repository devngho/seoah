import { createEffect, createSignal, onCleanup, Show, For } from 'solid-js';
import { createStore, unwrap } from 'solid-js/store';
import {
  BOARD_SIZE,
  type Stone,
  type Board,
  createEmptyBoard,
  checkWin,
  isBoardFull,
  bestMove,
} from '../game/engine';

export default function Game() {
  const [board, setBoard] = createStore<Board>(createEmptyBoard());
  const [playerColor, setPlayerColor] = createSignal<Stone>(null);
  const [currentTurn, setCurrentTurn] = createSignal<Stone>('black');
  const [winner, setWinner] = createSignal<Stone | 'draw' | null>(null);
  const [thinking, setThinking] = createSignal(false);

  function placeStone(row: number, col: number, color: Stone) {
    if (board[row][col] !== null || winner()) return;
    setBoard(row, col, color);

    if (checkWin(unwrap(board), row, col, color)) {
      setWinner(color);
      return;
    }
    if (isBoardFull(unwrap(board))) {
      setWinner('draw');
      return;
    }
    setCurrentTurn(color === 'black' ? 'white' : 'black');
  }

  function handleCellClick(row: number, col: number) {
    const me = playerColor();
    if (!me || winner() || currentTurn() !== me || thinking()) return;
    placeStone(row, col, me);
  }

  // AI 턴 자동 진행
  createEffect(() => {
    const me = playerColor();
    const turn = currentTurn();
    if (!me || winner() || turn === me) return;

    setThinking(true);
    const timer = setTimeout(() => {
      const move = bestMove(unwrap(board), turn, me);
      if (move) placeStone(move.row, move.col, turn);
      setThinking(false);
    }, 450);

    onCleanup(() => clearTimeout(timer));
  });

  function selectColor(color: Stone) {
    setPlayerColor(color);
    setCurrentTurn('black'); // 오목은 항상 흑이 선
  }

  function restart() {
    setBoard(createEmptyBoard());
    setPlayerColor(null);
    setCurrentTurn('black');
    setWinner(null);
    setThinking(false);
  }

  return (
    <div class="game">
      <div class="home__board-overlay" />
      <Show when={playerColor()} fallback={<ColorSelect onSelect={selectColor} />}>
        <div class="game__content">
          <TurnBar me={playerColor()!} turn={currentTurn()} thinking={thinking()} winner={winner()} />
          <BoardView board={board} onCellClick={handleCellClick} />
          <Show when={winner()}>
            <div class="result-banner">
              <p class="result-banner__text">
                {winner() === 'draw'
                  ? '무승부입니다'
                  : winner() === playerColor()
                  ? '승리했습니다'
                  : '패배했습니다'}
              </p>
              <button class="home__start-btn" onClick={restart}>
                다시 하기
              </button>
            </div>
          </Show>
        </div>
      </Show>
    </div>
  );
}

function ColorSelect(props: { onSelect: (c: Stone) => void }) {
  return (
    <div class="color-select">
      <h2 class="color-select__title">돌 색을 선택하세요</h2>
      <div class="color-select__options">
        <button class="color-select__btn" onClick={() => props.onSelect('black')}>
          <span class="stone stone--black" />
          흑돌
        </button>
        <button class="color-select__btn" onClick={() => props.onSelect('white')}>
          <span class="stone stone--white" />
          백돌
        </button>
      </div>
      <p class="color-select__hint">흑돌이 먼저 둡니다</p>
    </div>
  );
}

function TurnBar(props: { me: Stone; turn: Stone; thinking: boolean; winner: Stone | 'draw' | null }) {
  const label = () => {
    if (props.winner) return '대국 종료';
    if (props.turn === props.me) return '당신의 차례입니다';
    return props.thinking ? '상대가 생각 중입니다...' : '상대의 차례입니다';
  };
  return (
    <div class="turn-bar">
      <span class={`stone stone--${props.turn ?? 'black'} turn-bar__stone`} />
      <span>{label()}</span>
    </div>
  );
}

function BoardView(props: { board: Board; onCellClick: (row: number, col: number) => void }) {
  const starPoints = [3, 7, 11];
  const isStar = (r: number, c: number) => starPoints.includes(r) && starPoints.includes(c);

  return (
    <div class="board">
      <For each={props.board}>
        {(row, r) => (
          <For each={row}>
            {(cell, c) => (
              <button
                class="board__point"
                style={{
                  top: `${(r() / (BOARD_SIZE - 1)) * 100}%`,
                  left: `${(c() / (BOARD_SIZE - 1)) * 100}%`,
                }}
                onClick={() => props.onCellClick(r(), c())}
              >
                <Show when={isStar(r(), c()) && !cell}>
                  <span class="board__star" />
                </Show>
                <Show when={cell}>
                  <span class={`stone stone--${cell}`} />
                </Show>
              </button>
            )}
          </For>
        )}
      </For>
    </div>
  );
}