import { exerciseApi, resetLimits } from './lib/checks.js';

// Manual only (30 minutes): watch memory, goroutines and connection pools for leaks.
export const options = {
  scenarios: {
    soak: {
      executor: 'constant-arrival-rate',
      rate: 30,
      timeUnit: '1s',
      duration: '30m',
      preAllocatedVUs: 50,
      maxVUs: 200,
    },
  },
  thresholds: {
    http_req_duration: ['p(95)<300'],
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
