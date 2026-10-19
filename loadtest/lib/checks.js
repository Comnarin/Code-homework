import http from 'k6/http';
import { check } from 'k6';
import { BASE_URL, serverErrors } from './config.js';

const GREEN_BLUE_ORANGE_PINK = ['green', 'blue', 'orange', 'pink'];
const JSON_HEADERS = { 'Content-Type': 'application/json' };

function pick(list) {
  return list[Math.floor(Math.random() * list.length)];
}

function track(res) {
  if (res.status >= 500) {
    serverErrors.add(1);
  }
  return res;
}

// Call from every scenario's setup(): frees the Red set (both Postgres and
// Redis) so runs start from the same state, and makes sure the counter exists.
export function resetLimits() {
  serverErrors.add(0);
  const res = http.post(`${BASE_URL}/api/test/reset-limits`);
  if (!check(res, { 'reset-limits returned 204': (r) => r.status === 204 })) {
    throw new Error(
      `POST /api/test/reset-limits returned ${res.status}; start the stack with ENABLE_TEST_TOOLS=true (./loadtest/run.sh does)`
    );
  }
}

function calculate(items, key) {
  const headers = Object.assign({}, JSON_HEADERS);
  if (key) {
    headers['X-Client-Key'] = key;
  }
  const body = { items };
  // About half of the orders use the seeded active demo member.
  if (Math.random() < 0.5) {
    body.memberCard = 'MEM0001';
  }
  return track(http.post(`${BASE_URL}/api/calculate`, JSON.stringify(body), { headers, tags: { name: 'POST /api/calculate' } }));
}

// One iteration of realistic traffic: 60% menu, 10% promotions, 30% calculate.
// About 2% of the calculates ask for Red with a random key (200 or 409).
export function exerciseApi() {
  const roll = Math.random();
  if (roll < 0.6) {
    const res = track(http.get(`${BASE_URL}/api/products`, { tags: { name: 'GET /api/products' } }));
    check(res, { 'products 200': (r) => r.status === 200 });
  } else if (roll < 0.7) {
    const res = track(http.get(`${BASE_URL}/api/promotions`, { tags: { name: 'GET /api/promotions' } }));
    check(res, { 'promotions 200': (r) => r.status === 200 });
  } else if (Math.random() < 0.02) {
    const key = `load-${__VU}-${__ITER}-${Math.random().toString(36).slice(2)}`;
    const res = calculate([{ productId: 'red', quantity: 1 }], key);
    check(res, { 'red 200 or 409': (r) => r.status === 200 || r.status === 409 });
  } else {
    const items = [];
    const count = 1 + Math.floor(Math.random() * 3);
    for (let i = 0; i < count; i++) {
      items.push({ productId: pick(GREEN_BLUE_ORANGE_PINK), quantity: 1 + Math.floor(Math.random() * 4) });
    }
    const res = calculate(items);
    check(res, { 'calculate 200': (r) => r.status === 200 });
  }
}
