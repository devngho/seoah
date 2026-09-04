/* @refresh reload */
import { render } from 'solid-js/web'
import { Router, Route, type RouteSectionProps } from '@solidjs/router';

import Home from './pages/Home';
import Game from './pages/Game';
import './index.css'

function App(props: RouteSectionProps) {
  return (
    <div>
      {props.children}
    </div>
  );
}

render(
  () => (
    <Router root={App}>
      <Route path="/" component={Home} />
      <Route path="/game" component={Game} />
    </Router>
  ),
  document.getElementById('root') as HTMLElement
);