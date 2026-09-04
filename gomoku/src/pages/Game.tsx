import { createEffect, createSignal, Show, For } from 'solid-js';
import { createStore, unwrap } from 'solid-js/store';
import {
  BOARD_SIZE,
  type Stone,
  type Board,
  createEmptyBoard,
  checkWin,
  isBoardFull,
} from '../game/engine';
import { getBestMove } from '../game/api';

function toApiStone(color: Stone): 1 | 2 {
  return color === 'black' ? 1 : 2;
}

function toApiBoard(board: Board): (0 | 1 | 2)[][] {
  return board.map((row) =>
    row.map((cell) => (cell === 'black' ? 1 : cell === 'white' ? 2 : 0))
  );
}

export default function Game() {
  const [board, setBoard] = createStore<Board>(createEmptyBoard());
  const [playerColor, setPlayerColor] = createSignal<Stone>(null);
  const [currentTurn, setCurrentTurn] = createSignal<Stone>('black');
  const [winner, setWinner] = createSignal<Stone | 'draw' | null>(null);
  const [thinking, setThinking] = createSignal(false);
  const [aiError, setAiError] = createSignal<string | null>(null);

  const [gameId, setGameId] = createSignal(0);

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

  createEffect(() => {
    const me = playerColor();
    const turn = currentTurn();
    const myGameId = gameId();

    if (!me || winner() || turn === me) return;

    setThinking(true);
    setAiError(null);

    getBestMove(toApiBoard(unwrap(board)), toApiStone(turn))
      .then((result) => {
        if (gameId() !== myGameId) return; // 그 사이 재시작/색상변경 됐으면 무시

        if (result.noMove) {
          setWinner('draw');
          return;
        }
        placeStone(result.y, result.x, turn);
      })
      .catch((e) => {
        if (gameId() !== myGameId) return;
        setAiError(e instanceof Error ? e.message : '엔진 요청 실패');
      })
      .finally(() => {
        if (gameId() !== myGameId) return;
        setThinking(false);
      });
  });

  function selectColor(color: Stone) {
    setPlayerColor(color);
    setCurrentTurn('black');
  }

  function restart() {
    setGameId((id) => id + 1);
    setBoard(createEmptyBoard());
    setPlayerColor(null);
    setCurrentTurn('black');
    setWinner(null);
    setThinking(false);
    setAiError(null);
  }

  return (
    <div class="game">
      <div class="home__board-overlay" />
      <Show when={playerColor()} fallback={<ColorSelect onSelect={selectColor} />}>
        <div class="game__content">
          <TurnBar me={playerColor()!} turn={currentTurn()} thinking={thinking()} winner={winner()} />
          <Show when={aiError()}>
            <p class="ai-error">{aiError()}</p>
          </Show>
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