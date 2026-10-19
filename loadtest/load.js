import { exerciseApi, resetLimits } from './lib/checks.js';

// Steady state: 50 requests per second for 5 minutes.
export const options = {
  scenarios: {
    load: {
      executor: 'constant-arrival-rate',
      rate: 50,
      timeUnit: '1s',
      duration: '5m',
      preAllocatedVUs: 50,
      maxVUs: 200,
    },
  },
  thresholds: {
    http_req_duration: ['p(95)<250', 'p(99)<500'],
    http_req_failed: ['rate<0.01'],
    server_errors: ['count<1'],
  },
};

export function setup() {
  resetLimits();
}

export default function () {
  exerciseApi();
}
