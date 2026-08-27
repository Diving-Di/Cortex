import http from 'k6/http';
import exec from 'k6/execution';
import { check, sleep } from 'k6';
import { Counter, Trend } from 'k6/metrics';

const baseURL = __ENV.BASE_URL || 'http://127.0.0.1:8000';
const eventID = __ENV.EVENT_ID;
const runID = __ENV.RUN_ID;
const secret = __ENV.RUN_SECRET;
const users = Number(__ENV.JOURNEY_USERS || '2000');
const virtualUsers = Number(__ENV.JOURNEY_VUS || '200');
const eligibleUsers = Number(__ENV.ELIGIBLE_USERS || '1800');
const slots = Number(__ENV.EVENT_SLOTS || '500');

const statusDuration = new Trend('activity_status_duration', true);
const eligibilityDuration = new Trend('eligibility_duration', true);
const claimDuration = new Trend('claim_duration', true);
const resultDuration = new Trend('claim_result_duration', true);
const claimSuccess = new Counter('claim_success');
const claimSoldOut = new Counter('claim_sold_out');
const claimIneligible = new Counter('claim_ineligible');
const claimUnexpected = new Counter('claim_unexpected');
const resultExpected = new Counter('claim_result_expected');

export const options = {
  setupTimeout: '10m',
  summaryTrendStats: ['avg', 'min', 'med', 'p(90)', 'p(95)', 'p(99)', 'max'],
  scenarios: {
    realistic_user_journey: {
      executor: 'shared-iterations',
      vus: virtualUsers,
      iterations: users,
      maxDuration: '10m',
      gracefulStop: '30s',
    },
  },
  thresholds: {
    http_req_failed: ['rate<0.001'],
    http_req_duration: ['p(99)<8000'],
    activity_status_duration: ['p(95)<500'],
    eligibility_duration: ['p(95)<1000'],
    claim_duration: ['p(95)<3000'],
    claim_result_duration: ['p(95)<1000'],
    claim_success: [`count==${slots}`],
    claim_ineligible: [`count==${users - eligibleUsers}`],
    claim_sold_out: [`count==${eligibleUsers - slots}`],
    claim_unexpected: ['count==0'],
  },
};

function token(index) {
  return `loadtest-token:${runID}:${index}:${secret}`;
}

function headers(index) {
  return { Authorization: `Token ${token(index)}`, 'Content-Type': 'application/json' };
}

function uuid(index) {
  return `20000000-0000-4000-8000-${index.toString(16).padStart(12, '0').slice(-12)}`;
}

function get(url, index, operation, expectedStatuses) {
  return http.get(url, {
    headers: headers(index),
    tags: { operation, temperature: index < 1400 ? 'hot' : index < 1800 ? 'warm' : 'cold' },
    responseCallback: http.expectedStatuses(...expectedStatuses),
    timeout: '15s',
  });
}

export function setup() {
  // Hot tokens are reused before the measured phase; warm tokens appear once.
  for (let index = 0; index < 1800; index += 1) {
    get(`${baseURL}/api/v1/ai-events/${eventID}`, index, 'token_prewarm', [200]);
    if (index < 1400) get(`${baseURL}/api/v1/ai-points/balance`, index, 'token_hot_reuse', [200]);
  }
}

export default function () {
  const index = exec.scenario.iterationInTest;
  const statusRequests = index < users / 4 ? 3 : 2;
  for (let request = 0; request < statusRequests; request += 1) {
    const response = get(`${baseURL}/api/v1/ai-events/current`, index, 'activity_status', [200]);
    statusDuration.add(response.timings.duration, { temperature: index < 1400 ? 'hot' : index < 1800 ? 'warm' : 'cold' });
    check(response, { 'activity status returned': (value) => value.status === 200 });
  }

  sleep(1 + (index % 5));
  const eligibility = get(`${baseURL}/api/v1/ai-events/${eventID}`, index, 'eligibility', [200]);
  eligibilityDuration.add(eligibility.timings.duration);

  const claim = http.post(`${baseURL}/api/v1/ai-events/${eventID}/claims`, '{}', {
    headers: { ...headers(index), 'Idempotency-Key': uuid(index) },
    tags: { operation: 'claim', temperature: index < 1400 ? 'hot' : index < 1800 ? 'warm' : 'cold' },
    responseCallback: http.expectedStatuses(200, 409),
    timeout: '15s',
  });
  claimDuration.add(claim.timings.duration);
  let code = '';
  try { code = claim.json('code') || ''; } catch (_) { code = ''; }
  if (claim.status === 200) claimSuccess.add(1);
  else if (claim.status === 409 && code === 'AI_EVENT_SOLD_OUT') claimSoldOut.add(1);
  else if (claim.status === 409 && code === 'AI_EVENT_INELIGIBLE') claimIneligible.add(1);
  else claimUnexpected.add(1, { status: String(claim.status), code });

  if (index < users / 2) {
    const result = get(`${baseURL}/api/v1/ai-events/${eventID}/claims/me`, index, 'claim_result', [200, 404]);
    resultDuration.add(result.timings.duration);
    resultExpected.add(1, { status: String(result.status) });
  }
  if (index < users / 4) get(`${baseURL}/api/v1/ai-points/balance`, index, 'related_balance', [200]);
}
