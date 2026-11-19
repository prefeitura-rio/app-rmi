import http from "k6/http";
import { check, sleep } from "k6";
import { Rate } from "k6/metrics";

const BASE_URL_RMI = trimTrailingSlash(__ENV.BASE_URL_RMI || __ENV.BASE_URL || "http://rmi.rmi.svc.cluster.local:80");
const RMI_API_VERSION = "/v1";
const BASE_URL_BUSCA = trimTrailingSlash(__ENV.BASE_URL_BUSCA || "http://app-busca-search.busca.svc.cluster.local:8080");
// DURATION is the steady stage duration unless STEADY_DURATION is set.
const DURATION = __ENV.DURATION || "10m";
const RAMP_UP_DURATION = __ENV.RAMP_UP_DURATION || "5m";
const STEADY_DURATION = __ENV.STEADY_DURATION || DURATION;
const RAMP_DOWN_DURATION = __ENV.RAMP_DOWN_DURATION || "2m";
// PUT /citizen/{cpf}/phone sends a real WhatsApp verification code, so it only runs when explicitly enabled.
const ENABLE_PHONE_WRITES = __ENV.ENABLE_PHONE_WRITES === "true";
const KEYCLOAK_CLIENT_ID = __ENV.KEYCLOAK_CLIENT_ID;
const KEYCLOAK_CLIENT_SECRET = __ENV.KEYCLOAK_CLIENT_SECRET;
const KEYCLOAK_ISSUER = __ENV.KEYCLOAK_ISSUER;
const TEST_SCENARIO = __ENV.TEST_SCENARIO || "mixed";
const USER = __ENV.USER || "12345678901";
const VIRTUAL_USERS = parseInt(__ENV.VIRTUAL_USERS) || 10;

if (USER.length !== 11 || !/^\d{11}$/.test(USER)) {
    console.warn(`⚠️ Warning: CPF "${USER}" should be exactly 11 digits. Some tests may fail.`);
}

const failureRate = new Rate("failed_requests");
const httpFailureRate = new Rate("http_failures");
const checkFailureRate = new Rate("check_failures");

const MIXED_VU_SHARES = {
    first_login_onboarding: 0.1,
    home_dashboard: 0.25,
    personal_info_update: 0.2,
    wallet_cards: 0.2,
    category_browsing: 0.1,
    search_experience: 0.1,
    popular_services: 0.05,
};

const SCENARIO_EXEC = {
    first_login_onboarding: "firstLoginOnboarding",
    home_dashboard: "homeDashboard",
    personal_info_update: "personalInfoUpdate",
    wallet_cards: "walletCards",
    category_browsing: "categoryBrowsing",
    search_experience: "searchExperience",
    popular_services: "popularServices",
    legacy: "legacy",
};

function rampingScenario(name, targetVUs) {
    return {
        executor: "ramping-vus",
        exec: SCENARIO_EXEC[name],
        stages: [
            { duration: RAMP_UP_DURATION, target: targetVUs },
            { duration: STEADY_DURATION, target: targetVUs },
            { duration: RAMP_DOWN_DURATION, target: 0 },
        ],
        tags: { scenario: name },
    };
}

function buildScenarios() {
    if (TEST_SCENARIO === "mixed") {
        const scenarios = {};
        for (const [name, share] of Object.entries(MIXED_VU_SHARES)) {
            scenarios[name] = rampingScenario(name, Math.max(1, Math.floor(VIRTUAL_USERS * share)));
        }
        return scenarios;
    }

    if (!SCENARIO_EXEC[TEST_SCENARIO]) {
        throw new Error(`Unknown TEST_SCENARIO "${TEST_SCENARIO}". Use mixed or one of: ${Object.keys(SCENARIO_EXEC).join(", ")}`);
    }

    return { [TEST_SCENARIO]: rampingScenario(TEST_SCENARIO, VIRTUAL_USERS) };
}

export const options = {
    scenarios: buildScenarios(),
    thresholds: {
        http_req_duration: ["p(95)<5000"],
        http_req_failed: ["rate<0.05"],
        failed_requests: ["rate<0.05"],
        http_failures: ["rate<0.05"],
        check_failures: ["rate<0.15"],
    },
};

function trimTrailingSlash(url) {
    return url.replace(/\/+$/, "");
}

function makeApiCall(method, endpoint, payload = null, expectedStatus = 200, token = null) {
    if (!token) {
        console.error(`❌ Authentication required for ${method} ${endpoint}`);
        failureRate.add(1);
        httpFailureRate.add(1);
        return null;
    }

    const url = endpoint.startsWith('http') ? endpoint : `${BASE_URL_RMI}${RMI_API_VERSION}${endpoint}`;
    const headers = {
        "Content-Type": "application/json",
        "Authorization": `Bearer ${token}`
    };

    let response;
    if (method === "GET") {
        response = http.get(url, { headers, timeout: "30s" });
    } else if (method === "POST") {
        response = http.post(url, JSON.stringify(payload), { headers, timeout: "30s" });
    } else if (method === "PUT") {
        response = http.put(url, JSON.stringify(payload), { headers, timeout: "30s" });
    }

    // Check for actual HTTP failures (network, server errors)
    const isHttpFailure = response.status === 0 || response.status >= 500;

    if (isHttpFailure) {
        failureRate.add(1);
        httpFailureRate.add(1);

        if (response.status === 0) {
            console.error(`🌐 ${method} ${endpoint} failed: Network error or timeout`);
        } else {
            console.error(`❌ ${method} ${endpoint} failed: HTTP ${response.status}`);
        }
    }

    // Log auth and client errors separately (not counted as system failures)
    if (response.status === 401) {
        console.warn(`🔒 ${method} ${endpoint}: Authentication issue (401) - Check token validity or permissions`);
    } else if (response.status === 403) {
        console.warn(`🚫 ${method} ${endpoint}: Forbidden (403) - Check user permissions for CPF access`);
    } else if (response.status === 404) {
        console.warn(`🔍 ${method} ${endpoint}: Resource not found (404) - Check endpoint exists`);
    }

    // Run checks separately - these don't count as failures, just quality metrics
    const checksResult = check(response, {
        [`${method} ${endpoint} status is ${expectedStatus}`]: (r) => r.status === expectedStatus,
        [`${method} ${endpoint} response time < 5000ms`]: (r) => r.timings.duration < 5000,
        [`${method} ${endpoint} is not 401 Unauthorized`]: (r) => r.status !== 401,
        [`${method} ${endpoint} has valid response`]: (r) => r.status < 500 || expectedStatus >= 500,
    });

    // Track check failures separately (for debugging, not for failure thresholds)
    if (!checksResult) {
        checkFailureRate.add(1);
    }

    return response;
}

function makePublicApiCall(method, endpoint, payload = null, expectedStatus = 200) {
    const url = endpoint.startsWith('http') ? endpoint : `${BASE_URL_BUSCA}${endpoint}`;
    const headers = {
        "Content-Type": "application/json"
    };

    let response;
    if (method === "GET") {
        response = http.get(url, { headers, timeout: "30s" });
    } else if (method === "POST") {
        response = http.post(url, JSON.stringify(payload), { headers, timeout: "30s" });
    }

    // Check for actual HTTP failures (network, server errors)
    const isHttpFailure = response.status === 0 || response.status >= 500;

    if (isHttpFailure) {
        failureRate.add(1);
        httpFailureRate.add(1);

        if (response.status === 0) {
            console.error(`🌐 ${method} ${endpoint} failed: Network error or timeout`);
        } else {
            console.error(`❌ ${method} ${endpoint} failed: HTTP ${response.status}`);
        }
    }

    // Run checks separately - these don't count as failures, just quality metrics
    const checksResult = check(response, {
        [`${method} ${endpoint} status is ${expectedStatus}`]: (r) => r.status === expectedStatus,
        [`${method} ${endpoint} response time < 5000ms`]: (r) => r.timings.duration < 5000,
    });

    // Track check failures separately (for debugging, not for failure thresholds)
    if (!checksResult) {
        checkFailureRate.add(1);
    }

    return response;
}

function testFirstLoginOnboarding(data) {
    const token = data?.token;
    if (!token) {
        console.error("❌ testFirstLoginOnboarding: No authentication token available");
        return;
    }

    const cpf = USER;

    const firstLoginCheck = makeApiCall("GET", `/citizen/${cpf}/firstlogin`, null, 200, token);
    if (!firstLoginCheck) return;

    sleep(15);

    const personalInfoBatch = makeApiCall("GET", `/citizen/${cpf}`, null, 200, token);
    if (!personalInfoBatch) return;

    sleep(5);

    const updateEthnicity = makeApiCall("PUT", `/citizen/${cpf}/ethnicity`, {
        valor: "parda"
    }, 200, token);
    if (!updateEthnicity) return;

    sleep(8);

    if (ENABLE_PHONE_WRITES) {
        const phoneNumber = `9${Math.floor(Math.random() * 100000000).toString().padStart(8, '0')}`;
        const updatePhone = makeApiCall("PUT", `/citizen/${cpf}/phone`, {
            ddd: "11",
            ddi: "55",
            valor: phoneNumber
        }, 200, token);
        if (!updatePhone) return;
    }

    sleep(12);

    const updateEmail = makeApiCall("PUT", `/citizen/${cpf}/email`, {
        valor: `onboard.${Date.now()}.${Math.random().toString(36).substring(2)}@prefeitura.sp.gov.br`
    }, 200, token);
    if (!updateEmail) return;

    sleep(10);

    const updateFirstLogin = makeApiCall("PUT", `/citizen/${cpf}/firstlogin`, null, 200, token);
    if (!updateFirstLogin) return;

    sleep(3);

    const headers = {
        "Content-Type": "application/json",
        "Authorization": `Bearer ${token}`
    };

    const onboardingCompletionBatch = http.batch([
        {
            method: "GET",
            url: `${BASE_URL_RMI}${RMI_API_VERSION}/citizen/${cpf}/maintenance-request?page=1&per_page=10`,
            params: { headers: headers }
        },
        {
            method: "GET",
            url: `${BASE_URL_RMI}${RMI_API_VERSION}/citizen/${cpf}/wallet`,
            params: { headers: headers }
        }
    ]);

    check(onboardingCompletionBatch[0], {
        "onboarding maintenance-request status is 200": (r) => r.status === 200,
        "onboarding maintenance-request is not 401 Unauthorized": (r) => r.status !== 401,
    });

    check(onboardingCompletionBatch[1], {
        "onboarding wallet status is 200": (r) => r.status === 200,
        "onboarding wallet is not 401 Unauthorized": (r) => r.status !== 401,
    });

    sleep(5);
}

function testHomeDashboardAccess(data) {
    const token = data?.token;
    if (!token) {
        console.error("❌ testHomeDashboardAccess: No authentication token available");
        return;
    }

    const cpf = USER;

    const headers = {
        "Content-Type": "application/json",
        "Authorization": `Bearer ${token}`
    };

    const dashboardBatch = http.batch([
        {
            method: "GET",
            url: `${BASE_URL_RMI}${RMI_API_VERSION}/citizen/${cpf}/maintenance-request?page=1&per_page=10`,
            params: { headers: headers }
        },
        {
            method: "GET",
            url: `${BASE_URL_RMI}${RMI_API_VERSION}/citizen/${cpf}/wallet`,
            params: { headers: headers }
        },
        {
            method: "GET",
            url: `${BASE_URL_RMI}${RMI_API_VERSION}/citizen/${cpf}`,
            params: { headers: headers }
        }
    ]);

    check(dashboardBatch[0], {
        "dashboard maintenance-request status is 200": (r) => r.status === 200,
        "dashboard maintenance-request response time < 2000ms": (r) => r.timings.duration < 2000,
        "dashboard maintenance-request is not 401 Unauthorized": (r) => r.status !== 401,
    });

    check(dashboardBatch[1], {
        "dashboard wallet status is 200": (r) => r.status === 200,
        "dashboard wallet response time < 2000ms": (r) => r.timings.duration < 2000,
        "dashboard wallet is not 401 Unauthorized": (r) => r.status !== 401,
    });

    check(dashboardBatch[2], {
        "dashboard citizen info status is 200": (r) => r.status === 200,
        "dashboard citizen info is not 401 Unauthorized": (r) => r.status !== 401,
    });

    sleep(3);
}

function testPersonalInfoUpdate(data) {
    const token = data?.token;
    if (!token) {
        console.error("❌ testPersonalInfoUpdate: No authentication token available");
        return;
    }

    const cpf = USER;

    sleep(2);

    const personalInfo = makeApiCall("GET", `/citizen/${cpf}`, null, 200, token);
    if (!personalInfo) return;

    sleep(5);

    const phoneNumber = `9${Math.floor(Math.random() * 100000000).toString().padStart(8, '0')}`;
    if (ENABLE_PHONE_WRITES) {
        const updatePhone = makeApiCall("PUT", `/citizen/${cpf}/phone`, {
            ddd: "11",
            ddi: "55",
            valor: phoneNumber
        }, 200, token);
        if (!updatePhone) return;
    }

    sleep(8);

    const validatePhone = makeApiCall("POST", `/validate/phone`, {
        phone: `+55 11 ${phoneNumber}`
    }, 200, token);
    if (!validatePhone) return;

    sleep(6);

    const updateEmail = makeApiCall("PUT", `/citizen/${cpf}/email`, {
        valor: `update.${Date.now()}.${Math.random().toString(36).substring(2)}@prefeitura.sp.gov.br`
    }, 200, token);
    if (!updateEmail) return;

    sleep(12);

    const updateAddress = makeApiCall("PUT", `/citizen/${cpf}/address`, {
        bairro: "Vila Madalena",
        cep: "05435-070",
        complemento: `Bloco ${String.fromCharCode(65 + Math.floor(Math.random() * 26))}`,
        estado: "SP",
        logradouro: "Rua Harmonia",
        municipio: "São Paulo",
        numero: `${Math.floor(Math.random() * 999) + 100}`,
        tipo_logradouro: "Rua"
    }, 200, token);
    if (!updateAddress) return;

    sleep(8);

    const updateOptin = makeApiCall("PUT", `/citizen/${cpf}/optin`, {
        optin: Math.random() > 0.5
    }, 200, token);
    if (!updateOptin) return;

    sleep(3);
}

function testWalletCardInteractions(data) {
    const token = data?.token;
    if (!token) {
        console.error("❌ testWalletCardInteractions: No authentication token available");
        return;
    }

    const cpf = USER;

    const headers = {
        "Content-Type": "application/json",
        "Authorization": `Bearer ${token}`
    };

    const walletBatch = http.batch([
        {
            method: "GET",
            url: `${BASE_URL_RMI}${RMI_API_VERSION}/citizen/${cpf}/wallet`,
            params: { headers: headers }
        },
        {
            method: "GET",
            url: `${BASE_URL_RMI}${RMI_API_VERSION}/citizen/${cpf}/maintenance-request?page=1&per_page=10`,
            params: { headers: headers }
        }
    ]);

    check(walletBatch[0], {
        "wallet card info status is 200": (r) => r.status === 200,
        "wallet card info is not 401 Unauthorized": (r) => r.status !== 401,
    });

    check(walletBatch[1], {
        "wallet maintenance requests status is 200": (r) => r.status === 200,
        "wallet maintenance requests is not 401 Unauthorized": (r) => r.status !== 401,
    });

    sleep(4);

    const walletCardActions = [
        { name: "Saúde", action: () => makeApiCall("GET", `/citizen/${cpf}/wallet`, null, 200, token) },
        { name: "Educação", action: () => makeApiCall("GET", `/citizen/${cpf}/wallet`, null, 200, token) },
        { name: "Assistência Social", action: () => makeApiCall("GET", `/citizen/${cpf}/wallet`, null, 200, token) },
        { name: "Chamados", action: () => makeApiCall("GET", `/citizen/${cpf}/maintenance-request?page=1&per_page=10`, null, 200, token) }
    ];

    const selectedCards = walletCardActions.slice(0, Math.floor(Math.random() * 3) + 1);

    selectedCards.forEach((card, index) => {
        card.action();
        if (index < selectedCards.length - 1) {
            sleep(6);
        }
    });

    sleep(2);
}

function testLoggedOutCategoryBrowsing() {
    const collections = "servicos,beneficios,programas";

    const categoriesResponse = makePublicApiCall("GET", `/api/v1/categorias-relevancia?collections=${collections}`, null, 200);
    if (!categoriesResponse) return;

    let categories;
    try {
        categories = JSON.parse(categoriesResponse.body);
    } catch (error) {
        console.error("❌ Error parsing categories response:", error.message);
        return;
    }

    sleep(2);

    if (categories && categories.length > 0) {
        const randomCategory = categories[Math.floor(Math.random() * categories.length)];
        const categoryResponse = makePublicApiCall("GET", `/api/v1/categoria/${collections}?categoria=${encodeURIComponent(randomCategory.categoria || randomCategory.name)}`, null, 200);

        sleep(3);

        if (categoryResponse) {
            let services;
            try {
                services = JSON.parse(categoryResponse.body);
            } catch (error) {
                console.error("❌ Error parsing services response:", error.message);
                return;
            }

            if (services && services.length > 0) {
                const randomService = services[Math.floor(Math.random() * services.length)];
                sleep(2);
                makePublicApiCall("GET", `/api/v1/documento/${randomService.collection || 'servicos'}/${randomService.id}`, null, 200);

                sleep(10);
            }
        }
    }
}

function testLoggedOutSearch() {
    const collections = "servicos,beneficios,programas";
    const searchTerms = ["saude", "educacao", "transporte", "habitacao", "emprego", "trabalho", "auxilio", "carteira"];
    const randomTerm = searchTerms[Math.floor(Math.random() * searchTerms.length)];

    const searchResponse = makePublicApiCall("GET", `/api/v1/busca-hibrida-multi?collections=${collections}&q=${encodeURIComponent(randomTerm)}`, null, 200);

    sleep(3);

    if (searchResponse) {
        let searchResults;
        try {
            searchResults = JSON.parse(searchResponse.body);
        } catch (error) {
            console.error("❌ Error parsing search response:", error.message);
            return;
        }

        if (searchResults && searchResults.results && searchResults.results.length > 0) {
            const randomResult = searchResults.results[Math.floor(Math.random() * searchResults.results.length)];
            sleep(2);
            makePublicApiCall("GET", `/api/v1/documento/${randomResult.collection || 'servicos'}/${randomResult.id}`, null, 200);

            sleep(12);
        }
    }
}

function testLoggedOutPopularServices() {
    makePublicApiCall("GET", `/api/v1/documento/servicos/popular`, null, 200);

    sleep(8);

    const collections = "servicos,beneficios,programas";
    makePublicApiCall("GET", `/api/v1/categorias-relevancia?collections=${collections}`, null, 200);

    sleep(5);
}
function testLegacyLoad(data) {
    const token = data ? data.token : null;
    const headers = {
        "Content-Type": "application/json",
    };

    if (token) {
        headers["Authorization"] = `Bearer ${token}`;
    }

    const response = http.get(BASE_URL_RMI, {
        headers: headers,
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

function runFlow(flow, data) {
    flow(data);
    sleep(Math.floor(Math.random() * 3) + 1);
}

export function firstLoginOnboarding(data) {
    runFlow(testFirstLoginOnboarding, data);
}

export function homeDashboard(data) {
    runFlow(testHomeDashboardAccess, data);
}

export function personalInfoUpdate(data) {
    runFlow(testPersonalInfoUpdate, data);
}

export function walletCards(data) {
    runFlow(testWalletCardInteractions, data);
}

export function categoryBrowsing() {
    runFlow(testLoggedOutCategoryBrowsing);
}

export function searchExperience() {
    runFlow(testLoggedOutSearch);
}

export function popularServices() {
    runFlow(testLoggedOutPopularServices);
}

export function legacy(data) {
    runFlow(testLegacyLoad, data);
}

export function setup() {
    console.log("🚀 Setting up load tests...");
    console.log(`📝 Test scenario: ${TEST_SCENARIO}`);
    console.log(`👥 Virtual users: ${VIRTUAL_USERS}`);
    console.log(`⏱️ Stages: ramp-up ${RAMP_UP_DURATION}, steady ${STEADY_DURATION}, ramp-down ${RAMP_DOWN_DURATION}`);
    console.log(`📱 Phone writes (PUT /citizen/{cpf}/phone): ${ENABLE_PHONE_WRITES ? "enabled" : "disabled"}`);
    console.log(`🎯 Base URL RMI: ${BASE_URL_RMI}`);
    console.log(`🔍 Base URL Busca: ${BASE_URL_BUSCA}`);
    console.log(`🆔 Test CPF: ${USER}`);
    console.log("📊 Metrics tracking:");
    console.log("   • failed_requests: HTTP failures (network/5xx errors)");
    console.log("   • http_failures: Same as failed_requests (detailed tracking)");
    console.log("   • check_failures: Quality issues (slow responses, wrong status)");

    if (!KEYCLOAK_ISSUER || !KEYCLOAK_CLIENT_ID || !KEYCLOAK_CLIENT_SECRET) {
        console.error("❌ Authentication required: Missing Keycloak environment variables");
        console.error("   Please set: KEYCLOAK_ISSUER, KEYCLOAK_CLIENT_ID, and KEYCLOAK_CLIENT_SECRET");
        throw new Error("Authentication configuration missing");
    }

    console.log("🔐 Fetching Keycloak JWT token...");

    const tokenUrl = `${KEYCLOAK_ISSUER}/protocol/openid-connect/token`;
    const tokenData = {
        client_id: KEYCLOAK_CLIENT_ID,
        client_secret: KEYCLOAK_CLIENT_SECRET,
        grant_type: 'client_credentials',
        scope: 'profile email'
    };

    const response = http.post(tokenUrl, tokenData, {
        headers: {
            'Content-Type': 'application/x-www-form-urlencoded'
        },
        timeout: "30s"
    });

    if (response.status !== 200) {
        console.error(`❌ Failed to obtain JWT token: ${response.status}`);
        console.error(`Response: ${response.body}`);
        throw new Error(`Authentication failed with status: ${response.status}`);
    }

    let tokens;
    try {
        tokens = JSON.parse(response.body);
    } catch (error) {
        console.error("❌ Error parsing token response:", error.message);
        throw new Error("Invalid token response format");
    }

    if (!tokens.access_token) {
        console.error("❌ No access token in response");
        console.error(`Response: ${response.body}`);
        throw new Error("No access token received");
    }

    console.log("✅ JWT token obtained successfully");
    console.log(`🔑 Token type: ${tokens.token_type || 'Bearer'}`);
    console.log(`⏰ Token expires in: ${tokens.expires_in || 'unknown'} seconds`);
    console.log("✅ Setup completed - Improved failure tracking enabled");

    return {
        token: tokens.access_token
    };
}
export function teardown() {
    console.log("🧹 Cleaning up load tests...");
    console.log("✅ Test cleanup completed");
}
