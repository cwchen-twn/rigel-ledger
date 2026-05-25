import { render } from 'solid-js/web';
import App from './App';
import 'bootstrap/dist/css/bootstrap.min.css';
import 'bootstrap-icons/font/bootstrap-icons.css';
import 'bootstrap/dist/js/bootstrap.bundle.min.js';

const root = document.getElementById('app');
if (root) {
  render(() => <App />, root);
}
