const demo = document.querySelector('#demo');
const playback = document.querySelector('#playback');
const reducedMotion = window.matchMedia('(prefers-reduced-motion: reduce)');
let playing = false;

function setPlayback(value) {
  playing = value;
  demo.src = playing ? '/demo.gif' : '/demo.png';
  playback.textContent = playing ? 'Pause demo' : 'Play demo';
  playback.setAttribute('aria-label', playing ? 'Pause demo' : 'Play demo');
}
setPlayback(!reducedMotion.matches);
playback.addEventListener('click', () => setPlayback(!playing));
reducedMotion.addEventListener('change', () => setPlayback(!reducedMotion.matches));

document.querySelector('#copy').addEventListener('click', async () => {
  const command = document.querySelector('#simple-command');
  const feedback = document.querySelector('#feedback');
  try {
    await navigator.clipboard.writeText(command.textContent);
    feedback.textContent = 'Copied. Paste into your terminal to play.';
  } catch {
    const range = document.createRange();
    range.selectNodeContents(command);
    const selection = window.getSelection();
    selection.removeAllRanges();
    selection.addRange(range);
    feedback.textContent = 'Copy the selected command, then paste it into your terminal.';
  }
});

async function updateOnline() {
  if (document.hidden) return;
  const presence = document.querySelector('#presence');
  const label = document.querySelector('#online');
  try {
    const response = await fetch('/api/online', { signal: AbortSignal.timeout(5000), cache: 'no-store' });
    if (!response.ok) throw new Error('Unavailable');
    const { online } = await response.json();
    if (!Number.isSafeInteger(online) || online < 0) throw new Error('Invalid count');
    label.textContent = `${online} ${online === 1 ? 'Trainer' : 'Trainers'} online`;
    presence.classList.add('live');
  } catch {
    label.textContent = 'Online count unavailable';
    presence.classList.remove('live');
  }
}
updateOnline();
setInterval(updateOnline, 15000);
document.addEventListener('visibilitychange', updateOnline);
