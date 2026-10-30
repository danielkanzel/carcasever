const prefs = {
  get(key, fallback) {
    const v = localStorage.getItem("carca_" + key);
    return v == null || v === "" ? fallback : v;
  },
  set(key, value) {
    localStorage.setItem("carca_" + key, value);
  },
};

let catalogById = {};
let session = null;
let view = "menu";
let settingsReturn = "menu";
let table = null;
let socket = null;
let socketGame = "";
let retry = 0;
let browseTimer = 0;
let leaving = false;
let clockTimer = 0;
let clockMarkKey = "";
let clockPlayed = new Set();
let cam = { x: 40, y: 40, z: 1 };
let camReady = false;
let rot = 0;
let hover = null;
let hotSpot = null;
let spotHits = [];
let drag = null;
let drawQueued = false;
let lastEventKey = "";
let lastTurn = "";
let audioCtx = null;

function quality() {
  return prefs.get("quality", "med");
}
function soundOn() {
  return prefs.get("sound", "1") !== "0";
}

async function api(path, opts = {}) {
  const headers = { "Content-Type": "application/json" };
  const token = prefs.get("token", "");
  if (token) headers.Authorization = "Bearer " + token;
  const res = await fetch(path, {
    method: opts.method || "GET",
    headers,
    body: opts.body ? JSON.stringify(opts.body) : undefined,
  });
  const data = await res.json().catch(() => ({}));
  if (!res.ok) {
    const err = new Error(data.error || "bad_request");
    err.code = data.error || "bad_request";
    if (res.status === 401) {
      session = null;
      prefs.set("token", "");
    }
    throw err;
  }
  return data;
}

function toast(msg) {
  const el = document.createElement("div");
  el.className = "toast";
  el.textContent = msg;
  document.getElementById("toasts").appendChild(el);
  setTimeout(() => el.remove(), 3400);
}

function fail(err) {
  const code = err && (err.code || err.message);
  toast(t("err_" + code) === "err_" + code ? t("err_bad_request") : t("err_" + code));
}

function show(name) {
  view = name;
  for (const id of ["menu", "register", "login", "browse", "ranks", "rules", "settings", "lobby", "game", "tiles"]) {
    document.getElementById("view-" + id).hidden = id !== name;
  }
  clearInterval(browseTimer);
  document.body.classList.toggle("at-menu", name === "menu");
  if (name === "menu") {
    document.getElementById("go-resume").hidden = !prefs.get("game", "");
    paintAccount();
    playMenuSlides();
  }
  if (name === "register") {
    document.getElementById("reg-name").value = document.getElementById("name").value;
  }
  if (name === "ranks") loadRanks();
  if (name === "browse") {
    loadBrowse();
    browseTimer = setInterval(loadBrowse, 4000);
  }
  if (name === "game") requestAnimationFrame(drawBoard);
  if (name === "settings") paintSettings();
}

let menuSlidesReady = false;

async function playMenuSlides() {
  if (menuSlidesReady) return;
  menuSlidesReady = true;
  const root = document.querySelector(".menu-bg");
  if (!root) return;
  const have = [...root.querySelectorAll(".menu-slide")];
  const sizes = new Set();
  await Promise.all(have.map(async (img) => {
    try {
      const res = await fetch(img.getAttribute("src"), { method: "HEAD" });
      const len = res.headers.get("content-length");
      if (len) sizes.add(len);
    } catch {
      /* the slide stays in the rotation */
    }
  }));
  const names = ["output.png"];
  for (let n = 2; n <= 24; n++) names.push("output" + n + ".png");
  const known = new Set(have.map((img) => img.getAttribute("src")));
  const found = await Promise.all(names.map(async (name) => {
    const url = "/img/backgrounds/" + name;
    if (known.has(url)) return null;
    try {
      const res = await fetch(url, { method: "HEAD" });
      if (!res.ok) return null;
      return { url, len: res.headers.get("content-length") || "" };
    } catch {
      return null;
    }
  }));
  for (const item of found) {
    if (!item || (item.len && sizes.has(item.len))) continue;
    if (item.len) sizes.add(item.len);
    const img = document.createElement("img");
    img.className = "menu-slide";
    img.alt = "";
    img.src = item.url;
    root.append(img);
  }
  const slides = [...root.querySelectorAll(".menu-slide")];
  if (!slides.some((el) => el.classList.contains("is-on")) && slides[0]) slides[0].classList.add("is-on");
  if (slides.length < 2 || window.matchMedia("(prefers-reduced-motion: reduce)").matches) return;
  let i = Math.max(0, slides.findIndex((el) => el.classList.contains("is-on")));
  setInterval(() => {
    slides[i].classList.remove("is-on");
    i = (i + 1) % slides.length;
    slides[i].classList.add("is-on");
  }, 8000);
}

async function ensureSession() {
  const name = document.getElementById("name").value.trim();
  if (!name) {
    toast(t("err_name_required"));
    document.getElementById("name").focus();
    return null;
  }
  prefs.set("name", name);
  if (!session) {
    session = await api("/api/session", { method: "POST", body: { name } });
    prefs.set("token", session.token);
  } else if (session.name !== name) {
    const me = await api("/api/session", { method: "PATCH", body: { name } });
    session.name = me.name;
  }
  paintWho();
  return session;
}

function paintWho() {
  const who = document.getElementById("who");
  if (!session) {
    who.textContent = "";
    return;
  }
  who.textContent = session.login ? session.name + " · " + session.login : session.name;
}

function paintAccount() {
  const registered = !!(session && session.login);
  const reg = document.getElementById("go-register");
  const out = document.getElementById("go-logout");
  if (reg) reg.hidden = registered;
  if (out) out.hidden = !registered;
}

function adoptSession(me) {
  session = { id: me.id, name: me.name, token: me.token, login: me.login || "" };
  prefs.set("token", me.token);
  prefs.set("name", me.name);
  document.getElementById("name").value = me.name;
  paintWho();
  paintAccount();
  if (me.activeGame) {
    prefs.set("game", me.activeGame);
    connect(me.activeGame);
    return true;
  }
  return false;
}

async function boot() {
  applyI18n();
  document.getElementById("name").value = prefs.get("name", "");
  paintSettings();
  const catalog = await api("/api/catalog");
  catalogById = Object.fromEntries(catalog.map((d) => [d.id, d]));
  loadTileImages(catalog.map((d) => d.id));
  if (new URLSearchParams(location.search).has("tiles")) {
    show("tiles");
    drawGallery();
    return;
  }
  const token = prefs.get("token", "");
  if (token) {
    try {
      const me = await api("/api/me");
      session = { id: me.id, name: me.name, token, login: me.login || "" };
      document.getElementById("name").value = me.name;
      paintWho();
      prefs.set("name", me.name);
    } catch {
      session = null;
    }
  }
  const gameId = prefs.get("game", "");
  if (session && gameId) {
    try {
      const g = await api("/api/games/" + gameId);
      connect(g.id);
      return;
    } catch {
      prefs.set("game", "");
    }
  }
  show("menu");
}

function connect(gameId) {
  prefs.set("game", gameId);
  socketGame = gameId;
  if (socket) {
    socket.onclose = null;
    socket.close();
  }
  const proto = location.protocol === "https:" ? "wss" : "ws";
  const token = prefs.get("token", "");
  const ws = new WebSocket(`${proto}://${location.host}/ws?token=${encodeURIComponent(token)}&game=${encodeURIComponent(gameId)}`);
  socket = ws;
  ws.onopen = () => {
    retry = 0;
    ws.send(JSON.stringify({ type: "hello" }));
  };
  ws.onmessage = (ev) => {
    const msg = JSON.parse(ev.data);
    if (msg.type === "state") onState(msg.state);
    else if (msg.type === "gone") {
      prefs.set("game", "");
      table = null;
      toast(t("gone"));
      show("menu");
    } else if (msg.type === "error") {
      const key = "err_" + msg.error;
      const text = t(key);
      toast(text === key ? msg.error : text);
    }
  };
  ws.onclose = () => {
    if (socket !== ws || !prefs.get("game", "")) return;
    retry = Math.min(retry + 1, 6);
    toast(t("link"));
    setTimeout(() => {
      if (prefs.get("game", "") === gameId) connect(gameId);
    }, 500 * retry);
  };
}

function send(cmd) {
  if (!socket || socket.readyState !== 1) return;
  socket.send(JSON.stringify(cmd));
}

function onState(state) {
  const mine = state.players.some((p) => session && p.id === session.id);
  if (!mine) {
    if (leaving) return;
    prefs.set("game", "");
    table = null;
    const idle = (state.events || []).some((ev) => ev.kind === "idle" && ev.name === session.name);
    toast(idle ? t("idled") : t("kicked"));
    show("menu");
    if (socket) socket.close();
    return;
  }
  const prev = table;
  const prevStatus = prev && prev.status;
  if (state.status === "meeple" && state.tiles && state.tiles.length) {
    rot = state.tiles[state.tiles.length - 1].rot;
  } else if (state.status === "place" && prev && prev.status === "meeple" && prev.tiles && prev.tiles.length) {
    const last = prev.tiles[prev.tiles.length - 1];
    if (last.id === state.currentId) rot = last.rot;
  }
  table = state;
  noteSounds(state);
  if (state.status === "lobby") {
    if (view !== "settings") show("lobby");
    renderLobby();
  } else {
    if (view !== "settings") show("game");
    if (prevStatus === "lobby") camReady = false;
    renderGame();
    if (view === "game" && !camReady) {
      camReady = true;
      requestAnimationFrame(centerOnLast);
    } else if (view === "game") requestAnimationFrame(drawBoard);
  }
}

function noteSounds(state) {
  const key = JSON.stringify(state.events || []);
  if (lastEventKey && key !== lastEventKey && state.events && state.events.length) {
    const last = state.events[state.events.length - 1];
    if (last.kind === "score") blip("score");
    else if (last.kind === "place") blip("place");
    else if (last.kind === "end") blip("end");
    else if (last.kind === "start") blip("turn");
  }
  const turnId = state.players[state.turn] && state.players[state.turn].id;
  if (lastTurn && turnId !== lastTurn && session && turnId === session.id && state.status !== "done" && state.status !== "lobby") {
    blip("turn");
  }
  lastEventKey = key;
  lastTurn = turnId || "";
}

function blip(kind) {
  if (!soundOn()) return;
  const ctx = audioCtx || (audioCtx = new (window.AudioContext || window.webkitAudioContext)());
  const preset = {
    place: [420, 0.07, 0.08],
    score: [620, 0.16, 0.08],
    turn: [300, 0.09, 0.08],
    end: [480, 0.28, 0.08],
    tick: [680, 0.11, 0.022],
  }[kind] || [400, 0.08, 0.08];
  if (ctx.state === "suspended") ctx.resume();
  const o = ctx.createOscillator();
  const g = ctx.createGain();
  o.type = kind === "tick" ? "sine" : "triangle";
  o.frequency.value = preset[0];
  g.gain.setValueAtTime(0.0001, ctx.currentTime);
  g.gain.exponentialRampToValueAtTime(preset[2], ctx.currentTime + 0.02);
  g.gain.exponentialRampToValueAtTime(0.0001, ctx.currentTime + preset[1]);
  o.connect(g);
  g.connect(ctx.destination);
  o.start();
  o.stop(ctx.currentTime + preset[1] + 0.02);
}

function renderLobby() {
  const state = table;
  document.getElementById("lobby-code").textContent = state.code;
  const host = session && state.hostId === session.id;
  const open = document.getElementById("set-open");
  if (open.checked !== state.open) open.checked = state.open;
  open.disabled = !host;
  document.getElementById("open-label").textContent = state.open ? t("open_game") : t("closed_game");
  document.getElementById("set-max").textContent = String(state.maxPlayers);
  document.getElementById("set-meeples").textContent = String(state.meepleCount);
  for (const id of ["max-dec", "max-inc", "meeple-dec", "meeple-inc"]) {
    document.getElementById(id).disabled = !host;
  }
  const list = document.getElementById("lobby-players");
  list.replaceChildren();
  for (const p of state.players) {
    const row = document.createElement("div");
    row.className = "player" + (p.connected ? "" : " off");
    const sw = document.createElement("i");
    sw.style.background = p.color;
    const name = document.createElement("span");
    const tags = [];
    if (p.id === state.hostId) tags.push(t("host"));
    if (!p.connected) tags.push(t("offline"));
    name.textContent = p.name + (tags.length ? " · " + tags.join(", ") : "");
    row.append(sw, name);
    if (host && p.id !== session.id) {
      const kick = document.createElement("button");
      kick.type = "button";
      kick.className = "ghost";
      kick.textContent = t("kick");
      kick.onclick = () => send({ type: "kick", playerId: p.id });
      row.append(kick);
    }
    list.append(row);
  }
  const hint = document.getElementById("lobby-hint");
  hint.textContent = state.players.length < 2 ? t("need_two") : "";
  const start = document.getElementById("start-btn");
  start.disabled = !host || state.players.length < 2;
}

function myTurn() {
  if (!table || !session) return false;
  const p = table.players[table.turn];
  return p && p.id === session.id;
}

function renderGame() {
  const state = table;
  const me = myTurn();
  const current = state.players[state.turn];
  document.getElementById("turn-line").textContent =
    state.status === "done" ? t("done") : me ? t("your_turn") : t("their_turn", { name: current ? current.name : "" });
  document.getElementById("phase-hint").textContent =
    state.status === "place" ? t("place_hint") : state.status === "meeple" ? t("meeple_hint") : "";
  const curOff = current && !current.connected && state.status !== "done";
  document.getElementById("afk-btn").hidden = !curOff || me;
  document.getElementById("pass-btn").hidden = state.status !== "meeple" || !me;
  document.getElementById("recall-btn").hidden = state.status !== "meeple" || !me;
  const adjusting = me && (state.status === "place" || state.status === "meeple");
  document.getElementById("rot-right").hidden = !adjusting;
  document.getElementById("preview").hidden = state.status !== "place" && state.status !== "meeple";
  document.getElementById("spots").replaceChildren();

  const players = document.getElementById("players");
  players.replaceChildren();
  for (const p of state.players) {
    const row = document.createElement("div");
    row.className = "player" + (current && p.id === current.id && state.status !== "done" ? " current" : "") + (p.connected ? "" : " off");
    const sw = document.createElement("i");
    sw.style.background = p.color;
    const name = document.createElement("span");
    const marks = [];
    if (p.skips) marks.push(t("skips", { n: p.skips }));
    if (!p.connected) marks.push(t("offline"));
    name.textContent = p.name + (marks.length ? " · " + marks.join(", ") : "");
    const score = document.createElement("b");
    score.textContent = String(p.score);
    row.append(sw, name, pips(p.meeples, state.meepleCount), score);
    row.style.color = p.color;
    name.style.color = "var(--ink)";
    score.style.color = "var(--ink)";
    players.append(row);
  }

  const log = document.getElementById("log");
  log.replaceChildren();
  for (const ev of state.events || []) {
    const li = document.createElement("li");
    li.textContent = eventText(ev);
    log.append(li);
  }
  log.scrollTop = log.scrollHeight;

  const done = document.getElementById("done-card");
  done.hidden = state.status !== "done";
  if (state.status === "done") {
    const winners = state.players.filter((p) => (state.winners || []).includes(p.id));
    const names = winners.map((p) => p.name).join(", ");
    document.getElementById("done-winner").textContent = winners.length > 1 ? t("tie", { name: names }) : t("winner", { name: names });
    const ol = document.getElementById("done-scores");
    ol.replaceChildren();
    const ranked = [...state.players].sort((a, b) => b.score - a.score);
    for (const p of ranked) {
      const li = document.createElement("li");
      li.textContent = p.name + " — " + p.score;
      ol.append(li);
    }
  }
  drawPreview();
  showClock(state);
}

function showClock(state) {
  clearInterval(clockTimer);
  const el = document.getElementById("clock");
  if (!el) return;
  if (!state || !state.deadline || (state.status !== "place" && state.status !== "meeple")) {
    el.hidden = true;
    return;
  }
  const end = Date.parse(state.deadline);
  const mine = state.players[state.turn] && session && state.players[state.turn].id === session.id;
  const markKey = String(state.deadline);
  if (clockMarkKey !== markKey) {
    clockMarkKey = markKey;
    clockPlayed = new Set();
  }
  const tick = () => {
    const left = Math.max(0, Math.ceil((end - Date.now()) / 1000));
    el.hidden = false;
    el.textContent = t("clock", { n: left });
    if (!mine || ![9, 6, 3].includes(left) || clockPlayed.has(left)) return;
    clockPlayed.add(left);
    blip("tick");
  };
  tick();
  clockTimer = setInterval(tick, 500);
}

function pips(n, total) {
  const wrap = document.createElement("span");
  wrap.className = "pips";
  for (let i = 0; i < total; i++) {
    const d = document.createElement("i");
    if (i < n) d.className = "on";
    wrap.append(d);
  }
  return wrap;
}

function eventText(ev) {
  if (ev.kind === "score") return t("ev_score", { name: ev.name, points: ev.points, feature: t(ev.feature) });
  if (ev.kind === "vote") return ev.n ? t("ev_vote", { name: ev.name }) : t("ev_unvote", { name: ev.name });
  const key = "ev_" + ev.kind;
  const text = t(key, ev);
  return text === key ? ev.kind : text;
}

function colorOf(id) {
  const p = (table.players || []).find((pl) => pl.id === id);
  return p ? p.color : "#c23b3b";
}

function meepleTile() {
  if (!table || table.status !== "meeple" || !table.tiles.length) return null;
  const tile = table.tiles[table.tiles.length - 1];
  if (!catalogById[tile.id]) return null;
  return { tile, view: rotateView(catalogById[tile.id], tile.rot) };
}

function drawPreview() {
  const canvas = document.getElementById("preview");
  if (!table) return;
  let viewTile = null;
  let spots = [];
  if (table.status === "place" && catalogById[table.currentId]) {
    viewTile = rotateView(catalogById[table.currentId], rot);
  } else if (table.status === "meeple") {
    const placed = meepleTile();
    if (!placed) return;
    viewTile = placed.view;
    spots = table.spots || [];
  }
  if (!viewTile) return;
  const useRot = table.status === "meeple" && table.tiles.length ? table.tiles[table.tiles.length - 1].rot : rot;
  const ctx = canvas.getContext("2d");
  ctx.clearRect(0, 0, canvas.width, canvas.height);
  drawTile(ctx, viewTile, canvas.width, {
    quality: quality(),
    images: tileImages,
    rot: useRot,
    spots: spots.map((sp) => ({
      ...sp,
      hot: hotSpot && hotSpot.where === "preview" && hotSpot.kind === sp.kind && hotSpot.index === sp.index,
    })),
  });
}

function spotUnder(x, y, hits) {
  let best = null;
  let bestD = Infinity;
  for (const hit of hits) {
    const d = Math.hypot(x - hit.x, y - hit.y);
    if (d <= hit.r && d < bestD) {
      best = hit;
      bestD = d;
    }
  }
  return best;
}

function pickMeepleSpot(localX, localY, pixelX, pixelY, view, spots, hits) {
  const city = cityGroupAt(view, localX, localY);
  if (city >= 0) {
    return (spots || []).find((sp) => sp.kind === "city" && sp.index === city && sp.free === true) || null;
  }
  const hit = spotUnder(pixelX, pixelY, hits);
  if (!hit || hit.kind === "city") return null;
  return hit;
}

function dpr() {
  const cap = { low: 1, med: 1.5, high: 2 }[quality()] || 1.5;
  return Math.min(window.devicePixelRatio || 1, cap);
}

function scheduleDraw() {
  if (drawQueued) return;
  drawQueued = true;
  requestAnimationFrame(() => {
    drawQueued = false;
    drawBoard();
  });
}

function drawBoard() {
  if (view !== "game" || !table) return;
  const canvas = document.getElementById("board");
  const rect = canvas.getBoundingClientRect();
  const ratio = dpr();
  const w = Math.max(1, rect.width);
  const h = Math.max(1, rect.height);
  const bw = Math.floor(w * ratio);
  const bh = Math.floor(h * ratio);
  if (canvas.width !== bw || canvas.height !== bh) {
    canvas.width = bw;
    canvas.height = bh;
  }
  const ctx = canvas.getContext("2d");
  ctx.setTransform(ratio, 0, 0, ratio, 0, 0);
  spotHits = [];
  paintCloth(ctx, w, h);
  ctx.save();
  ctx.translate(cam.x, cam.y);
  ctx.scale(cam.z, cam.z);
  const S = TILE;
  for (const tile of table.tiles || []) {
    const viewTile = rotateView(catalogById[tile.id], tile.rot);
    if (!viewTile) continue;
    ctx.save();
    ctx.translate(tile.x * S, -tile.y * S);
    if (quality() !== "low") {
      ctx.fillStyle = "rgba(0,0,0,.22)";
      ctx.fillRect(4, 5, S, S);
    }
    const meeple = tile.meeple ? { kind: tile.meeple.kind, index: tile.meeple.index } : null;
    const placing = table.status === "meeple" && myTurn() && tile === table.tiles[table.tiles.length - 1];
    const spots = placing ? (table.spots || []) : [];
    drawTile(ctx, viewTile, S, {
      quality: quality(),
      images: tileImages,
      rot: tile.rot,
      meeple,
      color: meeple ? colorOf(tile.meeple.playerId) : "",
      spots: spots.map((sp) => ({
        ...sp,
        hot: hotSpot && hotSpot.where === "board" && hotSpot.kind === sp.kind && hotSpot.index === sp.index,
      })),
    });
    if (placing) {
      for (const sp of spots) {
        if (sp.free !== true || sp.kind === "city") continue;
        const [ax, ay] = anchorOf(viewTile, sp.kind, sp.index);
        spotHits.push({
          kind: sp.kind,
          index: sp.index,
          x: cam.x + cam.z * (tile.x * S + ax * S),
          y: cam.y + cam.z * (-tile.y * S + ay * S),
          r: Math.max(18, cam.z * S * 0.16),
        });
      }
    }
    if (table.hasLast && tile.x === table.lastX && tile.y === table.lastY) {
      ctx.strokeStyle = "rgba(226, 184, 92, .95)";
      ctx.lineWidth = 3;
      ctx.strokeRect(2, 2, S - 4, S - 4);
    }
    ctx.restore();
  }
  if (table.status === "place" && myTurn()) {
    for (const move of table.legal || []) {
      ctx.save();
      ctx.translate(move.x * S, -move.y * S);
      const hot = hover && hover.x === move.x && hover.y === move.y;
      ctx.strokeStyle = hot ? "rgba(255, 220, 140, 1)" : "rgba(226, 184, 92, .75)";
      ctx.setLineDash([7, 5]);
      ctx.lineWidth = 2;
      ctx.strokeRect(3, 3, S - 6, S - 6);
      ctx.restore();
    }
    if (hover && catalogById[table.currentId]) {
      const move = (table.legal || []).find((m) => m.x === hover.x && m.y === hover.y);
      if (move) {
        const use = move.rots.includes(rot) ? rot : move.rots[0];
        ctx.save();
        ctx.translate(hover.x * S, -hover.y * S);
        drawTile(ctx, rotateView(catalogById[table.currentId], use), S, { ghost: true, quality: quality(), images: tileImages, rot: use });
        ctx.restore();
      }
    }
  }
  ctx.restore();
  const shade = ctx.createRadialGradient(w * 0.5, h * 0.45, Math.min(w, h) * 0.25, w * 0.5, h * 0.5, Math.max(w, h) * 0.78);
  shade.addColorStop(0, "rgba(0,0,0,0)");
  shade.addColorStop(0.78, "rgba(0,0,0,0)");
  shade.addColorStop(1, "rgba(24, 14, 8, .28)");
  ctx.fillStyle = shade;
  ctx.fillRect(0, 0, w, h);
}

const tableCloth = new Image();
tableCloth.src = "/img/table.png";
tableCloth.onload = () => {
  if (view === "game") scheduleDraw();
};

let tablePattern = null;

function paintCloth(ctx, w, h) {
  if (!tableCloth.complete || tableCloth.naturalWidth === 0) {
    ctx.fillStyle = "#6a4b32";
    ctx.fillRect(0, 0, w, h);
    return;
  }
  if (!tablePattern) tablePattern = ctx.createPattern(tableCloth, "repeat");
  const z = cam.z || 1;
  // One repeat stays near the source resolution even at maximum zoom.
  const unit = 240 / tableCloth.naturalWidth;
  tablePattern.setTransform(new DOMMatrix().scale(unit, unit));
  const x0 = -cam.x / z;
  const y0 = -cam.y / z;
  ctx.save();
  ctx.translate(cam.x, cam.y);
  ctx.scale(z, z);
  ctx.fillStyle = tablePattern;
  ctx.fillRect(x0, y0, w / z, h / z);
  ctx.restore();
}

function centerOnLast() {
  const canvas = document.getElementById("board");
  const rect = canvas.getBoundingClientRect();
  const x = table && table.hasLast ? table.lastX : 0;
  const y = table && table.hasLast ? table.lastY : 0;
  const cx = x * TILE + TILE / 2;
  const cy = -y * TILE + TILE / 2;
  cam.z = 1;
  cam.x = rect.width / 2 - cx;
  cam.y = rect.height / 2 - cy;
  drawBoard();
}

function tileFromEvent(ev) {
  const canvas = document.getElementById("board");
  const rect = canvas.getBoundingClientRect();
  const sx = ev.clientX - rect.left;
  const sy = ev.clientY - rect.top;
  const wx = (sx - cam.x) / cam.z;
  const wy = (sy - cam.y) / cam.z;
  return { x: Math.floor(wx / TILE), y: -Math.floor(wy / TILE) };
}

function onBoardClick(ev) {
  if (!table || !myTurn()) return;
  if (table.status === "meeple") {
    const placed = meepleTile();
    if (!placed) return;
    const boardEl = document.getElementById("board");
    const rect = boardEl.getBoundingClientRect();
    const sx = ev.clientX - rect.left;
    const sy = ev.clientY - rect.top;
    const wx = (sx - cam.x) / cam.z;
    const wy = (sy - cam.y) / cam.z;
    const localX = (wx - placed.tile.x * TILE) / TILE;
    const localY = (wy + placed.tile.y * TILE) / TILE;
    const hit = pickMeepleSpot(localX, localY, sx, sy, placed.view, table.spots, spotHits);
    if (hit) send({ type: "meeple", kind: hit.kind, index: hit.index });
    return;
  }
  if (table.status !== "place") return;
  const cell = tileFromEvent(ev);
  const move = (table.legal || []).find((m) => m.x === cell.x && m.y === cell.y);
  if (!move) return;
  const use = move.rots.includes(rot) ? rot : move.rots[0];
  send({ type: "place", x: cell.x, y: cell.y, rot: use });
}

async function loadBrowse() {
  if (view !== "browse") return;
  try {
    if (!(await ensureSession())) return;
    const games = await api("/api/games");
    const list = document.getElementById("browse-list");
    list.replaceChildren();
    document.getElementById("browse-empty").hidden = games.length > 0;
    for (const g of games) {
      const row = document.createElement("div");
      row.className = "game-row";
      const text = document.createElement("span");
      text.textContent = (g.host || "") + " · " + (g.names || []).join(", ");
      const meta = document.createElement("small");
      const live = g.status === "place" || g.status === "meeple" ? " · " + t("playing") : "";
      meta.textContent = t("seats", { n: g.players, max: g.maxPlayers }) + live;
      const btn = document.createElement("button");
      btn.type = "button";
      btn.className = "seal";
      btn.textContent = t("sit");
      btn.onclick = () => joinGame({ id: g.id });
      row.append(text, meta, btn);
      list.append(row);
    }
  } catch (err) {
    fail(err);
  }
}

async function joinGame(body) {
  try {
    if (!(await ensureSession())) return;
    const g = await api("/api/games/join", { method: "POST", body });
    connect(g.id);
  } catch (err) {
    fail(err);
  }
}

async function loadRanks() {
  try {
    if (!(await ensureSession())) return;
    const rows = await api("/api/leaderboard");
    const list = document.getElementById("ranks-list");
    list.replaceChildren();
    document.getElementById("ranks-empty").hidden = rows.length > 0;
    rows.forEach((row, i) => {
      const line = document.createElement("div");
      line.className = "player";
      const place = document.createElement("b");
      place.textContent = String(i + 1);
      const name = document.createElement("span");
      name.textContent = row.name;
      const score = document.createElement("b");
      score.textContent = String(row.score);
      line.append(place, name, score);
      list.append(line);
    });
  } catch (err) {
    fail(err);
  }
}

async function leaveTable() {
  if (!table || leaving) return;
  leaving = true;
  let banked = 0;
  try {
    const res = await api("/api/games/" + table.id + "/leave", { method: "POST", body: {} });
    banked = res.score || 0;
  } catch (err) {
    leaving = false;
    fail(err);
    return;
  }
  prefs.set("game", "");
  table = null;
  camReady = false;
  lastEventKey = "";
  if (socket) {
    socket.onclose = null;
    socket.close();
    socket = null;
  }
  leaving = false;
  show("menu");
  if (banked > 0) toast(t("banked", { n: banked }));
}

function paintSettings() {
  document.getElementById("sound-btn").textContent = soundOn() ? t("on") : t("off");
  for (const btn of document.querySelectorAll("[data-lang]")) {
    btn.setAttribute("aria-pressed", btn.dataset.lang === lang ? "true" : "false");
  }
  for (const btn of document.querySelectorAll("[data-quality]")) {
    btn.setAttribute("aria-pressed", btn.dataset.quality === quality() ? "true" : "false");
  }
}

function drawMenuArt() {
  const canvas = document.getElementById("menu-art");
  if (!canvas || !catalogById.city_road) return;
  const ctx = canvas.getContext("2d");
  const w = canvas.width;
  const h = canvas.height;
  paintCloth(ctx, w, h);
  const S = 148;
  const pieces = [
    { id: "city_road", rot: 0, x: 0, y: 0 },
    { id: "city2_s", rot: 2, x: 0, y: 1, meeple: { kind: "city", index: 0 }, color: "#c23b3b" },
    { id: "road", rot: 1, x: 1, y: 0 },
    { id: "bend", rot: 3, x: 2, y: 0 },
    { id: "road", rot: 0, x: 2, y: 1 },
    { id: "mon", rot: 0, x: 0, y: -1, meeple: { kind: "monastery", index: 0 }, color: "#2c6bed" },
    { id: "mon_road", rot: 3, x: -1, y: 0 },
    { id: "city1", rot: 2, x: -1, y: -1, meeple: { kind: "city", index: 0 }, color: "#e0a825" },
  ];
  const minX = -1;
  const maxY = 1;
  const ox = (w - 4 * S) / 2;
  const oy = (h - 4 * S) / 2;
  for (const piece of pieces) {
    ctx.save();
    ctx.translate(ox + (piece.x - minX) * S, oy + (maxY - piece.y) * S);
    ctx.fillStyle = "rgba(0,0,0,.2)";
    ctx.fillRect(5, 6, S, S);
    drawTile(ctx, rotateView(catalogById[piece.id], piece.rot), S, {
      quality: "high",
      images: tileImages,
      rot: piece.rot,
      meeple: piece.meeple,
      color: piece.color,
    });
    ctx.restore();
  }
}

function drawGallery() {
  const root = document.getElementById("tile-gallery");
  root.replaceChildren();
  for (const def of Object.values(catalogById)) {
    const fig = document.createElement("figure");
    const canvas = document.createElement("canvas");
    canvas.width = 160;
    canvas.height = 160;
    drawTile(canvas.getContext("2d"), rotateView(def, 0), 160, { quality: "high", images: tileImages, rot: 0 });
    const cap = document.createElement("figcaption");
    cap.textContent = def.id + " ×" + def.weight;
    fig.append(canvas, cap);
    root.append(fig);
  }
}

function bump(field, delta) {
  if (!table || !session || table.hostId !== session.id) return;
  const maxPlayers = field === "max" ? table.maxPlayers + delta : table.maxPlayers;
  const meeples = field === "meeples" ? table.meepleCount + delta : table.meepleCount;
  send({ type: "settings", open: table.open, maxPlayers, meeples });
}

document.getElementById("menu-form").addEventListener("submit", async (ev) => {
  ev.preventDefault();
  try {
    if (!(await ensureSession())) return;
    const g = await api("/api/games", { method: "POST", body: { open: true, maxPlayers: 5, meeples: 7 } });
    lastEventKey = "";
    camReady = false;
    connect(g.id);
  } catch (err) {
    fail(err);
  }
});
let rulesFrom = "menu";
document.getElementById("go-browse").onclick = () => show("browse");
document.getElementById("go-rules").onclick = () => {
  rulesFrom = "menu";
  show("rules");
};
document.getElementById("game-rules").onclick = () => {
  rulesFrom = "game";
  show("rules");
};
document.getElementById("rules-back").onclick = () => show(rulesFrom);
document.getElementById("go-ranks").onclick = () => show("ranks");
document.getElementById("go-register").onclick = () => show("register");
document.getElementById("go-login").onclick = () => show("login");
document.getElementById("go-logout").onclick = () => {
  session = null;
  prefs.set("token", "");
  prefs.set("game", "");
  paintWho();
  paintAccount();
  show("menu");
};
document.getElementById("register-form").addEventListener("submit", async (ev) => {
  ev.preventDefault();
  const password = document.getElementById("reg-pass").value;
  if (password !== document.getElementById("reg-pass2").value) {
    toast(t("err_password_mismatch"));
    return;
  }
  try {
    const me = await api("/api/register", {
      method: "POST",
      body: {
        name: document.getElementById("reg-name").value.trim(),
        login: document.getElementById("reg-login").value.trim(),
        password,
      },
    });
    document.getElementById("reg-pass").value = "";
    document.getElementById("reg-pass2").value = "";
    if (!adoptSession(me)) show("menu");
  } catch (err) {
    fail(err);
  }
});
document.getElementById("login-form").addEventListener("submit", async (ev) => {
  ev.preventDefault();
  try {
    const me = await api("/api/login", {
      method: "POST",
      body: {
        login: document.getElementById("login-name").value.trim(),
        password: document.getElementById("login-pass").value,
      },
    });
    document.getElementById("login-pass").value = "";
    if (!adoptSession(me)) show("menu");
  } catch (err) {
    fail(err);
  }
});
document.getElementById("go-resume").onclick = () => {
  const id = prefs.get("game", "");
  if (id) connect(id);
};
document.getElementById("join-form").addEventListener("submit", (ev) => {
  ev.preventDefault();
  joinGame({ code: document.getElementById("code").value });
});
for (const btn of document.querySelectorAll("[data-back]")) btn.onclick = () => show("menu");
document.getElementById("settings-btn").onclick = () => {
  settingsReturn = view === "settings" ? "menu" : view;
  show("settings");
};
document.getElementById("settings-back").onclick = () => {
  const back = settingsReturn === "settings" ? "menu" : (settingsReturn || "menu");
  show(back);
  if (view === "lobby" && table) renderLobby();
  if (view === "game" && table) {
    renderGame();
    if (!camReady) {
      camReady = true;
      requestAnimationFrame(centerOnLast);
    }
  }
};
document.getElementById("sound-btn").onclick = () => {
  prefs.set("sound", soundOn() ? "0" : "1");
  paintSettings();
  blip("place");
};
for (const btn of document.querySelectorAll("[data-lang]")) {
  btn.onclick = () => {
    lang = btn.dataset.lang;
    prefs.set("lang", lang);
    localStorage.setItem("carca_lang", lang);
    applyI18n();
    paintSettings();
    if (table && table.status === "lobby") renderLobby();
    else if (table) renderGame();
    else if (view === "menu") show("menu");
  };
}
for (const btn of document.querySelectorAll("[data-quality]")) {
  btn.onclick = () => {
    prefs.set("quality", btn.dataset.quality);
    paintSettings();
    if (view === "game") drawBoard();
    if (view === "menu") drawMenuArt();
  };
}
document.getElementById("copy-code").onclick = async () => {
  if (!table) return;
  try {
    await navigator.clipboard.writeText(table.code);
  } catch {
    const input = document.createElement("input");
    input.value = table.code;
    document.body.append(input);
    input.select();
    document.execCommand("copy");
    input.remove();
  }
  toast(t("copied"));
};
document.getElementById("set-open").onchange = (ev) => {
  if (!table) return;
  send({ type: "settings", open: ev.target.checked, maxPlayers: table.maxPlayers, meeples: table.meepleCount });
};
document.getElementById("max-dec").onclick = () => bump("max", -1);
document.getElementById("max-inc").onclick = () => bump("max", 1);
document.getElementById("meeple-dec").onclick = () => bump("meeples", -1);
document.getElementById("meeple-inc").onclick = () => bump("meeples", 1);
document.getElementById("start-btn").onclick = () => send({ type: "start" });
document.getElementById("lobby-leave").onclick = () => leaveTable();
document.getElementById("game-leave").onclick = () => leaveTable();
document.getElementById("done-menu").onclick = () => leaveTable();
document.getElementById("pass-btn").onclick = () => send({ type: "pass" });
document.getElementById("recall-btn").onclick = () => send({ type: "recall" });
function spin(dir) {
  if (!table || !myTurn()) return;
  if (table.status === "meeple") {
    send({ type: "turn", rot: dir });
    return;
  }
  if (table.status !== "place") return;
  rot = (rot + (dir > 0 ? 1 : 3)) % 4;
  drawPreview();
  scheduleDraw();
}
document.getElementById("afk-btn").onclick = () => send({ type: "skip_afk" });
document.getElementById("center-btn").onclick = () => centerOnLast();
document.getElementById("rot-right").onclick = () => spin(1);

const board = document.getElementById("board");
const pointers = new Map();
let pinch = null;

function zoomAt(sx, sy, nextZ) {
  const wx = (sx - cam.x) / cam.z;
  const wy = (sy - cam.y) / cam.z;
  cam.z = Math.min(2.4, Math.max(0.35, nextZ));
  cam.x = sx - wx * cam.z;
  cam.y = sy - wy * cam.z;
}

function movePinch() {
  if (pointers.size < 2) return false;
  const [a, b] = [...pointers.values()];
  const dist = Math.hypot(a.x - b.x, a.y - b.y) || 1;
  const midX = (a.x + b.x) / 2;
  const midY = (a.y + b.y) / 2;
  const rect = board.getBoundingClientRect();
  if (!pinch) {
    pinch = { dist, z: cam.z };
  } else {
    zoomAt(midX - rect.left, midY - rect.top, pinch.z * (dist / pinch.dist));
    scheduleDraw();
  }
  if (drag) drag.moved = true;
  return true;
}

board.addEventListener("pointerdown", (ev) => {
  pointers.set(ev.pointerId, { x: ev.clientX, y: ev.clientY });
  if (pointers.size === 1) {
    drag = { x: ev.clientX, y: ev.clientY, camx: cam.x, camy: cam.y, moved: false };
    board.classList.add("grabbing");
  } else if (drag) {
    drag.moved = true;
    pinch = null;
  }
  board.setPointerCapture(ev.pointerId);
});
function sameHot(a, b) {
  return !!a === !!b && (!a || (a.where === b.where && a.kind === b.kind && a.index === b.index));
}

board.addEventListener("pointermove", (ev) => {
  if (pointers.has(ev.pointerId)) pointers.set(ev.pointerId, { x: ev.clientX, y: ev.clientY });
  if (pointers.size >= 2) {
    movePinch();
    return;
  }
  if (!drag) {
    hover = tileFromEvent(ev);
    const rect = board.getBoundingClientRect();
    const sx = ev.clientX - rect.left;
    const sy = ev.clientY - rect.top;
    const placed = meepleTile();
    let hit = null;
    if (placed) {
      const wx = (sx - cam.x) / cam.z;
      const wy = (sy - cam.y) / cam.z;
      hit = pickMeepleSpot(
        (wx - placed.tile.x * TILE) / TILE,
        (wy + placed.tile.y * TILE) / TILE,
        sx,
        sy,
        placed.view,
        table.spots,
        spotHits,
      );
    }
    const next = hit ? { where: "board", kind: hit.kind, index: hit.index } : (hotSpot && hotSpot.where === "preview" ? hotSpot : null);
    if (!sameHot(hotSpot, next)) {
      hotSpot = next;
      drawPreview();
    }
    board.style.cursor = hit ? "pointer" : "";
    scheduleDraw();
    return;
  }
  if (Math.hypot(ev.clientX - drag.x, ev.clientY - drag.y) > 4) drag.moved = true;
  if (drag.moved) {
    cam.x = drag.camx + (ev.clientX - drag.x);
    cam.y = drag.camy + (ev.clientY - drag.y);
    scheduleDraw();
  } else {
    hover = tileFromEvent(ev);
  }
});
function endPointer(ev) {
  pointers.delete(ev.pointerId);
  if (pointers.size < 2) pinch = null;
  if (pointers.size > 0) return;
  board.classList.remove("grabbing");
  const was = drag;
  drag = null;
  if (was && !was.moved) onBoardClick(ev);
}
board.addEventListener("pointerup", endPointer);
board.addEventListener("pointercancel", endPointer);
board.addEventListener("pointerleave", () => {
  hover = null;
  if (hotSpot && hotSpot.where === "board") hotSpot = null;
  board.style.cursor = "";
  scheduleDraw();
  drawPreview();
});

const preview = document.getElementById("preview");
preview.addEventListener("pointermove", (ev) => {
  if (!table || table.status !== "meeple" || !myTurn()) return;
  const placed = meepleTile();
  if (!placed) return;
  const rect = preview.getBoundingClientRect();
  const x = ((ev.clientX - rect.left) / rect.width) * preview.width;
  const y = ((ev.clientY - rect.top) / rect.height) * preview.height;
  const S = preview.width;
  const hits = (table.spots || []).filter((sp) => sp.free === true && sp.kind !== "city").map((sp) => {
    const [ax, ay] = anchorOf(placed.view, sp.kind, sp.index);
    return { kind: sp.kind, index: sp.index, x: ax * S, y: ay * S, r: S * 0.16 };
  });
  const hit = pickMeepleSpot(x / S, y / S, x, y, placed.view, table.spots, hits);
  const next = hit ? { where: "preview", kind: hit.kind, index: hit.index } : (hotSpot && hotSpot.where === "board" ? hotSpot : null);
  preview.style.cursor = hit ? "pointer" : "";
  if (!sameHot(hotSpot, next)) {
    hotSpot = next;
    drawPreview();
    scheduleDraw();
  }
});
preview.addEventListener("pointerleave", () => {
  if (hotSpot && hotSpot.where === "preview") {
    hotSpot = null;
    drawPreview();
    scheduleDraw();
  }
  preview.style.cursor = "";
});
preview.addEventListener("click", (ev) => {
  if (!table || table.status !== "meeple" || !myTurn()) return;
  const placed = meepleTile();
  if (!placed) return;
  const rect = preview.getBoundingClientRect();
  const x = ((ev.clientX - rect.left) / rect.width) * preview.width;
  const y = ((ev.clientY - rect.top) / rect.height) * preview.height;
  const S = preview.width;
  const hits = (table.spots || []).filter((sp) => sp.free === true && sp.kind !== "city").map((sp) => {
    const [ax, ay] = anchorOf(placed.view, sp.kind, sp.index);
    return { kind: sp.kind, index: sp.index, x: ax * S, y: ay * S, r: S * 0.16 };
  });
  const hit = pickMeepleSpot(x / S, y / S, x, y, placed.view, table.spots, hits);
  if (hit) send({ type: "meeple", kind: hit.kind, index: hit.index });
});
board.addEventListener("wheel", (ev) => {
  ev.preventDefault();
  const rect = board.getBoundingClientRect();
  const sx = ev.clientX - rect.left;
  const sy = ev.clientY - rect.top;
  const factor = ev.deltaY < 0 ? 1.08 : 0.92;
  zoomAt(sx, sy, cam.z * factor);
  scheduleDraw();
}, { passive: false });

window.addEventListener("keydown", (ev) => {
  if (view !== "game" || ev.target.matches("input, textarea")) return;
  if (ev.key === "r" || ev.key === "R" || ev.key === "ArrowRight") {
    spin(1);
  } else if (ev.key === "q" || ev.key === "Q" || ev.key === "ArrowLeft") {
    spin(-1);
  } else if ((ev.key === "Escape" || ev.key === "Backspace") && table && table.status === "meeple" && myTurn()) {
    send({ type: "recall" });
  } else if (ev.key === "Enter" && table && table.status === "meeple" && myTurn()) {
    send({ type: "pass" });
  }
});
window.addEventListener("resize", () => {
  if (view === "game") scheduleDraw();
});

boot().catch((err) => {
  console.error(err);
  toast(t("err_bad_request"));
});
