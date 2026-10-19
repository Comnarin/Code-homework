import { sleep } from 'k6';
import { exerciseApi, resetLimits } from './lib/checks.js';

export const options = {
  scenarios: {
    smoke: { executor: 'constant-vus', vus: 2, duration: '30s' },
  },
  thresholds: {
    http_req_failed: ['rate<0.01'],
    http_req_duration: ['p(95)<300'],
    server_errors: ['count<1'],
  },
};

export function setup() {
  resetLimits();
}

export default function () {
  exerciseApi();
  sleep(0.1);
}
