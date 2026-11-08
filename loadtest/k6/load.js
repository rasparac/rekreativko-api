// k6 load test against the gateway (task loadtest:run, loadtest/README.md).
//
//   MODE=prepare  every load-test user creates a public session near Zagreb and
//                 RSVPs to a few others, so the reads below return real rows.
//                 Run once after task loadtest:seed.
//   MODE=load     (default) a constant RATE requests per second for DURATION,
//                 picked from MIX - roughly what the app sends: mostly reads
//                 of sessions, the feed and profiles, some RSVP writes (which
//                 also put events through the outbox, NATS and notifications)
//                 and the occasional login.
import http from 'k6/http';
import { check, fail } from 'k6';

const BASE = __ENV.BASE_URL || 'http://gateway:8080';
const MODE = __ENV.MODE || 'load';
const RATE = parseInt(__ENV.RATE || '50', 10);
const DURATION = __ENV.DURATION || '60s';
const USERS = parseInt(__ENV.USERS || '40', 10);
const PASSWORD = 'Loadtest1234!';
const ZAGREB = { lat: 45.815, lng: 15.982 };

export const options = MODE === 'prepare'
  ? { vus: 1, iterations: 1 }
  : {
      scenarios: {
        load: {
          executor: 'constant-arrival-rate',
          rate: RATE,
          timeUnit: '1s',
          duration: DURATION,
          preAllocatedVUs: Math.max(10, RATE),
          maxVUs: Math.max(40, RATE * 4),
        },
      },
      thresholds: {
        // a run where requests fail or queue measures the failure, not the load
        http_req_failed: ['rate<0.01'],
        dropped_iterations: ['count<' + Math.max(1, RATE)],
      },
      summaryTrendStats: ['avg', 'p(50)', 'p(95)', 'p(99)', 'max'],
    };

function email(i) {
  return `loadtest-${String(i).padStart(2, '0')}@example.com`;
}

function json(token) {
  const headers = { 'Content-Type': 'application/json' };
  if (token) headers.Authorization = `Bearer ${token}`;
  return { headers };
}

function login(i) {
  const res = http.post(`${BASE}/identity/api/v1/login`,
    JSON.stringify({ email: email(i), password: PASSWORD }), json(),
  );
  if (res.status !== 200) fail(`login ${email(i)}: ${res.status} ${res.body} - run task loadtest:seed`);
  return res.json('data.access_token');
}

// The list may be the data itself or wrapped in a page object.
function list(res) {
  const data = res.json('data');
  if (Array.isArray(data)) return data;
  for (const v of Object.values(data || {})) {
    if (Array.isArray(v)) return v;
  }
  return [];
}

function discoverSessions(token) {
  const res = http.get(
    `${BASE}/activity/api/v1/sessions/discover?lat=${ZAGREB.lat}&lng=${ZAGREB.lng}&radius_km=50`,
    Object.assign(json(token), { tags: { name: 'sessions/discover' } }),
  );
  return list(res).map((s) => s.id);
}

function prepare() {
  const tokens = [];
  for (let i = 1; i <= USERS; i++) tokens.push(login(i));

  const start = new Date(Date.now() + 7 * 24 * 3600 * 1000).toISOString();
  tokens.forEach((token, i) => {
    const res = http.post(`${BASE}/activity/api/v1/sessions`, JSON.stringify({
      title: `Load test game ${i + 1}`,
      activity_type: 'football',
      difficulty_level: 'intermediate',
      location_city: 'Zagreb',
      location_country: 'Croatia',
      location_lat: ZAGREB.lat + (i % 10) * 0.01,
      location_lng: ZAGREB.lng + Math.floor(i / 10) * 0.01,
      start_time: start,
      capacity: 30,
      visibility: 'public',
    }), json(token));
    check(res, { 'session created': (r) => r.status === 201 });
  });

  const sessions = discoverSessions(tokens[0]);
  tokens.forEach((token, i) => {
    for (let k = 1; k <= 5; k++) {
      const id = sessions[(i + k * 7) % sessions.length];
      http.post(`${BASE}/activity/api/v1/sessions/${id}/rsvp`, JSON.stringify({ status: 'going' }), json(token));
    }
  });
  console.log(`prepared: ${sessions.length} sessions near Zagreb`);
}

export function setup() {
  if (MODE === 'prepare') return {};

  const tokens = [];
  for (let i = 1; i <= USERS; i++) tokens.push(login(i));
  const sessions = discoverSessions(tokens[0]);
  if (sessions.length === 0) fail('no sessions near Zagreb - run task loadtest:prepare');
  // a reverse geocode that misses the cache waits for Nominatim (1 req/s); warm the one the mix asks for
  http.get(`${BASE}/location/api/v1/geocode/reverse?lat=${ZAGREB.lat}&lon=${ZAGREB.lng}`, json(tokens[0]));
  return { tokens, sessions };
}

function pick(xs) {
  return xs[Math.floor(Math.random() * xs.length)];
}

// [weight, name, request]; the weights add up to 100.
const MIX = [
  [20, 'sessions/discover', (t) => http.get(`${BASE}/activity/api/v1/sessions/discover?lat=${ZAGREB.lat}&lng=${ZAGREB.lng}&radius_km=50`, t)],
  [12, 'sessions/{id}', (t, d) => http.get(`${BASE}/activity/api/v1/sessions/${pick(d.sessions)}`, t)],
  [10, 'sessions/{id}/attendees', (t, d) => http.get(`${BASE}/activity/api/v1/sessions/${pick(d.sessions)}/attendees`, t)],
  [8, 'sessions/{id}/team-formation', (t, d) => http.get(`${BASE}/activity/api/v1/sessions/${pick(d.sessions)}/team-formation`, t)],
  [8, 'sessions (mine)', (t) => http.get(`${BASE}/activity/api/v1/sessions`, t)],
  [5, 'activity-groups/discover', (t) => http.get(`${BASE}/activity/api/v1/activity-groups/discover?lat=${ZAGREB.lat}&lng=${ZAGREB.lng}&radius_km=50`, t)],
  [12, 'notifications', (t) => http.get(`${BASE}/notifications/api/v1/notifications`, t)],
  [8, 'my/profile', (t) => http.get(`${BASE}/account-profile/api/v1/my/profile`, t)],
  [3, 'my/settings', (t) => http.get(`${BASE}/account-profile/api/v1/my/settings`, t)],
  [5, 'me', (t) => http.get(`${BASE}/identity/api/v1/me`, t)],
  [2, 'geocode/reverse', (t) => http.get(`${BASE}/location/api/v1/geocode/reverse?lat=${ZAGREB.lat}&lon=${ZAGREB.lng}`, t)],
  [6, 'rsvp', (t, d) => {
    // RSVP, or flip an existing RSVP between going and maybe - reading it
    // first, as the app does, because the wrong one of POST and PUT answers
    // 500 rather than 409/404 (rekreativko-api-clp)
    const id = pick(d.sessions);
    const url = `${BASE}/activity/api/v1/sessions/${id}/rsvp`;
    const status = Math.random() < 0.5 ? 'going' : 'maybe';
    const mine = http.get(url, Object.assign({}, t, { responseCallback: http.expectedStatuses(200, 404) }));
    if (mine.status === 404) {
      return http.post(url, JSON.stringify({ status }), t);
    }
    return http.put(url, JSON.stringify({ status }), t);
  }],
  [1, 'login', (_t, _d, i) => http.post(`${BASE}/identity/api/v1/login`,
    JSON.stringify({ email: email(i), password: PASSWORD }), { headers: { 'Content-Type': 'application/json' } })],
];

export default function (data) {
  if (MODE === 'prepare') {
    prepare();
    return;
  }

  const i = Math.floor(Math.random() * data.tokens.length);
  const params = json(data.tokens[i]);
  let roll = Math.random() * 100;
  for (const [weight, name, request] of MIX) {
    roll -= weight;
    if (roll < 0) {
      params.tags = { name };
      request(params, data, i + 1);
      return;
    }
  }
}
