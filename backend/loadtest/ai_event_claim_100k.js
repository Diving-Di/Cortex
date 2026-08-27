import http from 'k6/http';
import exec from 'k6/execution';
import { check } from 'k6';
import { Counter } from 'k6/metrics';

const baseURL = __ENV.BASE_URL || 'http://127.0.0.1:8000';
const eventID = __ENV.EVENT_ID;
const runID = __ENV.RUN_ID;
const secret = __ENV.RUN_SECRET;
const total = Number(__ENV.TOTAL_REQUESTS || '100000');
const eligible = Number(__ENV.ELIGIBLE_USERS || '90000');
const slots = Number(__ENV.EVENT_SLOTS || '10000');
const virtualUsers = Number(__ENV.VUS || '2000');

const accepted = new Counter('claim_accepted');
const soldOut = new Counter('claim_sold_out');
const ineligible = new Counter('claim_ineligible');
const unexpected = new Counter('claim_unexpected');
const busy = new Counter('claim_busy');
const unavailable = new Counter('claim_unavailable');
const serverError = new Counter('claim_server_error');

export const options = {
  summaryTrendStats: ['avg', 'min', 'med', 'p(90)', 'p(95)', 'p(99)', 'max'],
  scenarios: {
    mixed_claims: {
      executor: 'shared-iterations',
      vus: virtualUsers,
      iterations: total,
      maxDuration: '20m',
      gracefulStop: '30s',
    },
  },
  thresholds: {
    http_req_failed: ['rate<0.001'],
    http_req_duration: ['p(95)<10000', 'p(99)<20000'],
    claim_unexpected: ['count==0'],
    claim_accepted: [`count==${slots}`],
    claim_ineligible: [`count==${total - eligible}`],
    claim_sold_out: [`count==${eligible - slots}`],
  },
};

function uuid(index) {
  const tail = index.toString(16).padStart(12, '0').slice(-12);
  return `10000000-0000-4000-8000-${tail}`;
}

export default function () {
  const index = exec.scenario.iterationInTest;
  const token = `loadtest-token:${runID}:${index}:${secret}`;
  const response = http.post(`${baseURL}/api/v1/ai-events/${eventID}/claims`, '{}', {
    headers: {
      Authorization: `Token ${token}`,
      'Idempotency-Key': uuid(index),
      'Content-Type': 'application/json',
    },
    tags: { operation: 'ai_event_claim_100k' },
    responseCallback: http.expectedStatuses(200, 409),
    timeout: '30s',
  });
  let code = '';
  try {
    code = response.json('code') || '';
  } catch (_) {
    code = '';
  }
  if (response.status === 200) {
    accepted.add(1);
  } else if (response.status === 409 && code === 'AI_EVENT_SOLD_OUT') {
    soldOut.add(1);
  } else if (response.status === 409 && code === 'AI_EVENT_INELIGIBLE') {
    ineligible.add(1);
  } else {
    if (code === 'AI_EVENT_BUSY') busy.add(1);
    if (code === 'AI_EVENT_UNAVAILABLE') unavailable.add(1);
    if (response.status >= 500) serverError.add(1, { status: String(response.status), code });
    unexpected.add(1, { status: String(response.status), code });
  }
  check(response, {
    'mixed claim result is expected': () =>
      response.status === 200 ||
      (response.status === 409 && (code === 'AI_EVENT_SOLD_OUT' || code === 'AI_EVENT_INELIGIBLE')),
  });
}
