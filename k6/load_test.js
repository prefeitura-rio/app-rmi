import http from "k6/http";
import { check, sleep } from "k6";

const ISSUER = __ENV.KEYCLOAK_ISSUER;
const CLIENT_ID = __ENV.KEYCLOAK_CLIENT_ID;
const CLIENT_SECRET = __ENV.KEYCLOAK_CLIENT_SECRET;
const TARGET_URL = __ENV.TARGET_URL;

let globalAccessToken = null;
let tokenExpiryTime = 0;

/**
 * Gets a new token from Keycloak using Client Credentials Grant
 * @returns {string|null} Access token or null on error
 */
function getNewToken() {
    const tokenUrl = `${ISSUER}/protocol/openid-connect/token`;

    const tokenData = {
        client_id: CLIENT_ID,
        client_secret: CLIENT_SECRET,
        grant_type: "client_credentials",
        scope: "openid"
    };

    const tokenResponse = http.post(tokenUrl, tokenData, {
        headers: { "Content-Type": "application/x-www-form-urlencoded" },
        timeout: "30s"
    });

    if (tokenResponse.status === 200) {
        const tokens = JSON.parse(tokenResponse.body);

        globalAccessToken = tokens.access_token;
        tokenExpiryTime = Date.now() + (tokens.expires_in * 1000) - 60000;

        console.log(`✅ New token obtained. Expires in: ${tokens.expires_in} seconds`);
        return tokens.access_token;
    } else {
        console.error(`❌ Error obtaining token: ${tokenResponse.status} - ${tokenResponse.body}`);
        return null;
    }
}

/**
 * Gets a valid token (reuses cache if still valid, otherwise gets a new one)
 * @returns {string|null} Valid access token or null on error
 */
function getAccessToken() {
    if (globalAccessToken && Date.now() < tokenExpiryTime) {
        return globalAccessToken;
    }

    console.log("🔄 Token expired, renewing...");
    return getNewToken();
}

/**
 * Gets complete token with additional information
 * @returns {Object|null} Object with token and metadata or null on error
 */
function getTokenInfo() {
    const tokenUrl = `${ISSUER}/protocol/openid-connect/token`;

    const tokenData = {
        client_id: CLIENT_ID,
        client_secret: CLIENT_SECRET,
        grant_type: "client_credentials",
        scope: "openid"
    };

    const tokenResponse = http.post(tokenUrl, tokenData, {
        headers: { "Content-Type": "application/x-www-form-urlencoded" },
        timeout: "30s"
    });

    if (tokenResponse.status === 200) {
        const tokens = JSON.parse(tokenResponse.body);

        globalAccessToken = tokens.access_token;
        tokenExpiryTime = Date.now() + (tokens.expires_in * 1000) - 60000;

        return {
            access_token: tokens.access_token,
            token_type: tokens.token_type,
            expires_in: tokens.expires_in,
            scope: tokens.scope,
            expires_at: new Date(Date.now() + (tokens.expires_in * 1000)),
            created_at: new Date()
        };
    } else {
        console.error(`❌ Error obtaining token: ${tokenResponse.status} - ${tokenResponse.body}`);
        return null;
    }
}

/**
 * Validates if a token is working
 * @param {string} token - Token to validate
 * @returns {boolean} true if token is valid, false otherwise
 */
function validateToken(token) {
    const userinfoResponse = http.get(`${ISSUER}/protocol/openid-connect/userinfo`, {
        headers: {
            "Authorization": `Bearer ${token}`,
            "Content-Type": "application/json",
        },
        timeout: "10s"
    });

    return userinfoResponse.status === 200;
}

/**
 * Clears the token cache (useful in teardown)
 */
function clearTokenCache() {
    globalAccessToken = null;
    tokenExpiryTime = 0;
    console.log("🧹 Token cache cleared");
}

/**
 * Gets information about the current token
 * @returns {Object} Cache information
 */
function getTokenCacheInfo() {
    return {
        hasToken: globalAccessToken !== null,
        isExpired: Date.now() >= tokenExpiryTime,
        expiresIn: Math.max(0, Math.floor((tokenExpiryTime - Date.now()) / 1000)),
        expiresAt: new Date(tokenExpiryTime + 60000)
    };
}

export function setup() {
    console.log("🚀 Setting up load test...");

    const token = getAccessToken();

    if (!token) {
        throw new Error("Failed to obtain authentication token in setup");
    }

    console.log("✅ Authentication token obtained successfully");

    return { token };
}

export default function(data) {
    const token = getAccessToken();

    if (!token) {
        console.error("❌ No valid token available");
        return;
    }

    const response = http.get(TARGET_URL, {
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
        console.error(`❌ Request failed: ${response.status} - ${response.statusText}`);
    }

    sleep(1);
}

export function teardown() {
    console.log("🧹 Cleaning up load test...");
    clearTokenCache();
    console.log("✅ Load test cleanup completed");
}
