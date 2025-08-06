import http from 'k6/http';
import { check, sleep } from 'k6';
import { Rate, Counter } from 'k6/metrics';
import papaparse from 'https://jslib.k6.io/papaparse/5.1.1/index.js';

const TOKEN_FILE = __ENV.TOKEN_FILE || 'scripts/tokens.json';
const OAUTH_CONFIG = __ENV.OAUTH_CONFIG || 'scripts/idcarioca_staging.json';
const tokenData = JSON.parse(open(TOKEN_FILE));
const oauthConfig = JSON.parse(open(OAUTH_CONFIG)).oauth2;

let accessToken = tokenData.access_token;
let refreshToken = tokenData.refresh_token;
let expiresAt = Date.now();

function refreshAccessTokenIfNeeded() {
  if (Date.now() > expiresAt - 60000) {
    const tokenUrl = `${oauthConfig.issuer}/protocol/openid-connect/token`;
    const payload = {
      client_id: oauthConfig.client_id,
      client_secret: oauthConfig.client_secret,
      grant_type: 'refresh_token',
      refresh_token: refreshToken,
    };
    const params = { headers: { 'Content-Type': 'application/x-www-form-urlencoded' } };
    const res = http.post(tokenUrl, Object.entries(payload).map(([k, v]) => `${k}=${encodeURIComponent(v)}`).join('&'), params);
    if (res.status === 200) {
      const tokens = res.json();
      accessToken = tokens.access_token;
      refreshToken = tokens.refresh_token;
      expiresAt = Date.now() + (tokens.expires_in * 1000);
    } else {
      throw new Error(`Failed to refresh token: ${res.status} ${res.body}`);
    }
  }
}

const failedRequests = new Rate('failed_requests');

export const options = {
  stages: [
    { duration: '5m', target: 5000 },
    { duration: '15m', target: 5000 },
    { duration: '5m', target: 0 },
  ],
  thresholds: {
    'failed_requests': ['rate<0.05'],
    'http_req_duration': ['p(95)<5000'],
  },
};

const BASE_URL = __ENV.BASE_URL || 'http://localhost:8080/v1';
const CPF_CSV = __ENV.CPF_CSV;

let cpfList = ['12345678901'];
if (CPF_CSV) {
  try {
    const csvContent = open(CPF_CSV);
    const parsed = papaparse.parse(csvContent, { header: false });
    cpfList = parsed.data
      .map(row => Array.isArray(row) ? row[0] : row)
      .filter(cpf => cpf.length === 11)
    if (cpfList.length === 0) {
      cpfList = ['12345678901'];
    }
  } catch (e) {
    console.error('Failed to load CPF CSV:', e);
    cpfList = ['12345678901'];
  }
}

function randomInt(min, max) {
  return Math.floor(Math.random() * (max - min + 1)) + min;
}
function randomString(length) {
  const chars = 'abcdefghijklmnopqrstuvwxyz';
  let result = '';
  for (let i = 0; i < length; i++) {
    result += chars.charAt(Math.floor(Math.random() * chars.length));
  }
  return result;
}
function randomEmail() {
  return `${randomString(8)}${randomInt(100,999)}@example.com`;
}
function randomPhone() {
  return {
    ddi: '55',
    ddd: randomInt(11, 99).toString(),
    valor: `${randomInt(900000000, 999999999)}`
  };
}
function randomAddress() {
  return {
    logradouro: `Rua ${randomString(6)}`,
    numero: `${randomInt(1, 9999)}`,
    complemento: `Apto ${randomInt(1, 50)}`,
    bairro: `Bairro ${randomString(5)}`,
    cidade: 'Rio de Janeiro',
    estado: 'RJ',
    cep: `${randomInt(20000000, 29999999)}`,
    municipio: 'Rio de Janeiro'
  };
}

export default function() {
  refreshAccessTokenIfNeeded();
  const headers = {
    'Content-Type': 'application/json',
    'Authorization': `Bearer ${accessToken}`,
  };
  const TEST_CPF = cpfList[(__VU - 1) % cpfList.length];

  const healthCheck = http.get(`${BASE_URL}/health`, { headers });
  failedRequests.add(healthCheck.status >= 400);
  check(healthCheck, {
    'health check status is 200': (r) => r.status === 200,
    'health check has services': (r) => r.json('services') !== undefined,
  });

  const getCitizen = http.get(`${BASE_URL}/citizen/${TEST_CPF}`, { headers });
  failedRequests.add(getCitizen.status >= 400);
  check(getCitizen, {
    'get citizen status is 200': (r) => r.status === 200,
    'citizen has cpf': (r) => r.json('cpf') !== undefined,
  });

  const addressData = randomAddress();
  const updateAddress = http.put(
    `${BASE_URL}/citizen/${TEST_CPF}/address`,
    JSON.stringify(addressData),
    { headers }
  );
  failedRequests.add(updateAddress.status >= 400);
  check(updateAddress, {
    'update address status is 200 or 409': (r) => r.status === 200 || r.status === 409,
  });

  const phoneData = randomPhone();
  const updatePhone = http.put(
    `${BASE_URL}/citizen/${TEST_CPF}/phone`,
    JSON.stringify(phoneData),
    { headers }
  );
  failedRequests.add(updatePhone.status >= 400);
  check(updatePhone, {
    'update phone status is 200 or 409': (r) => r.status === 200 || r.status === 409,
  });

  const emailData = { valor: randomEmail() };
  const updateEmail = http.put(
    `${BASE_URL}/citizen/${TEST_CPF}/email`,
    JSON.stringify(emailData),
    { headers }
  );
  failedRequests.add(updateEmail.status >= 400);
  check(updateEmail, {
    'update email status is 200 or 409': (r) => r.status === 200 || r.status === 409,
  });

  const getFirstLogin = http.get(`${BASE_URL}/citizen/${TEST_CPF}/firstlogin`, { headers });
  failedRequests.add(getFirstLogin.status >= 400);
  check(getFirstLogin, {
    'get first login status is 200': (r) => r.status === 200,
  });

  const getOptIn = http.get(`${BASE_URL}/citizen/${TEST_CPF}/optin`, { headers });
  failedRequests.add(getOptIn.status >= 400);
  check(getOptIn, {
    'get opt-in status is 200': (r) => r.status === 200,
  });

  console.log(`Statuses: health=${healthCheck.status}, citizen=${getCitizen.status}, address=${updateAddress.status}, phone=${updatePhone.status}, email=${updateEmail.status}, firstlogin=${getFirstLogin.status}, optin=${getOptIn.status}`);

  sleep(1);
}