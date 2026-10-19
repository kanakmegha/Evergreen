/**
 * Answer every request with one stub response, recording the URLs asked for
 * and the bodies sent. Nothing here touches the network.
 */
export function stubFetch(status = 200, body: unknown = []) {
  const calls: string[] = [];
  const bodies: string[] = [];
  const original = globalThis.fetch;
  globalThis.fetch = (async (input: RequestInfo | URL, init?: RequestInit) => {
    calls.push(String(input));
    bodies.push(String(init?.body ?? ''));
    return {
      ok: status >= 200 && status < 300,
      status,
      json: async () => body,
    } as Response;
  }) as typeof fetch;
  return {
    calls,
    bodies,
    restore() {
      globalThis.fetch = original;
    },
  };
}

/** Fail every request at the transport, as an offline browser would. */
export function stubFetchFailure(message = 'network down') {
  const original = globalThis.fetch;
  globalThis.fetch = (async () => {
    throw new Error(message);
  }) as typeof fetch;
  return {
    restore() {
      globalThis.fetch = original;
    },
  };
}
