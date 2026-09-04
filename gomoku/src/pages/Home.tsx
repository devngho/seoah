import { useNavigate } from '@solidjs/router';

export default function Home() {
  const navigate = useNavigate();

  return (
    <div class="home">
      <div class="home__board-overlay" />
      <div class="home__content">
        <div class="home__stones">
          <span class="stone stone--black" />
          <span class="stone stone--white" />
          <span class="stone stone--black" />
        </div>
        <h1 class="home__title">오목</h1>
        <p class="home__subtitle">다섯 개를 먼저 이으면 승리합니다</p>
        <button class="home__start-btn" onClick={() => navigate('/game')}>
          시작하기
        </button>
      </div>
    </div>
  );
}