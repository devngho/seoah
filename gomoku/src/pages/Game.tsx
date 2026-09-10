import { createEffect, createMemo, createSignal, Show, For } from "solid-js";
import {
  BOARD_SIZE,
  type Stone,
  type Board,
  createEmptyBoard,
  getForbiddenMoves,
} from "../game/engine";
import {
  createGame,
  placeMove,
  requestBestMove,
  type GameState,
  type GameStatus,
  type RenjuConfig,
  type Stone as ApiStone,
} from "../game/api";

type Move = { row: number; col: number; color: Stone };
type GameMode = "black" | "white" | "ai-vs-ai" | null;

const DEFAULT_RENJU: RenjuConfig = {
  enabled: true,
  forbidDoubleThree: true,
  forbidDoubleFour: true,
  forbidOverline: true,
};

function fromApiStone(v: ApiStone): Stone {
  if (v === 1) return "black";
  if (v === 2) return "white";
  return null;
}

function fromApiBoard(board: ApiStone[][]): Board {
  return board.map((row) => row.map(fromApiStone));
}

function isAiTurn(mode: GameMode, turn: Stone): boolean {
  if (mode === "ai-vs-ai") return true;
  if (mode === "black") return turn === "white";
  if (mode === "white") return turn === "black";
  return false;
}

export default function Game() {
  const [gameId, setGameId] = createSignal<string | null>(null);
  const [board, setBoard] = createSignal<Board>(createEmptyBoard());
  const [mode, setMode] = createSignal<GameMode>(null);
  const [currentTurn, setCurrentTurn] = createSignal<Stone>("black");
  const [status, setStatus] = createSignal<GameStatus>("playing");
  const [winner, setWinner] = createSignal<Stone | "draw" | null>(null);
  const [forbiddenPoints, setForbiddenPoints] = createSignal<
    [number, number][]
  >([]);
  const [moveHistory, setMoveHistory] = createSignal<Move[]>([]);

  const [starting, setStarting] = createSignal(false);
  const [thinking, setThinking] = createSignal(false);
  const [moveError, setMoveError] = createSignal<string | null>(null);
  const [isReviewing, setIsReviewing] = createSignal(false);

  const forbiddenSet = createMemo(
    () => new Set(forbiddenPoints().map(([r, c]) => `${r}-${c}`)),
  );

  function applyState(state: GameState) {
    setBoard(fromApiBoard(state.board));
    setCurrentTurn(fromApiStone(state.turn));
    setStatus(state.status);
    if (state.status === "draw") {
      setWinner("draw");
    } else if (state.status === "win") {
      setWinner(fromApiStone(state.winner ?? 0));
    } else {
      setWinner(null);
    }
    setMoveHistory(
      state.history.map((m) => ({
        row: m.row,
        col: m.col,
        color: fromApiStone(m.color),
      })),
    );
    setForbiddenPoints(state.forbiddenMoves);
  }

  async function selectMode(m: GameMode) {
    setStarting(true);
    setMoveError(null);
    try {
      const state = await createGame({ renju: DEFAULT_RENJU });
      setGameId(state.gameId);
      applyState(state);
      setMode(m);
    } catch (e) {
      setMoveError(e instanceof Error ? e.message : "게임 시작 실패");
    } finally {
      setStarting(false);
    }
  }

  function handleCellClick(row: number, col: number) {
    const m = mode();
    const id = gameId();
    if (!m || m === "ai-vs-ai" || !id) return;
    if (status() !== "playing" || thinking()) return;
    if (currentTurn() !== m) return;

    setMoveError(null);
    placeMove(id, row, col)
      .then((state) => {
        if (gameId() !== id) return;
        applyState(state);
      })
      .catch((e) => {
        if (gameId() !== id) return;
        setMoveError(e instanceof Error ? e.message : "착수 실패");
      });
  }

  createEffect(() => {
    const m = mode();
    const id = gameId();
    const turn = currentTurn();

    if (!m || !id || status() !== "playing" || !isAiTurn(m, turn)) return;

    setThinking(true);
    setMoveError(null);

    requestBestMove(id, true)
      .then((state) => {
        if (gameId() !== id) return;
        applyState(state);
      })
      .catch((e) => {
        if (gameId() !== id) return;
        setMoveError(e instanceof Error ? e.message : "엔진 요청 실패");
      })
      .finally(() => {
        if (gameId() !== id) return;
        setThinking(false);
      });
  });

  function restart() {
    setGameId(null);
    setBoard(createEmptyBoard());
    setMode(null);
    setCurrentTurn("black");
    setStatus("playing");
    setWinner(null);
    setForbiddenPoints([]);
    setMoveHistory([]);
    setThinking(false);
    setMoveError(null);
    setIsReviewing(false);
  }

  return (
    <div class="game">
      <div class="home__board-overlay" />
      <Show
        when={mode()}
        fallback={<ModeSelect onSelect={selectMode} starting={starting()} />}
      >
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
            <Show when={moveError()}>
              <p class="ai-error">{moveError()}</p>
            </Show>
            <BoardView
              board={board()}
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

function ModeSelect(props: {
  onSelect: (m: GameMode) => void;
  starting: boolean;
}) {
  return (
    <div class="color-select">
      <h2 class="color-select__title">대국 방식을 선택하세요</h2>
      <div class="color-select__options">
        <button
          type="button"
          class="color-select__btn"
          disabled={props.starting}
          onClick={() => props.onSelect("black")}
        >
          <span class="stone stone--black" />
          흑돌로 두기
        </button>
        <button
          type="button"
          class="color-select__btn"
          disabled={props.starting}
          onClick={() => props.onSelect("white")}
        >
          <span class="stone stone--white" />
          백돌로 두기
        </button>
        <button
          type="button"
          class="color-select__btn"
          disabled={props.starting}
          onClick={() => props.onSelect("ai-vs-ai")}
        >
          <span class="color-select__pair">
            <span class="stone stone--black" />
            <span class="stone stone--white" />
          </span>
          엔진끼리 대국 관전
        </button>
      </div>
      <p class="color-select__hint">
        {props.starting ? "게임을 생성하는 중입니다..." : "흑돌이 먼저 둡니다"}
      </p>
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

  const currentTurn = () => (step() % 2 === 0 ? "black" : "white");

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