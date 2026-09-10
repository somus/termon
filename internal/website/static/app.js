const demo = document.querySelector('#demo');
const playback = document.querySelector('#playback');
const reducedMotion = window.matchMedia('(prefers-reduced-motion: reduce)');
let playing = false;

let visitID;
if (document.body.dataset.analyticsEnabled === 'true') {
  try {
    visitID = crypto.randomUUID();
  } catch {
    // Analytics stays off when secure random IDs are unavailable.
  }
}

async function track(event, outcome) {
  if (!visitID) return;
  try {
    await fetch('/api/events', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      credentials: 'omit',
      mode: 'same-origin',
      keepalive: true,
      body: JSON.stringify({ visit_id: visitID, event, outcome }),
    });
  } catch {
    // Analytics must never interrupt the page's controls.
  }
}
void track('website:page_view');

function updatePlayback() {
  playing = !demo.paused;
  playback.textContent = playing ? 'Pause demo' : 'Play demo';
  playback.setAttribute('aria-label', playing ? 'Pause demo' : 'Play demo');
}
async function setPlayback(value) {
  if (value) {
    try {
      await demo.play();
    } catch {
      // A blocked autoplay leaves the poster and manual play control available.
    }
  } else {
    demo.pause();
  }
  updatePlayback();
}
demo.addEventListener('play', updatePlayback);
demo.addEventListener('pause', updatePlayback);
setPlayback(!reducedMotion.matches);
playback.addEventListener('click', () => {
  const requested = !playing;
  void setPlayback(requested);
  void track('website:demo_toggle', requested ? 'play' : 'pause');
});
reducedMotion.addEventListener('change', () => setPlayback(!reducedMotion.matches));

document.querySelector('#copy').addEventListener('click', async () => {
  const command = document.querySelector('#simple-command');
  const feedback = document.querySelector('#feedback');
  try {
    await navigator.clipboard.writeText(command.textContent);
    feedback.textContent = 'Copied. Paste into your terminal to play.';
    void track('website:command_copy', 'success');
  } catch {
    const range = document.createRange();
    range.selectNodeContents(command);
    const selection = window.getSelection();
    selection.removeAllRanges();
    selection.addRange(range);
    feedback.textContent = 'Copy the selected command, then paste it into your terminal.';
    void track('website:command_copy', 'fallback');
  }
});

const instructions = document.querySelector('.connection details');
let instructionsTracked = false;
instructions.addEventListener('toggle', () => {
  if (instructions.open && !instructionsTracked) {
    instructionsTracked = true;
    void track('website:instructions_open');
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
