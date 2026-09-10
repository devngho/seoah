import { createEffect, createMemo, createSignal, Show, For, untrack } from "solid-js";
import { createStore, unwrap } from "solid-js/store";
import {
  BOARD_SIZE,
  type Stone,
  type Board,
  createEmptyBoard,
  checkWin,
  isBoardFull,
  isForbidden,
  getForbiddenMoves,
} from "../game/engine";
import { getBestMove } from "../game/api";

type Move = { row: number; col: number; color: Stone };
type GameMode = "black" | "white" | "ai-vs-ai" | null;

function toApiStone(color: Stone): 1 | 2 {
  return color === "black" ? 1 : 2;
}

function toApiBoard(board: Board): (0 | 1 | 2)[][] {
  return board.map((row) =>
    row.map((cell) => {
      if (cell === "black") return 1;
      if (cell === "white") return 2;
      return 0;
    }),
  );
}

function isAiTurn(mode: GameMode, turn: Stone): boolean {
  if (mode === "ai-vs-ai") return true;
  if (mode === "black") return turn === "white";
  if (mode === "white") return turn === "black";
  return false;
}

export default function Game() {
  const [board, setBoard] = createStore<Board>(createEmptyBoard());
  const [mode, setMode] = createSignal<GameMode>(null);
  const [currentTurn, setCurrentTurn] = createSignal<Stone>("black");
  const [winner, setWinner] = createSignal<Stone | "draw" | null>(null);
  const [thinking, setThinking] = createSignal(false);
  const [aiError, setAiError] = createSignal<string | null>(null);
  const [moveHistory, setMoveHistory] = createSignal<Move[]>([]);
  const [isReviewing, setIsReviewing] = createSignal(false);

  const [gameId, setGameId] = createSignal(0);

  // 대국 중 흑돌 차례일 때 금수 위치 계산
  const forbiddenSet = createMemo(() => {
    if (currentTurn() !== "black" || winner()) return new Set<string>();
    const plainBoard = board.map((row) => [...row]);
    return getForbiddenMoves(plainBoard);
  });

  function placeStone(row: number, col: number, color: Stone) {
    if (board[row][col] !== null || winner()) return;
    setBoard(row, col, color);
    setMoveHistory((h) => [...h, { row, col, color }]);

    if (checkWin(unwrap(board), row, col, color)) {
      setWinner(color);
      return;
    }
    if (isBoardFull(unwrap(board))) {
      setWinner("draw");
      return;
    }
    setCurrentTurn(color === "black" ? "white" : "black");
  }

  function handleCellClick(row: number, col: number) {
    const m = mode();
    if (!m || m === "ai-vs-ai" || winner() || thinking()) return;
    if (currentTurn() !== m) return;

    // 흑돌 착수 시 금수 자리는 착수 불가
    if (
      m === "black" &&
      isForbidden(
        board.map((r) => [...r]),
        row,
        col,
      )
    )
      return;

    placeStone(row, col, m);
  }

  createEffect(() => {
    const m = mode();
    const turn = currentTurn();
    const myGameId = gameId();

    if (!m || winner() || !isAiTurn(m, turn)) return;

    setThinking(true);
    setAiError(null);

    const lastMove = untrack(() => moveHistory().at(-1));

    getBestMove(
      toApiBoard(unwrap(board)),
      toApiStone(turn),
      {
      renju: {
        enabled: true,
        forbidDoubleThree: true,
        forbidDoubleFour: true,
        forbidOverline: true,
      }},
      lastMove)
      .then((result) => {
        if (gameId() !== myGameId) return;

        if (result.noMove) {
          setWinner("draw");
          return;
        }
        placeStone(result.y, result.x, turn);
      })
      .catch((e) => {
        if (gameId() !== myGameId) return;
        setAiError(e instanceof Error ? e.message : "엔진 요청 실패");
      })
      .finally(() => {
        if (gameId() !== myGameId) return;
        setThinking(false);
      });
  });

  function selectMode(m: GameMode) {
    setMode(m);
    setCurrentTurn("black");
  }

  function restart() {
    setGameId((id) => id + 1);
    setBoard(createEmptyBoard());
    setMode(null);
    setCurrentTurn("black");
    setWinner(null);
    setThinking(false);
    setAiError(null);
    setMoveHistory([]);
    setIsReviewing(false);
  }

  return (
    <div class="game">
      <div class="home__board-overlay" />
      <Show when={mode()} fallback={<ModeSelect onSelect={selectMode} />}>
        <Show
          when={!isReviewing()}
          fallback={
            <ReplayView
              history={moveHistory()}
              onExit={() => setIsReviewing(false)}
            />
          }
        >
          <div class="game__content">
            <TurnBar
              mode={mode()}
              turn={currentTurn()}
              thinking={thinking()}
              winner={winner()}
            />
            <Show when={aiError()}>
              <p class="ai-error">{aiError()}</p>
            </Show>
            <BoardView
              board={board}
              onCellClick={handleCellClick}
              forbiddenSet={forbiddenSet()}
            />
            <Show when={winner()}>
              <div class="result-banner">
                <p class="result-banner__text">
                  {winner() === "draw"
                    ? "무승부입니다"
                    : mode() === "ai-vs-ai"
                      ? `${winner() === "black" ? "흑" : "백"} 엔진이 승리했습니다`
                      : winner() === mode()
                        ? "승리했습니다"
                        : "패배했습니다"}
                </p>
                <div class="result-banner__actions">
                  <button
                    type="button"
                    class="home__start-btn"
                    onClick={restart}
                  >
                    다시 하기
                  </button>
                  <button
                    type="button"
                    class="home__start-btn home__start-btn--ghost"
                    onClick={() => setIsReviewing(true)}
                  >
                    복기하기
                  </button>
                </div>
              </div>
            </Show>
          </div>
        </Show>
      </Show>
    </div>
  );
}

function ModeSelect(props: { onSelect: (m: GameMode) => void }) {
  return (
    <div class="color-select">
      <h2 class="color-select__title">대국 방식을 선택하세요</h2>
      <div class="color-select__options">
        <button
          type="button"
          class="color-select__btn"
          onClick={() => props.onSelect("black")}
        >
          <span class="stone stone--black" />
          흑돌로 두기
        </button>
        <button
          type="button"
          class="color-select__btn"
          onClick={() => props.onSelect("white")}
        >
          <span class="stone stone--white" />
          백돌로 두기
        </button>
        <button
          type="button"
          class="color-select__btn"
          onClick={() => props.onSelect("ai-vs-ai")}
        >
          <span class="color-select__pair">
            <span class="stone stone--black" />
            <span class="stone stone--white" />
          </span>
          엔진끼리 대국 관전
        </button>
      </div>
      <p class="color-select__hint">흑돌이 먼저 둡니다</p>
    </div>
  );
}

function TurnBar(props: {
  mode: GameMode;
  turn: Stone;
  thinking: boolean;
  winner: Stone | "draw" | null;
}) {
  const label = () => {
    if (props.winner) return "대국 종료";
    if (props.mode === "ai-vs-ai") {
      const colorLabel = props.turn === "black" ? "흑" : "백";
      return props.thinking
        ? `${colorLabel} 엔진이 생각 중입니다...`
        : `${colorLabel} 엔진 차례입니다`;
    }
    const isMyTurn = props.turn === props.mode;
    if (isMyTurn) return "당신의 차례입니다";
    return props.thinking ? "상대가 생각 중입니다..." : "상대의 차례입니다";
  };
  return (
    <div class="turn-bar">
      <span class={`stone stone--${props.turn ?? "black"} turn-bar__stone`} />
      <span>{label()}</span>
    </div>
  );
}

function BoardView(props: {
  board: Board;
  onCellClick: (row: number, col: number) => void;
  lastMove?: { row: number; col: number } | null;
  forbiddenSet?: Set<string>;
}) {
  const starPoints = [3, 7, 11];
  const isStar = (r: number, c: number) =>
    starPoints.includes(r) && starPoints.includes(c);
  const isLast = (r: number, c: number) =>
    props.lastMove?.row === r && props.lastMove?.col === c;

  return (
    <div class="board">
      <For each={props.board}>
        {(row, r) => (
          <For each={row}>
            {(cell, c) => {
              const isForbiddenPoint = () =>
                !cell && props.forbiddenSet?.has(`${r()}-${c()}`);

              return (
                <button
                  type="button"
                  class="board__point"
                  style={{
                    top: `${(r() / (BOARD_SIZE - 1)) * 100}%`,
                    left: `${(c() / (BOARD_SIZE - 1)) * 100}%`,
                  }}
                  onClick={() => props.onCellClick(r(), c())}
                >
                  <Show when={isStar(r(), c()) && !cell && !isForbiddenPoint()}>
                    <span class="board__star" />
                  </Show>
                  <Show when={isForbiddenPoint()}>
                    <span class="board__forbidden">✕</span>
                  </Show>
                  <Show when={cell}>
                    <span
                      class={`stone stone--${cell}`}
                      classList={{ "stone--last": isLast(r(), c()) }}
                    />
                  </Show>
                </button>
              );
            }}
          </For>
        )}
      </For>
    </div>
  );
}

function ReplayView(props: { history: Move[]; onExit: () => void }) {
  const [step, setStep] = createSignal(props.history.length);

  const board = createMemo<Board>(() => {
    const b = createEmptyBoard();
    for (let i = 0; i < step(); i++) {
      const m = props.history[i];
      b[m.row][m.col] = m.color;
    }
    return b;
  });

  // 복기 단계에서 다음 둘 차례 계산 (0수, 2수, 4수... = 흑 차례)
  const currentTurn = () => (step() % 2 === 0 ? "black" : "white");

  // 복기 중 흑 차례일 때만 금수 위치 계산
  const forbiddenSet = createMemo(() => {
    if (currentTurn() !== "black" || step() >= props.history.length) {
      return new Set<string>();
    }
    const plainBoard = board().map((row) => [...row]);
    return getForbiddenMoves(plainBoard);
  });

  const lastMove = () => (step() > 0 ? props.history[step() - 1] : null);

  const goFirst = () => setStep(0);
  const goPrev = () => setStep((s) => Math.max(0, s - 1));
  const goNext = () => setStep((s) => Math.min(props.history.length, s + 1));
  const goLast = () => setStep(props.history.length);

  return (
    <div class="game__content">
      <div class="replay-bar">
        <span>
          복기 · {step()} / {props.history.length}수 (
          {step() < props.history.length
            ? `${currentTurn() === "black" ? "흑" : "백"} 차례`
            : "종료"}
          )
        </span>
      </div>
      <BoardView
        board={board()}
        onCellClick={() => {}}
        lastMove={lastMove()}
        forbiddenSet={forbiddenSet()}
      />
      <div class="replay-controls">
        <button
          type="button"
          class="replay-btn"
          onClick={goFirst}
          disabled={step() === 0}
        >
          처음
        </button>
        <button
          type="button"
          class="replay-btn"
          onClick={goPrev}
          disabled={step() === 0}
        >
          이전
        </button>
        <button
          type="button"
          class="replay-btn"
          onClick={goNext}
          disabled={step() === props.history.length}
        >
          다음
        </button>
        <button
          type="button"
          class="replay-btn"
          onClick={goLast}
          disabled={step() === props.history.length}
        >
          마지막
        </button>
      </div>
      <button
        type="button"
        class="home__start-btn home__start-btn--ghost"
        onClick={props.onExit}
      >
        복기 종료
      </button>
    </div>
  );
}
