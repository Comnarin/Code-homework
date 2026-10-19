import { exerciseApi, resetLimits } from './lib/checks.js';

// Find the knee: ramp from 50 to 400 requests per second over 10 minutes.
// Report-only: thresholds are evaluated and printed but never abort the run.
export const options = {
  scenarios: {
    stress: {
      executor: 'ramping-arrival-rate',
      startRate: 50,
      timeUnit: '1s',
      preAllocatedVUs: 100,
      maxVUs: 1000,
      stages: [
        { target: 400, duration: '10m' },
      ],
    },
  },
  thresholds: {
    http_req_duration: [{ threshold: 'p(95)<500', abortOnFail: false }],
    http_req_failed: [{ threshold: 'rate<0.05', abortOnFail: false }],
    server_errors: [{ threshold: 'count<1', abortOnFail: false }],
  },
};

export function setup() {
  resetLimits();
}

export default function () {
  exerciseApi();
}
