/**
 * Run the browser checks with the frontend they need.
 *
 * `test/ui.mjs` only drives a browser: it navigates straight to whatever
 * `EVERROOT_WEB_URL` says (default `http://127.0.0.1:5173`) and assumes a dev
 * server is already answering there. Run on its own it fails with
 * `net::ERR_CONNECTION_REFUSED`, and every later assertion reports a missing
 * element because the page never rendered — a cascade that hides the one cause.
 *
 * This wrapper owns a frontend for the run:
 *
 *   1. it starts its own Vite dev server (cwd `apps/web`, host 127.0.0.1)
 *      unless the caller explicitly supplied one with `EVERROOT_WEB_URL`;
 *   2. it never adopts a process it did not start, and never binds a port
 *      something else is using — it moves to the next free port instead, so a
 *      stale or half-dead dev server cannot poison the run;
 *   3. it waits until the server answers twice in a row, then re-checks
 *      liveness immediately before Playwright starts and while it runs;
 *   4. it gives Vite a stdin it keeps open, because Vite treats stdin end as
 *      SIGTERM (`setupSIGTERMListener` in vite/dist) and would otherwise close
 *      itself down mid-run;
 *   5. it checks the API that the Vite proxy forwards `/api` to and says so
 *      plainly when that API is not up;
 *   6. it always stops the server it started — the whole process group, SIGTERM
 *      then SIGKILL — and prints the captured server output on any failure.
 *
 * The API is deliberately NOT started here. It needs a database, and a test
 * harness must not choose one: `/api/v1/discoveries` is a write endpoint, so
 * picking the wrong database would modify real rows. Start it yourself against
 * a local database (see README), or point `EVERROOT_API_URL` at a running one.
 *
 * Environment:
 *   EVERROOT_WEB_URL    explicit frontend; used as-is, never started or stopped
 *   EVERROOT_API_URL    API the proxy target check uses (default :8080)
 *   EVERROOT_WEB_PORT   fixed port for the server this wrapper starts
 *   EVERROOT_READY_TIMEOUT_MS  how long to wait for the frontend (default 90000)
 *   EVERROOT_UI_LOG_DIR where the captured dev-server log is written
 */
import { spawn } from 'node:child_process';
import fs from 'node:fs';
import net from 'node:net';
import os from 'node:os';
import path from 'node:path';
import process from 'node:process';
import { setTimeout as delay } from 'node:timers/promises';
import { fileURLToPath } from 'node:url';

const here = path.dirname(fileURLToPath(import.meta.url));
const web = path.resolve(here, '..');

const explicitWebURL = (process.env.EVERROOT_WEB_URL ?? '').replace(/\/+$/, '');
/** The Vite proxy target in vite.config.ts, i.e. the API these checks talk to. */
const API_ORIGIN = (process.env.EVERROOT_API_URL ?? 'http://127.0.0.1:8080').replace(/\/+$/, '');
const READY_TIMEOUT_MS = Number(process.env.EVERROOT_READY_TIMEOUT_MS ?? 90_000);
const LOG_DIR = process.env.EVERROOT_UI_LOG_DIR ?? path.join(os.tmpdir(), 'everroot-ui-tests');
const LOG_KEEP_LINES = 200;
const DEFAULT_PORT = 5173;
const PORT_ATTEMPTS = 20;

/** A failure of the harness or its servers, not of the application. */
class InfrastructureError extends Error {}

let viteChild = null;
let viteLog = [];
let viteLogFile = '';
let frontendExit = null; // string once our own server has exited
let runningChecks = false;
let tearingDown = false;
let diedWhileRunning = false;
let startedOrigin = ''; // set only when this run started the frontend itself

/* --------------------------------------------------------------- utilities */

async function probe(url, timeoutMs = 5_000) {
  try {
    const res = await fetch(url, { signal: AbortSignal.timeout(timeoutMs), redirect: 'manual' });
    return { status: res.status, body: await res.text() };
  } catch {
    return null;
  }
}

/** Is this the Everroot dev server, or some unrelated process on the port? */
function looksLikeTheApp(page) {
  return (
    page !== null &&
    page.status === 200 &&
    (/Everroot/.test(page.body) || /_app\/immutable/.test(page.body) || /@vite\/client/.test(page.body))
  );
}

function findVite() {
  let dir = web;
  for (;;) {
    const candidate = path.join(dir, 'node_modules', 'vite', 'bin', 'vite.js');
    if (fs.existsSync(candidate)) return candidate;
    const parent = path.dirname(dir);
    if (parent === dir) break;
    dir = parent;
  }
  throw new InfrastructureError(
    `could not find vite under ${web}/node_modules — run \`npm install\` in apps/web first`
  );
}

function remember(text) {
  for (const line of text.split('\n')) {
    const kept = line.trimEnd();
    if (!kept) continue;
    viteLog.push(kept);
    if (viteLog.length > LOG_KEEP_LINES) viteLog.shift();
  }
}

function printViteLog(why) {
  if (!viteLog.length && !viteLogFile) return;
  console.log(`\n---- dev server output (${why}) ----`);
  for (const line of viteLog) console.log(`  ${line}`);
  if (viteLogFile && fs.existsSync(viteLogFile)) console.log(`  full log: ${viteLogFile}`);
}

function portIsFree(port) {
  return new Promise((resolve) => {
    const server = net.createServer();
    server.once('error', () => resolve(false));
    server.once('listening', () => server.close(() => resolve(true)));
    server.listen(port, '127.0.0.1');
  });
}

function killTree(child, signal) {
  // `detached: true` makes the child a process-group leader, so a negative pid
  // reaches Vite together with anything it spawned (esbuild, etc.).
  try {
    process.kill(-child.pid, signal);
  } catch {
    try {
      child.kill(signal);
    } catch {
      /* already gone */
    }
  }
}

/* ------------------------------------------------------------ frontend server */

async function pickPort() {
  const explicit = process.env.EVERROOT_WEB_PORT;
  if (explicit) {
    const port = Number(explicit);
    if (!Number.isInteger(port) || port <= 0 || port > 65_535) {
      throw new InfrastructureError(`EVERROOT_WEB_PORT must be a port number, got "${explicit}"`);
    }
    if (!(await portIsFree(port))) {
      throw new InfrastructureError(
        `EVERROOT_WEB_PORT=${port} is already in use — free it, or unset EVERROOT_WEB_PORT to let this run pick a free port`
      );
    }
    return port;
  }
  for (let port = DEFAULT_PORT; port < DEFAULT_PORT + PORT_ATTEMPTS; port += 1) {
    if (await portIsFree(port)) return port;
  }
  throw new InfrastructureError(
    `no free port between ${DEFAULT_PORT} and ${DEFAULT_PORT + PORT_ATTEMPTS - 1} for the dev server`
  );
}

async function startFrontend(origin, port) {
  viteLogFile = path.join(LOG_DIR, 'vite.log');
  fs.mkdirSync(LOG_DIR, { recursive: true });
  fs.writeFileSync(viteLogFile, '');
  viteLog = [];

  const vite = findVite();
  console.log(`starting the frontend dev server on ${origin} (cwd apps/web) ...`);
  viteChild = spawn(
    process.execPath,
    [vite, '--host', '127.0.0.1', '--port', String(port), '--strictPort'],
    {
      cwd: web,
      // `stdin: 'pipe'` is deliberate: Vite calls process.exit() when its stdin
      // ends (it is a SIGTERM equivalent). Keeping the write end open, and never
      // ending it, means the dev server cannot shut itself down mid-run.
      stdio: ['pipe', 'pipe', 'pipe'],
      detached: true,
    }
  );
  viteChild.stdin.on('error', () => {
    /* the child may exit before we ever write; nothing is written here anyway */
  });

  const record = (chunk) => {
    const text = chunk.toString();
    try {
      fs.appendFileSync(viteLogFile, text);
    } catch {
      /* the log file is a convenience; never let it fail the run */
    }
    remember(text);
  };
  viteChild.stdout.on('data', record);
  viteChild.stderr.on('data', record);

  viteChild.once('exit', (code, signal) => {
    frontendExit = signal ? `signal ${signal}` : `exit code ${code}`;
    // A shutdown we asked for is not a failure; a shutdown during the checks is.
    if (tearingDown || !runningChecks) return;
    diedWhileRunning = true;
    console.log(`\n!! the dev server at ${origin} exited while the checks were running (${frontendExit}).`);
    console.log('!! Any net::ERR_CONNECTION_REFUSED or missing-element failure after this point is that,');
    console.log('!! not the interface: those checks were not run against a live app.');
    printViteLog('the dev server died mid-run');
  });
}

/** Wait until the server has answered the app twice in a row (it can restart early on). */
async function waitUntilReady(origin) {
  const deadline = Date.now() + READY_TIMEOUT_MS;
  let consecutive = 0;
  while (Date.now() < deadline) {
    if (frontendExit) {
      throw new InfrastructureError(
        `the dev server exited before it was ready (${frontendExit}); its output is below`
      );
    }
    const page = await probe(`${origin}/discover`);
    consecutive = looksLikeTheApp(page) ? consecutive + 1 : 0;
    if (consecutive >= 2) return;
    await delay(500);
  }
  throw new InfrastructureError(
    `the dev server did not serve this app at ${origin} within ${Math.round(READY_TIMEOUT_MS / 1000)}s`
  );
}

/** The gap between "ready" and "Playwright starts" must not hide a dead server. */
async function ensureStillAlive(origin) {
  for (let attempt = 0; attempt < 10; attempt += 1) {
    if (frontendExit) {
      throw new InfrastructureError(
        `the frontend at ${origin} exited before the checks started (${frontendExit}); its output is below`
      );
    }
    if (looksLikeTheApp(await probe(`${origin}/discover`))) return;
    await delay(300);
  }
  throw new InfrastructureError(`the frontend at ${origin} stopped answering before the checks started`);
}

async function stopFrontend() {
  tearingDown = true;
  const child = viteChild;
  viteChild = null;
  if (!child || child.exitCode !== null || child.signalCode !== null) return;

  const stopped = new Promise((resolve) => child.once('exit', resolve));
  killTree(child, 'SIGTERM');
  const ended = await Promise.race([stopped.then(() => true), delay(5_000).then(() => false)]);
  if (!ended) {
    killTree(child, 'SIGKILL');
    await Promise.race([stopped, delay(2_000)]);
  }
  console.log(`stopped the dev server at ${startedOrigin}`);
}

/* ------------------------------------------------------------------ the API */

async function checkAPI({ required }) {
  const health = await probe(`${API_ORIGIN}/healthz`);
  if (!health || health.status !== 200) {
    const message =
      `nothing healthy is answering at ${API_ORIGIN}, which is where the Vite proxy sends /api\n` +
      '  The checks call the real catalogue, so the API has to be running before them.\n' +
      '  Start it against a local database, for example:\n' +
      "    DATABASE_URL='postgres://…/everroot?sslmode=disable' go run ./cmd/api\n" +
      '  or point these checks at a running pair:\n' +
      '    EVERROOT_WEB_URL=http://127.0.0.1:5173 EVERROOT_API_URL=http://127.0.0.1:8080 npm run test:ui';
    if (required) throw new InfrastructureError(message);
    console.log(`\n!! the API the proxy forwards /api to is not answering at ${API_ORIGIN}.`);
    console.log('!! catalogue and tree checks will fail for that reason, not because of the interface.\n');
    return;
  }

  const ready = await probe(`${API_ORIGIN}/readyz`);
  if (!ready || ready.status !== 200) {
    const detail = ready ? `HTTP ${ready.status} ${ready.body.slice(0, 200)}` : 'no answer';
    console.log(
      `\n!! the API at ${API_ORIGIN} is up but its database is not ready (${detail}).\n` +
        '!! The catalogue and tree checks will fail for that reason, not because of the interface:\n' +
        '!! check /readyz and DATABASE_URL before reading anything else below.\n'
    );
  }
}

/* --------------------------------------------------------------------- run */

function runChecks(origin) {
  runningChecks = true;
  const child = spawn(process.execPath, [path.join(here, 'ui.mjs')], {
    cwd: web,
    // ui.mjs has no other way to be told where the frontend is.
    env: { ...process.env, EVERROOT_WEB_URL: origin },
    stdio: 'inherit',
  });
  return new Promise((resolve) => {
    child.once('exit', (code, signal) => {
      runningChecks = false;
      resolve(signal ? 1 : (code ?? 1));
    });
    child.once('error', (error) => {
      runningChecks = false;
      console.log(`could not start the browser checks: ${error.message}`);
      resolve(1);
    });
  });
}

async function main() {
  let origin;

  if (explicitWebURL) {
    origin = explicitWebURL;
    if (!looksLikeTheApp(await probe(`${origin}/discover`))) {
      throw new InfrastructureError(
        `EVERROOT_WEB_URL is set to ${origin} but nothing serving this app answers there.\n` +
          '  Start that server first, or unset EVERROOT_WEB_URL to let these checks start their own.'
      );
    }
    console.log(`using the frontend you supplied at ${origin} (this run will not start or stop it)`);
    await checkAPI({ required: false });
  } else {
    const port = await pickPort();
    origin = `http://127.0.0.1:${port}`;
    if (port !== DEFAULT_PORT) {
      console.log(
        `port ${DEFAULT_PORT} is in use, so the checks' own dev server goes on ${port} instead.\n` +
          `(To test the server you already run: EVERROOT_WEB_URL=http://127.0.0.1:${DEFAULT_PORT} npm run test:ui)`
      );
    }
    startedOrigin = origin;
    await startFrontend(origin, port);
    await waitUntilReady(origin);
    console.log(`frontend ready at ${origin}`);
    await ensureStillAlive(origin);
    await checkAPI({ required: true });
  }

  const code = await runChecks(origin);

  // A frontend we did not start has no exit event to watch, so check on it once.
  if (!startedOrigin && code !== 0 && !looksLikeTheApp(await probe(`${origin}/discover`))) {
    console.log('\n---- infrastructure failure ----');
    console.log(
      `the frontend you supplied at ${origin} is not answering any more, so the pages could not\n` +
        'load above. The checks are not evidence about the interface; start that server again (or\n' +
        'unset EVERROOT_WEB_URL to have this run start its own) and re-run.'
    );
    return 1;
  }

  if (diedWhileRunning) {
    console.log('\n---- infrastructure failure ----');
    console.log(
      `the frontend this run started stopped answering mid-run (${frontendExit}).\n` +
        'That is what the failures above mean: the pages could not load, so the checks reported\n' +
        'missing elements. They are not evidence about the interface.'
    );
    printViteLog('the frontend died mid-run');
    return 1;
  }

  if (code !== 0) {
    console.log('\n---- result ----');
    console.log(
      'the browser checks failed. If a failure quotes net::ERR_CONNECTION_REFUSED, or reports a\n' +
        'missing element on a page that never finished loading, treat it as a server failure rather\n' +
        'than an application one; the checks themselves were not changed.'
    );
    printViteLog('the checks failed');
  }
  return code;
}

let exitCode = 1;

for (const signal of ['SIGINT', 'SIGTERM']) {
  process.on(signal, async () => {
    await stopFrontend();
    process.exit(signal === 'SIGINT' ? 130 : 143);
  });
}
// Last resort: never leave the dev server behind if we die unexpectedly.
process.on('exit', () => {
  if (viteChild && viteChild.exitCode === null && viteChild.signalCode === null) {
    killTree(viteChild, 'SIGKILL');
  }
});

try {
  exitCode = await main();
} catch (error) {
  exitCode = 1;
  console.log(`\n---- infrastructure failure ----\n${error instanceof Error ? error.message : error}`);
  if (error instanceof InfrastructureError) {
    console.log(
      '\nThis is not an application regression: the browser checks were not run, so nothing above\n' +
        'would have been evidence about the interface.'
    );
  }
  printViteLog('the harness failed');
} finally {
  await stopFrontend();
}

process.exit(exitCode);
