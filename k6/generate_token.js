#!/usr/bin/env node

import https from 'https';
import http from 'http';
import { stringify } from 'querystring';
import { URL } from 'url';

function getAccessToken() {
    const ISSUER = process.env.KEYCLOAK_ISSUER;
    const CLIENT_ID = process.env.KEYCLOAK_CLIENT_ID;
    const CLIENT_SECRET = process.env.KEYCLOAK_CLIENT_SECRET;

    if (!ISSUER || !CLIENT_ID || !CLIENT_SECRET) {
        console.error('Missing required environment variables: KEYCLOAK_ISSUER, KEYCLOAK_CLIENT_ID, KEYCLOAK_CLIENT_SECRET');
        process.exit(1);
    }

    const tokenUrl = new URL(`${ISSUER}/protocol/openid-connect/token`);
    const tokenData = stringify({
        client_id: CLIENT_ID,
        client_secret: CLIENT_SECRET,
        grant_type: 'client_credentials',
        scope: 'profile email'
    });

    const options = {
        hostname: tokenUrl.hostname,
        port: tokenUrl.port || (tokenUrl.protocol === 'https:' ? 443 : 80),
        path: tokenUrl.pathname,
        method: 'POST',
        headers: {
            'Content-Type': 'application/x-www-form-urlencoded',
            'Content-Length': Buffer.byteLength(tokenData)
        }
    };

    const httpModule = tokenUrl.protocol === 'https:' ? https : http;

    const req = httpModule.request(options, (res) => {
        let body = '';
        res.on('data', (chunk) => {
            body += chunk;
        });
        res.on('end', () => {
            if (res.statusCode === 200) {
                try {
                    const tokens = JSON.parse(body);
                    console.log(tokens.access_token);
                } catch (error) {
                    console.error('Error parsing token response:', error.message);
                    process.exit(1);
                }
            } else {
                console.error(`Error obtaining token: ${res.statusCode}`);
                console.error(body);
                process.exit(1);
            }
        });
    });

    req.on('error', (error) => {
        console.error('Request error:', error.message);
        process.exit(1);
    });

    req.write(tokenData);
    req.end();
}

getAccessToken();
