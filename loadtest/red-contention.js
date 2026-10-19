import http from 'k6/http';
import { check } from 'k6';
import { Counter, Gauge } from 'k6/metrics';
import { BASE_URL, serverErrors } from './lib/config.js';
import { resetLimits } from './lib/checks.js';

// N buyers with N different client keys fire at the same moment for the Red
// set. Exactly one may win; everyone else must get 409; nobody may get a 5xx.
// DEGRADED=1 is for the Redis-down run (see redis-down.sh): it also bounds p95.
const N = parseInt(__ENV.N || '100', 10);
const DEGRADED = __ENV.DEGRADED === '1';

const winners = new Counter('red_winners');
const rejected = new Counter('red_rejected');
const winnerIndex = new Gauge('red_winner_index');
const stampGauge = new Gauge('red_stamp');

const thresholds = {
  red_winners: ['count>0', 'count<2'],
  server_errors: ['count<1'],
  checks: ['rate==1'],
};
if (DEGRADED) {
  thresholds.http_req_duration = ['p(95)<1000'];
}

export const options = {
  scenarios: {
    race: { executor: 'shared-iterations', vus: 1, iterations: 1 },
  },
  batch: N,
  batchPerHost: N,
  thresholds,
};

export function setup() {
  resetLimits();
  winners.add(0);
  rejected.add(0);
}

export default function () {
  const stamp = Date.now();
  const body = JSON.stringify({ items: [{ productId: 'red', quantity: 1 }] });
  const requests = [];
  for (let i = 0; i < N; i++) {
    requests.push([
      'POST',
      `${BASE_URL}/api/calculate`,
      body,
      { headers: { 'Content-Type': 'application/json', 'X-Client-Key': `race-${i}-${stamp}` }, tags: { name: 'POST /api/calculate (red race)' } },
    ]);
  }
  const responses = http.batch(requests);

  let won = 0;
  let lost = 0;
  responses.forEach((res, i) => {
    if (res.status === 200) {
      won++;
      winners.add(1);
      winnerIndex.add(i);
      stampGauge.add(stamp);
    } else if (res.status === 409) {
      lost++;
      rejected.add(1);
    } else if (res.status >= 500) {
      serverErrors.add(1);
    }
  });
  check(null, {
    'exactly one winner': () => won === 1,
    'every other buyer rejected with 409': () => lost === N - 1,
  });
}

export function handleSummary(data) {
  const v = (name, key) => (data.metrics[name] && data.metrics[name].values[key]) || 0;
  const lines = [
    `red-contention N=${N}${DEGRADED ? ' (degraded)' : ''}`,
    `  winners:   ${v('red_winners', 'count')}`,
    `  rejected:  ${v('red_rejected', 'count')}`,
    `  5xx:       ${v('server_errors', 'count')}`,
    `  http p95:  ${v('http_req_duration', 'p(95)').toFixed(1)} ms`,
    `  http max:  ${v('http_req_duration', 'max').toFixed(1)} ms`,
    '',
  ];
  const out = { stdout: lines.join('\n') };
  if (v('red_winners', 'count') === 1) {
    out['/scripts/out/winner.txt'] = `race-${v('red_winner_index', 'value')}-${v('red_stamp', 'value')}`;
  }
  return out;
}
