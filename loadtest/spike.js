import { exerciseApi, resetLimits } from './lib/checks.js';

// Sudden burst: 10 -> 300 requests per second in 10 seconds, hold a minute, drop back.
export const options = {
  scenarios: {
    spike: {
      executor: 'ramping-arrival-rate',
      startRate: 10,
      timeUnit: '1s',
      preAllocatedVUs: 100,
      maxVUs: 1000,
      stages: [
        { target: 300, duration: '10s' },
        { target: 300, duration: '1m' },
        { target: 10, duration: '10s' },
      ],
    },
  },
  thresholds: {
    http_req_failed: ['rate<0.05'],
    server_errors: ['count<1'],
  },
};

export function setup() {
  resetLimits();
}

export default function () {
  exerciseApi();
}
