import http from 'k6/http';
import exec from 'k6/execution';
import { check } from 'k6';
import { Counter } from 'k6/metrics';

const users = JSON.parse(open(__ENV.USERS_FILE));
const baseURL = __ENV.BASE_URL || 'http://127.0.0.1:8000';
const eventID = __ENV.EVENT_ID;
const expectedAccepted = Number(__ENV.EXPECTED_ACCEPTED || '100');
const virtualUsers = Number(__ENV.VUS || String(users.length));

const accepted = new Counter('claim_accepted');
const soldOut = new Counter('claim_sold_out');
const unexpected = new Counter('claim_unexpected');

export const options = {
  scenarios: {
    flash_claim: {
      executor: 'shared-iterations',
      vus: virtualUsers,
      iterations: users.length,
      maxDuration: '2m',
    },
  },
  thresholds: {
    http_req_failed: ['rate<0.001'],
    http_req_duration: ['p(95)<5000', 'p(99)<10000'],
    claim_unexpected: ['count==0'],
    claim_accepted: [`count==${expectedAccepted}`],
  },
};

function uuid() {
  return 'xxxxxxxx-xxxx-4xxx-yxxx-xxxxxxxxxxxx'.replace(/[xy]/g, (value) => {
    const random = Math.floor(Math.random() * 16);
    const nibble = value === 'x' ? random : (random & 0x3) | 0x8;
    return nibble.toString(16);
  });
}

export default function () {
  const index = exec.scenario.iterationInTest;
  const user = users[index];
  const response = http.post(`${baseURL}/api/v1/ai-events/${eventID}/claims`, '{}', {
    headers: {
      Authorization: `Token ${user.Token}`,
      'Idempotency-Key': uuid(),
      'Content-Type': 'application/json',
    },
    tags: { operation: 'ai_event_claim' },
    responseCallback: http.expectedStatuses(200, 409),
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
  } else {
    unexpected.add(1, { status: String(response.status), code });
  }
  check(response, {
    'claim result is expected': () =>
      response.status === 200 || (response.status === 409 && code === 'AI_EVENT_SOLD_OUT'),
  });
}
