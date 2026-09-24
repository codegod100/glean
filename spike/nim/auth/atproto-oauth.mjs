#!/usr/bin/env node
/** Durable AT Protocol OAuth bridge, adapted from ~/code-editor. */
import { mkdir, readFile, rename, unlink, writeFile } from 'node:fs/promises';
import { createHash, randomBytes } from 'node:crypto';
import { join } from 'node:path';
import { NodeOAuthClient } from '@atproto/oauth-client-node';
import { requestLocalLock } from '@atproto/oauth-client';

const root = process.env.PULSEBOARD_OAUTH_ROOT;
const appUrl = process.env.PULSEBOARD_PUBLIC_URL?.replace(/\/$/, '');
if (!root || !appUrl) {
  throw new Error('PULSEBOARD_OAUTH_ROOT and PULSEBOARD_PUBLIC_URL are required');
}

const stateDir = join(root, 'states');
const sessionDir = join(root, 'sessions');

function durableStore(directory) {
  const pathFor = (key) => join(
    directory,
    createHash('sha256').update(key).digest('hex') + '.json',
  );
  return {
    async get(key) {
      try {
        return JSON.parse(await readFile(pathFor(key), 'utf8'));
      } catch (error) {
        if (error.code === 'ENOENT') return undefined;
        throw error;
      }
    },
    async set(key, value) {
      await mkdir(directory, { recursive: true, mode: 0o700 });
      const path = pathFor(key);
      const temporary = `${path}.${process.pid}.${randomBytes(8).toString('hex')}.next`;
      await writeFile(temporary, JSON.stringify(value), { mode: 0o600 });
      await rename(temporary, path);
    },
    async del(key) {
      try {
        await unlink(pathFor(key));
      } catch (error) {
        if (error.code !== 'ENOENT') throw error;
      }
    },
  };
}

const client = new NodeOAuthClient({
  clientMetadata: {
    client_id: `${appUrl}/oauth-client-metadata.json`,
    client_name: 'Pulseboard',
    client_uri: appUrl,
    redirect_uris: [`${appUrl}/auth/callback`],
    grant_types: ['authorization_code', 'refresh_token'],
    response_types: ['code'],
    scope: 'atproto',
    token_endpoint_auth_method: 'none',
    dpop_bound_access_tokens: true,
  },
  requestLock: requestLocalLock,
  stateStore: durableStore(stateDir),
  sessionStore: durableStore(sessionDir),
});

const input = JSON.parse(await new Promise((resolve, reject) => {
  let value = '';
  process.stdin.setEncoding('utf8');
  process.stdin.on('data', (chunk) => { value += chunk; });
  process.stdin.on('end', () => resolve(value || '{}'));
  process.stdin.on('error', reject);
}));

const command = process.argv[2];
let result;
if (command === 'metadata') {
  result = client.clientMetadata;
} else if (command === 'authorize') {
  result = { url: String(await client.authorize(input.identity, { scope: 'atproto' })) };
} else if (command === 'callback') {
  const params = new URLSearchParams(input.params);
  try {
    const { session } = await client.callback(params);
    // Pulseboard needs verified identity, not ongoing access to the user's PDS.
    await client.revoke(session.did);
    result = { did: session.did };
  } catch (error) {
    // OAuth errors and missing state are expected when a user returns to an
    // expired, cancelled, or already-consumed authorization page. Do not leak
    // library stack traces or weaken state validation; ask them to start over.
    if (params.has('error') || error?.name === 'OAuthCallbackError') {
      result = {
        error: params.get('error_description') || 'This login is no longer valid.',
        retry: true,
      };
    } else {
      throw error;
    }
  }
} else {
  throw new Error(`unknown command: ${command}`);
}

process.stdout.write(JSON.stringify(result));
