import http from "k6/http";
import { check, sleep } from "k6";
import { Rate } from "k6/metrics";

const ISSUER = __ENV.KEYCLOAK_ISSUER;
const CLIENT_ID = __ENV.KEYCLOAK_CLIENT_ID;
const CLIENT_SECRET = __ENV.KEYCLOAK_CLIENT_SECRET;
const BASE_URL = __ENV.BASE_URL || "http://rmi.rmi.svc.cluster.local";
const TEST_CPF = __ENV.TEST_CPF || "12345678901";
const TEST_SCENARIO = __ENV.TEST_SCENARIO || "mixed";
const DURATION = __ENV.DURATION || "30s";
const VIRTUAL_USERS = parseInt(__ENV.VIRTUAL_USERS) || 10;

let globalAccessToken = null;
let tokenExpiryTime = 0;

const failureRate = new Rate("failed_requests");

export const options = {
    scenarios: {},
    thresholds: {
        http_req_duration: ["p(95)<5000"],
        http_req_failed: ["rate<0.05"],
        failed_requests: ["rate<0.05"],
    },
};

if (TEST_SCENARIO === "mixed") {
    options.scenarios = {
        first_login_user: {
            executor: "constant-vus",
            vus: Math.max(1, Math.floor(VIRTUAL_USERS * 0.1)),
            duration: DURATION,
            tags: { scenario: "first_login" },
        },
        home_access: {
            executor: "constant-vus",
            vus: Math.max(1, Math.floor(VIRTUAL_USERS * 0.4)),
            duration: DURATION,
            tags: { scenario: "home_access" },
        },
        personal_info_management: {
            executor: "constant-vus",
            vus: Math.max(1, Math.floor(VIRTUAL_USERS * 0.2)),
            duration: DURATION,
            tags: { scenario: "personal_info" },
        },
        wallet_interactions: {
            executor: "constant-vus",
            vus: Math.max(1, Math.floor(VIRTUAL_USERS * 0.3)),
            duration: DURATION,
            tags: { scenario: "wallet" },
        },
    };
} else if (TEST_SCENARIO === "legacy") {
    options.vus = VIRTUAL_USERS;
    options.duration = DURATION;
} else {
    options.scenarios[TEST_SCENARIO] = {
        executor: "constant-vus",
        vus: VIRTUAL_USERS,
        duration: DURATION,
        tags: { scenario: TEST_SCENARIO },
    };
}

function getAccessToken() {
    if (globalAccessToken && Date.now() < tokenExpiryTime) {
        return globalAccessToken;
    }

    const tokenUrl = `${ISSUER}/protocol/openid-connect/token`;
    const tokenData = `client_id=${CLIENT_ID}&client_secret=${CLIENT_SECRET}&grant_type=client_credentials&scope=openid`;

    const tokenResponse = http.post(tokenUrl, tokenData, {
        headers: { "Content-Type": "application/x-www-form-urlencoded" },
        timeout: "30s"
    });

    if (tokenResponse.status === 200) {
        const tokens = JSON.parse(tokenResponse.body);
        globalAccessToken = tokens.access_token;
        tokenExpiryTime = Date.now() + (tokens.expires_in * 1000) - 60000;
        return tokens.access_token;
    }

    console.error(`❌ Error obtaining token: ${tokenResponse.status}`);
    return null;
}

function makeApiCall(method, endpoint, payload = null, expectedStatus = 200) {
    const token = getAccessToken();
    if (!token) {
        failureRate.add(1);
        return null;
    }

    const url = endpoint.startsWith('http') ? endpoint : `${BASE_URL}${endpoint}`;
    const headers = {
        "Authorization": `Bearer ${token}`,
        "Content-Type": "application/json",
    };

    let response;
    if (method === "GET") {
        response = http.get(url, { headers, timeout: "30s" });
    } else if (method === "POST") {
        response = http.post(url, JSON.stringify(payload), { headers, timeout: "30s" });
    } else if (method === "PUT") {
        response = http.put(url, JSON.stringify(payload), { headers, timeout: "30s" });
    }

    const success = check(response, {
        [`${method} ${endpoint} status is ${expectedStatus}`]: (r) => r.status === expectedStatus,
        [`${method} ${endpoint} response time < 5000ms`]: (r) => r.timings.duration < 5000,
    });

    if (!success) {
        failureRate.add(1);
        console.error(`❌ ${method} ${endpoint} failed: ${response.status}`);
    }

    return response;
}

function testFirstLoginUser() {
    const cpf = TEST_CPF;

    const firstLoginCheck = makeApiCall("GET", `/citizen/${cpf}/firstlogin`);
    if (!firstLoginCheck) return;

    sleep(10);

    const updateFirstLogin = makeApiCall("PUT", `/citizen/${cpf}/firstlogin`);
    if (!updateFirstLogin) return;

    sleep(3);

    const responses = http.batch([
        {
            method: "GET",
            url: `${BASE_URL}/citizen/${cpf}/maintenance-request`,
            params: {
                headers: {
                    "Authorization": `Bearer ${getAccessToken()}`,
                    "Content-Type": "application/json",
                }
            }
        },
        {
            method: "GET",
            url: `${BASE_URL}/citizen/${cpf}/wallet`,
            params: {
                headers: {
                    "Authorization": `Bearer ${getAccessToken()}`,
                    "Content-Type": "application/json",
                }
            }
        }
    ]);

    check(responses[0], {
        "maintenance-request status is 200": (r) => r.status === 200,
    });

    check(responses[1], {
        "wallet status is 200": (r) => r.status === 200,
    });
}

function testHomeAccess() {
    const cpf = TEST_CPF;

    const responses = http.batch([
        {
            method: "GET",
            url: `${BASE_URL}/citizen/${cpf}/maintenance-request`,
            params: {
                headers: {
                    "Authorization": `Bearer ${getAccessToken()}`,
                    "Content-Type": "application/json",
                }
            }
        },
        {
            method: "GET",
            url: `${BASE_URL}/citizen/${cpf}/wallet`,
            params: {
                headers: {
                    "Authorization": `Bearer ${getAccessToken()}`,
                    "Content-Type": "application/json",
                }
            }
        }
    ]);

    check(responses[0], {
        "home maintenance-request status is 200": (r) => r.status === 200,
        "home maintenance-request response time < 3000ms": (r) => r.timings.duration < 3000,
    });

    check(responses[1], {
        "home wallet status is 200": (r) => r.status === 200,
        "home wallet response time < 3000ms": (r) => r.timings.duration < 3000,
    });
}

function testPersonalInfoManagement() {
    const cpf = TEST_CPF;

    sleep(2);

    const personalInfo = makeApiCall("GET", `/citizen/${cpf}`);
    if (!personalInfo) return;

    sleep(2);

    const updateEthnicity = makeApiCall("PUT", `/citizen/${cpf}/ethnicity`, {
        valor: "Pardo"
    });
    if (!updateEthnicity) return;

    sleep(10);

    const updatePhone = makeApiCall("POST", `/citizen/${cpf}/phone`, {
        code: "123456",
        ddd: "21",
        ddi: "55",
        valor: "987654321"
    });
    if (!updatePhone) return;

    sleep(15);

    const validatePhone = makeApiCall("PUT", `/citizen/${cpf}/phone/validate`, {
        ddd: "21",
        ddi: "55",
        valor: "987654321"
    });
    if (!validatePhone) return;

    sleep(10);

    const updateEmail = makeApiCall("PUT", `/citizen/${cpf}/email`, {
        valor: "test@example.com"
    });
    if (!updateEmail) return;

    sleep(10);

    const addressInfo = makeApiCall("GET", `/citizen/${cpf}`);
    if (!addressInfo) return;

    sleep(10);

    const updateAddress = makeApiCall("PUT", `/citizen/${cpf}/address`, {
        bairro: "Centro",
        cep: "20040-020",
        complemento: "Apto 101",
        estado: "RJ",
        logradouro: "Rua da Carioca",
        municipio: "Rio de Janeiro",
        numero: "123",
        tipo_logradouro: "Rua"
    });
    if (!updateAddress) return;

    sleep(10);

    const optinInfo = makeApiCall("GET", `/citizen/${cpf}/optin`);
    if (!optinInfo) return;

    sleep(2);

    const updateOptin = makeApiCall("PUT", `/citizen/${cpf}/optin`, {
        optin: true
    });
}

function testWalletInteractions() {
    const cpf = TEST_CPF;

    const responses = http.batch([
        {
            method: "GET",
            url: `${BASE_URL}/citizen/${cpf}/wallet`,
            params: {
                headers: {
                    "Authorization": `Bearer ${getAccessToken()}`,
                    "Content-Type": "application/json",
                }
            }
        },
        {
            method: "GET",
            url: `${BASE_URL}/citizen/${cpf}/maintenance-request`,
            params: {
                headers: {
                    "Authorization": `Bearer ${getAccessToken()}`,
                    "Content-Type": "application/json",
                }
            }
        }
    ]);

    check(responses[0], {
        "wallet info status is 200": (r) => r.status === 200,
    });

    check(responses[1], {
        "maintenance requests status is 200": (r) => r.status === 200,
    });

    sleep(2);

    const walletCards = [
        "Clínica da Família",
        "Educação",
        "Cadúnico",
        "Chamados"
    ];

    walletCards.forEach((cardName, index) => {
        if (cardName === "Chamados") {
            makeApiCall("GET", `/citizen/${cpf}/maintenance-request`);
        } else {
            makeApiCall("GET", `/citizen/${cpf}/wallet`);
        }

        if (index < walletCards.length - 1) {
            sleep(5);
        }
    });
}

function testLegacyLoad() {
    const token = getAccessToken();

    if (!token) {
        console.error("❌ No valid token available");
        return;
    }

    const response = http.get(BASE_URL, {
        headers: {
            "Authorization": `Bearer ${token}`,
            "Content-Type": "application/json",
        },
        timeout: "30s"
    });

    check(response, {
        "status is 200": (r) => r.status === 200,
        "status is not 401": (r) => r.status !== 401,
        "response time < 5000ms": (r) => r.timings.duration < 5000,
    });

    if (response.status !== 200) {
        console.error(`❌ Request failed: ${response.status}`);
    }
}

export default function() {
    const scenario = __ENV.K6_SCENARIO || TEST_SCENARIO;

    switch (scenario) {
        case "first_login":
            testFirstLoginUser();
            break;
        case "home_access":
            testHomeAccess();
            break;
        case "personal_info":
            testPersonalInfoManagement();
            break;
        case "wallet":
            testWalletInteractions();
            break;
        case "legacy":
            testLegacyLoad();
            break;
        case "mixed":
            const scenarios = ["first_login", "home_access", "personal_info", "wallet"];
            const randomScenario = scenarios[Math.floor(Math.random() * scenarios.length)];
            switch (randomScenario) {
                case "first_login":
                    testFirstLoginUser();
                    break;
                case "home_access":
                    testHomeAccess();
                    break;
                case "personal_info":
                    testPersonalInfoManagement();
                    break;
                case "wallet":
                    testWalletInteractions();
                    break;
            }
            break;
        default:
            testHomeAccess();
    }

    sleep(1);
}

export function setup() {
    console.log("🚀 Setting up load tests...");
    console.log(`📝 Test scenario: ${TEST_SCENARIO}`);
    console.log(`👥 Virtual users: ${VIRTUAL_USERS}`);
    console.log(`⏱️ Duration: ${DURATION}`);
    console.log(`🎯 Base URL: ${BASE_URL}`);
    console.log(`🆔 Test CPF: ${TEST_CPF}`);

    const token = getAccessToken();
    if (!token) {
        throw new Error("Failed to obtain authentication token in setup");
    }

    console.log("✅ Authentication token obtained successfully");

    return { token };
}

export function teardown() {
    console.log("🧹 Cleaning up load tests...");
    globalAccessToken = null;
    tokenExpiryTime = 0;
    console.log("✅ Test cleanup completed");
}
