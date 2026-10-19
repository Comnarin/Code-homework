import http from 'k6/http';
import { Counter } from 'k6/metrics';

// Inside compose the k6 service sets BASE_URL to http://api:8080.
export const BASE_URL = __ENV.BASE_URL || 'http://localhost:28080';

// 5xx responses. Every scenario fails when this is not zero.
export const serverErrors = new Counter('server_errors');

// 200/204 are success; 409 (Red already ordered) is an expected business answer.
// Anything else counts as a failed request in http_req_failed.
http.setResponseCallback(http.expectedStatuses(200, 204, 409));
