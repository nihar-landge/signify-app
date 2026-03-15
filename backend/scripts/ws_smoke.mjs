const backendUrl = process.env.BACKEND_URL ?? "http://127.0.0.1:8080";

function toWebSocketUrl(httpUrl) {
  const url = new URL(httpUrl);
  url.protocol = url.protocol === "https:" ? "wss:" : "ws:";
  return url;
}

function sleep(ms) {
  return new Promise((resolve) => setTimeout(resolve, ms));
}

async function startSession() {
  const response = await fetch(`${backendUrl}/api/sessions/start`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ to: "+15551234567" }),
  });

  if (!response.ok) {
    throw new Error(`start session failed: ${response.status}`);
  }

  const payload = await response.json();
  if (!payload.sessionId) {
    throw new Error("start session response missing sessionId");
  }
  return payload.sessionId;
}

async function run() {
  const sessionId = await startSession();
  const wsUrl = toWebSocketUrl(backendUrl);
  wsUrl.pathname = "/ws/app/landmarks";
  wsUrl.searchParams.set("sessionId", sessionId);
  wsUrl.searchParams.set("language", "ASL");

  const frame = new Array(1629).fill(0);
  frame[1503] = 0.52;
  frame[1504] = 0.38;
  frame[1566] = 0.49;
  frame[1567] = 0.41;

  const ws = new WebSocket(wsUrl.toString());

  let resolveDone;
  let rejectDone;
  const done = new Promise((resolve, reject) => {
    resolveDone = resolve;
    rejectDone = reject;
  });

  let eventCount = 0;

  ws.onopen = async () => {
    const now = Date.now();
    for (let i = 0; i < 30; i += 1) {
      ws.send(
        JSON.stringify({
          sessionId,
          seq: i,
          ts_ms: now + i * 80,
          present: true,
          landmarks: frame,
          normalized: true,
        }),
      );
      await sleep(50);
    }

    setTimeout(() => {
      if (eventCount > 0) {
        resolveDone(eventCount);
      } else {
        rejectDone(new Error("no sign events received"));
      }
      ws.close();
    }, 6000);
  };

  ws.onmessage = (event) => {
    try {
      const payload = JSON.parse(event.data);
      if (payload.type === "sign_partial" || payload.type === "sign_final") {
        eventCount += 1;
      }
    } catch {
      // ignore non-JSON payloads
    }
  };

  ws.onerror = () => {
    rejectDone(new Error("landmarks websocket error"));
  };

  const count = await done;
  console.log(`landmark events received: ${count}`);
}

run().catch((error) => {
  console.error(error.message);
  process.exit(1);
});
