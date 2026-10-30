const TILE = 96;

function rotateView(def, rot) {
  rot = ((rot % 4) + 4) % 4;
  const edges = new Array(4);
  for (let i = 0; i < 4; i++) edges[(i + rot) % 4] = def.edges[i];
  const turn = (groups) => (groups || []).map((g) => g.map((v) => (v + rot) % 4));
  return {
    id: def.id,
    edges,
    cities: turn(def.cities),
    roads: turn(def.roads),
    fields: turn(def.fields),
    monastery: !!def.monastery,
    shield: !!def.shield,
  };
}

// Painted tiles from resources/tiles. File numbers are clockwise turns.
// offset: file index of catalog rotation 0.
// count 1 + spin: one picture, turned with the tile.
// pair: two pictures; rotations 2 and 3 are those pictures turned 180°.
const TILE_ART = {
  mon: { stem: "Monastery", offset: 0, count: 1, spin: true },
  mon_road: { stem: "MonasteryRoad", offset: 0, count: 4 },
  city4: { stem: "CastleCenter", offset: 0, count: 1, spin: true },
  city_road: { stem: "CastleWallRoad", offset: 0, count: 4 },
  city1: { stem: "CastleWall", offset: 0, count: 4 },
  bridge_s: { stem: "CastleTube", offset: 1, count: 4 },
  bridge: { stem: "CastleTube", offset: 1, count: 4 },
  opp: { stem: "CastleSides", offset: 0, count: 2, pair: true },
  corner: { stem: "CastleSidesEdge", offset: 1, count: 4 },
  bend_r: { stem: "CastleWallCurveRight", offset: 0, count: 4 },
  bend_l: { stem: "CastleWallCurveLeft", offset: 0, count: 4 },
  tee_city: { stem: "CastleWallJunction", offset: 0, count: 4 },
  city2_s: { stem: "CastleEdge", offset: 0, count: 4 },
  city2: { stem: "CastleEdge", offset: 0, count: 4 },
  city2_road_s: { stem: "CastleEdgeRoad", offset: 3, count: 4 },
  city2_road: { stem: "CastleEdgeRoad", offset: 3, count: 4 },
  city3_s: { stem: "CastleCenterSide", offset: 0, count: 4 },
  city3: { stem: "CastleCenterSide", offset: 0, count: 4 },
  city3_road_s: { stem: "CastleCenterEntry", offset: 0, count: 4 },
  city3_road: { stem: "CastleCenterEntry", offset: 0, count: 4 },
  road: { stem: "Road", offset: 0, count: 4 },
  bend: { stem: "RoadCurve", offset: 2, count: 4 },
  tee: { stem: "RoadJunctionSmall", offset: 0, count: 4 },
  cross: { stem: "RoadJunctionLarge", offset: 0, count: 1, spin: true },
};

const MEEPLE_ART = {
  road: ["meeple_road_template.png", "meeple_road.png"],
  city: ["meeple_castle_template.png", "meeple_castle.png"],
  field: ["meeple_fields_template.png", "meeple_fields.png"],
  monastery: ["meeple_monastery_template.png", "meeple_monastery.png"],
};

const tileImages = {};
const tileImageJobs = {};
const meepleArt = {};
const tintCache = new Map();

function artPlacement(id, rot) {
  const spec = TILE_ART[id];
  if (!spec) return null;
  const r = ((rot % 4) + 4) % 4;
  if (spec.pair) {
    return { file: spec.stem + (r % spec.count), twist: r >= 2 ? 2 : 0 };
  }
  if (spec.spin) return { file: spec.stem + "0", twist: r };
  const file = (r + (spec.offset || 0)) % spec.count;
  return { file: spec.stem + file, twist: 0 };
}

function noteArtLoaded() {
  if (typeof scheduleDraw === "function") scheduleDraw();
  if (typeof drawPreview === "function" && typeof table !== "undefined" && table) drawPreview();
  const menu = document.getElementById("view-menu");
  if (menu && !menu.hidden && typeof drawMenuArt === "function") drawMenuArt();
  const tiles = document.getElementById("view-tiles");
  if (tiles && !tiles.hidden && typeof drawGallery === "function") drawGallery();
}

function loadImage(src, done) {
  const img = new Image();
  img.onload = () => done(img);
  img.src = src;
  return img;
}

function loadTileImages(ids) {
  const names = new Set();
  for (const id of ids) {
    const spec = TILE_ART[id];
    if (!spec) continue;
    const n = spec.spin ? 1 : spec.count;
    for (let i = 0; i < n; i++) names.add(spec.stem + i);
  }
  for (const name of names) {
    if (name in tileImages || name in tileImageJobs) continue;
    tileImageJobs[name] = true;
    loadImage("/img/tiles/" + name + ".png", (img) => {
      tileImages[name] = img;
      noteArtLoaded();
    });
  }
  for (const [kind, files] of Object.entries(MEEPLE_ART)) {
    if (meepleArt[kind]) continue;
    meepleArt[kind] = {};
    loadImage("/img/meeple/" + files[0], (img) => {
      meepleArt[kind].fill = img;
      noteArtLoaded();
    });
    loadImage("/img/meeple/" + files[1], (img) => {
      meepleArt[kind].line = img;
      noteArtLoaded();
    });
  }
}

function edgePoint(edge) {
  return [
    [0.5, 0],
    [1, 0.5],
    [0.5, 1],
    [0, 0.5],
  ][edge];
}

function pointInCityGroup(group, x, y) {
  const set = new Set(group);
  const m = 0.15;
  const d = 0.33;
  for (const edge of group) {
    if (edge === 0 && y <= d && x >= (set.has(3) ? 0 : m) && x <= (set.has(1) ? 1 : 1 - m)) return true;
    if (edge === 1 && x >= 1 - d && y >= (set.has(0) ? 0 : m) && y <= (set.has(2) ? 1 : 1 - m)) return true;
    if (edge === 2 && y >= 1 - d && x >= (set.has(3) ? 0 : m) && x <= (set.has(1) ? 1 : 1 - m)) return true;
    if (edge === 3 && x <= d && y >= (set.has(0) ? 0 : m) && y <= (set.has(2) ? 1 : 1 - m)) return true;
  }
  if (group.length >= 3 && x >= m && x <= 1 - m && y >= m && y <= 1 - m) return true;
  if (set.has(0) && set.has(2) && x >= m && x <= 1 - m && y >= d && y <= 1 - d) return true;
  if (set.has(1) && set.has(3) && y >= m && y <= 1 - m && x >= d && x <= 1 - d) return true;
  return false;
}

function cityGroupAt(view, x, y) {
  const cities = view.cities || [];
  for (let i = 0; i < cities.length; i++) {
    if (pointInCityGroup(cities[i], x, y)) return i;
  }
  return -1;
}

const FIELD_SEEDS = [
  [[0.78, 0.34], [0.62, 0.28], [0.9, 0.1], [0.84, 0.2]],
  [[0.78, 0.66], [0.62, 0.72], [0.9, 0.9], [0.84, 0.8]],
  [[0.22, 0.66], [0.38, 0.72], [0.1, 0.9], [0.16, 0.8]],
  [[0.22, 0.34], [0.38, 0.28], [0.1, 0.1], [0.16, 0.2]],
];
const FIELD_SIDES = [
  [[0, 1], [0.9, 0.5]],
  [[1, 2], [0.5, 0.9]],
  [[2, 3], [0.1, 0.5]],
  [[3, 0], [0.5, 0.1]],
];

function dedupePoints(pts) {
  const out = [];
  for (const p of pts) {
    const x = Math.min(0.92, Math.max(0.08, p[0]));
    const y = Math.min(0.92, Math.max(0.08, p[1]));
    if (out.some((q) => Math.hypot(q[0] - x, q[1] - y) < 0.02)) continue;
    out.push([x, y]);
  }
  return out.length ? out : [[0.5, 0.5]];
}

function spotOptions(view, kind, index) {
  if (kind === "monastery") return [[0.5, 0.4], [0.46, 0.48]];
  if (kind === "city") {
    const g = view.cities[index] || [];
    let x = 0;
    let y = 0;
    for (const e of g) {
      const m = edgePoint(e);
      x += 0.5 + (m[0] - 0.5) * 0.62;
      y += 0.5 + (m[1] - 0.5) * 0.62;
    }
    const c = [x / g.length, y / g.length];
    const pts = [c];
    if (g.length === 1) {
      if (g[0] === 0 || g[0] === 2) pts.push([0.3, c[1]], [0.7, c[1]]);
      else pts.push([c[0], 0.3], [c[0], 0.7]);
    } else if (g.length === 2) {
      for (const [dx, dy] of [[0.12, 0], [-0.12, 0], [0, 0.12], [0, -0.12]]) pts.push([c[0] + dx, c[1] + dy]);
    }
    return dedupePoints(pts);
  }
  if (kind === "road") {
    const g = view.roads[index] || [];
    if (g.length === 1) {
      const m = edgePoint(g[0]);
      return [0.72, 0.56, 0.88].map((t) => [0.5 + (m[0] - 0.5) * t, 0.5 + (m[1] - 0.5) * t]);
    }
    if (g.length === 2) {
      const a = edgePoint(g[0]);
      const b = edgePoint(g[1]);
      const mid = [(a[0] + b[0]) / 2, (a[1] + b[1]) / 2];
      const bend = Math.abs(a[0] - b[0]) > 0.4 && Math.abs(a[1] - b[1]) > 0.4;
      if (!bend) {
        if (Math.abs(a[0] - b[0]) < 0.2) return [[0.5, 0.5], [0.5, 0.36], [0.5, 0.64]];
        return [[0.5, 0.5], [0.36, 0.5], [0.64, 0.5]];
      }
      return dedupePoints([
        [0.5 + (mid[0] - 0.5) * 0.5, 0.5 + (mid[1] - 0.5) * 0.5],
        mid,
      ]);
    }
    return [[0.5, 0.5]];
  }
  const g = view.fields[index] || [];
  const alive = [];
  const pocket = [];
  for (const c of g) {
    const got = FIELD_SEEDS[c].filter((p) => cityGroupAt(view, p[0], p[1]) < 0);
    if (got.length) {
      alive.push(c);
      pocket.push(got);
    }
  }
  const pts = [];
  if (alive.length >= 2) {
    const set = new Set(alive);
    for (const [[a, b], mid] of FIELD_SIDES) {
      if (set.has(a) && set.has(b) && cityGroupAt(view, mid[0], mid[1]) < 0) pts.push(mid);
    }
  }
  for (const got of pocket) pts.push(...got);
  let use = pts;
  if (view.monastery) {
    use = [...pts].sort((p, q) => q[1] - p[1]);
    const far = use.filter((p) => Math.hypot(p[0] - 0.5, p[1] - 0.4) > 0.32);
    if (far.length) use = far;
  }
  return dedupePoints(use);
}

function layoutSpots(view) {
  const items = [];
  (view.cities || []).forEach((_, i) => items.push(["city", i, spotOptions(view, "city", i)]));
  (view.roads || []).forEach((_, i) => items.push(["road", i, spotOptions(view, "road", i)]));
  (view.fields || []).forEach((_, i) => items.push(["field", i, spotOptions(view, "field", i)]));
  if (view.monastery) items.push(["monastery", 0, spotOptions(view, "monastery", 0)]);
  let best = null;
  let bestKey = null;
  const chosen = [];
  const indexes = [];
  const walk = (n) => {
    if (n === items.length) {
      let md = 9;
      for (let i = 0; i < chosen.length; i++) {
        for (let j = i + 1; j < chosen.length; j++) {
          const d = Math.hypot(chosen[i][0] - chosen[j][0], chosen[i][1] - chosen[j][1]);
          if (d < md) md = d;
        }
      }
      const key = (md >= 0.36 ? 1 : 0) * 1000 - indexes.reduce((s, v) => s + v, 0) + md / 100;
      if (bestKey === null || key > bestKey) {
        bestKey = key;
        best = chosen.map((p) => p.slice());
      }
      return;
    }
    items[n][2].forEach((p, i) => {
      chosen.push(p);
      indexes.push(i);
      walk(n + 1);
      chosen.pop();
      indexes.pop();
    });
  };
  if (items.length) walk(0);
  const map = {};
  items.forEach((it, i) => {
    map[it[0] + ":" + it[1]] = best ? best[i] : [0.5, 0.5];
  });
  return map;
}

function anchorOf(view, kind, index) {
  if (!view._spots) view._spots = layoutSpots(view);
  return view._spots[kind + ":" + index] || [0.5, 0.5];
}

function roadControl(a, b, S) {
  const midX = (a[0] + b[0]) / 2;
  const midY = (a[1] + b[1]) / 2;
  const adjacent = Math.abs(a[0] - b[0]) > S * 0.2 && Math.abs(a[1] - b[1]) > S * 0.2;
  if (!adjacent) return [S / 2, S / 2];
  return [midX + (midX - S / 2) * 0.85, midY + (midY - S / 2) * 0.85];
}

function traceRoad(ctx, edges, S) {
  const pts = edges.map((e) => edgePoint(e).map((v) => v * S));
  ctx.beginPath();
  if (edges.length === 1) {
    const m = pts[0];
    ctx.moveTo(m[0], m[1]);
    ctx.lineTo(m[0] + (S / 2 - m[0]) * 0.46, m[1] + (S / 2 - m[1]) * 0.46);
    return;
  }
  if (edges.length === 2) {
    const [cx, cy] = roadControl(pts[0], pts[1], S);
    ctx.moveTo(pts[0][0], pts[0][1]);
    ctx.quadraticCurveTo(cx, cy, pts[1][0], pts[1][1]);
    return;
  }
  for (const p of pts) {
    ctx.moveTo(p[0], p[1]);
    ctx.lineTo(S / 2, S / 2);
  }
}

function drawRoad(ctx, edges, S) {
  ctx.lineCap = "round";
  ctx.lineJoin = "round";
  ctx.strokeStyle = "#6a4930";
  ctx.lineWidth = S * 0.2;
  traceRoad(ctx, edges, S);
  ctx.stroke();
  ctx.strokeStyle = "#f0ddb0";
  ctx.lineWidth = S * 0.12;
  traceRoad(ctx, edges, S);
  ctx.stroke();
  ctx.strokeStyle = "rgba(92, 61, 36, .45)";
  ctx.lineWidth = Math.max(1, S * 0.012);
  ctx.setLineDash([S * 0.05, S * 0.045]);
  traceRoad(ctx, edges, S);
  ctx.stroke();
  ctx.setLineDash([]);
  if (edges.length >= 3) {
    ctx.beginPath();
    ctx.fillStyle = "#e0c48a";
    ctx.strokeStyle = "#5c3d24";
    ctx.lineWidth = S * 0.02;
    ctx.arc(S / 2, S / 2, S * 0.075, 0, Math.PI * 2);
    ctx.fill();
    ctx.stroke();
  }
}

function cityRect(edge, S) {
  const margin = S * 0.15;
  const depth = S * 0.33;
  if (edge === 0) return [margin, 0, S - margin * 2, depth];
  if (edge === 1) return [S - depth, margin, depth, S - margin * 2];
  if (edge === 2) return [margin, S - depth, S - margin * 2, depth];
  return [0, margin, depth, S - margin * 2];
}

function cornerRect(edge, S) {
  const depth = S * 0.33;
  if (edge === 0) return [S - depth, 0, depth, depth];
  if (edge === 1) return [S - depth, S - depth, depth, depth];
  if (edge === 2) return [0, S - depth, depth, depth];
  return [0, 0, depth, depth];
}

function drawCity(ctx, group, S) {
  const set = new Set(group);
  const stone = ctx.createLinearGradient(0, 0, S, S);
  stone.addColorStop(0, "#ddd6c8");
  stone.addColorStop(1, "#b7b1a4");
  ctx.fillStyle = stone;
  ctx.beginPath();
  for (const edge of group) {
    const [x, y, w, h] = cityRect(edge, S);
    ctx.rect(x, y, w, h);
  }
  for (let edge = 0; edge < 4; edge++) {
    if (!set.has(edge) || !set.has((edge + 1) % 4)) continue;
    const [x, y, w, h] = cornerRect(edge, S);
    ctx.rect(x, y, w, h);
  }
  if (set.has(0) && set.has(2)) ctx.rect(S * 0.15, S * 0.33, S * 0.7, S * 0.34);
  if (set.has(1) && set.has(3)) ctx.rect(S * 0.33, S * 0.15, S * 0.34, S * 0.7);
  if (group.length >= 3) ctx.rect(S * 0.15, S * 0.15, S * 0.7, S * 0.7);
  ctx.fill();

  const m = S * 0.15;
  const d = S * 0.33;
  ctx.strokeStyle = "#5c5142";
  ctx.lineWidth = Math.max(1.5, S * 0.02);
  ctx.lineJoin = "round";
  ctx.beginPath();
  for (const edge of group) {
    const prevOpen = !set.has((edge + 3) % 4);
    const nextOpen = !set.has((edge + 1) % 4);
    const through = group.length >= 3 || (edge % 2 === 0 ? set.has(0) && set.has(2) : set.has(1) && set.has(3));
    if (edge === 0) {
      ctx.moveTo(prevOpen ? m : 0, 0);
      ctx.lineTo(nextOpen ? S - m : S, 0);
      if (prevOpen) { ctx.moveTo(m, 0); ctx.lineTo(m, d); }
      if (nextOpen) { ctx.moveTo(S - m, 0); ctx.lineTo(S - m, d); }
      if (!through) {
        ctx.moveTo(prevOpen ? m : d, d);
        ctx.lineTo(nextOpen ? S - m : S - d, d);
      }
    } else if (edge === 1) {
      ctx.moveTo(S, prevOpen ? m : 0);
      ctx.lineTo(S, nextOpen ? S - m : S);
      if (prevOpen) { ctx.moveTo(S, m); ctx.lineTo(S - d, m); }
      if (nextOpen) { ctx.moveTo(S, S - m); ctx.lineTo(S - d, S - m); }
      if (!through) {
        ctx.moveTo(S - d, prevOpen ? m : d);
        ctx.lineTo(S - d, nextOpen ? S - m : S - d);
      }
    } else if (edge === 2) {
      ctx.moveTo(nextOpen ? m : 0, S);
      ctx.lineTo(prevOpen ? S - m : S, S);
      if (nextOpen) { ctx.moveTo(m, S); ctx.lineTo(m, S - d); }
      if (prevOpen) { ctx.moveTo(S - m, S); ctx.lineTo(S - m, S - d); }
      if (!through) {
        ctx.moveTo(nextOpen ? m : d, S - d);
        ctx.lineTo(prevOpen ? S - m : S - d, S - d);
      }
    } else {
      ctx.moveTo(0, nextOpen ? m : 0);
      ctx.lineTo(0, prevOpen ? S - m : S);
      if (nextOpen) { ctx.moveTo(0, m); ctx.lineTo(d, m); }
      if (prevOpen) { ctx.moveTo(0, S - m); ctx.lineTo(d, S - m); }
      if (!through) {
        ctx.moveTo(d, nextOpen ? m : d);
        ctx.lineTo(d, prevOpen ? S - m : S - d);
      }
    }
  }
  if (set.has(0) && set.has(2) && group.length < 3) {
    ctx.moveTo(m, d); ctx.lineTo(m, S - d);
    ctx.moveTo(S - m, d); ctx.lineTo(S - m, S - d);
  }
  if (set.has(1) && set.has(3) && group.length < 3) {
    ctx.moveTo(d, m); ctx.lineTo(S - d, m);
    ctx.moveTo(d, S - m); ctx.lineTo(S - d, S - m);
  }
  ctx.stroke();

  ctx.fillStyle = "#f7efd8";
  const tooth = S * 0.055;
  const gap = S * 0.04;
  for (const edge of group) {
    const span = S * 0.7;
    const start = (S - span) / 2;
    for (let i = start; i < start + span - tooth; i += tooth + gap) {
      if (edge === 0) ctx.fillRect(i, 0, tooth, S * 0.055);
      if (edge === 1) ctx.fillRect(S - S * 0.055, i, S * 0.055, tooth);
      if (edge === 2) ctx.fillRect(i, S - S * 0.055, tooth, S * 0.055);
      if (edge === 3) ctx.fillRect(0, i, S * 0.055, tooth);
    }
  }
  drawHouses(ctx, group, S);
}

function drawMonastery(ctx, S) {
  const w = S * 0.34;
  const x = (S - w) / 2;
  const roof = S * 0.3;
  ctx.fillStyle = "rgba(40, 30, 16, .18)";
  ctx.beginPath();
  ctx.ellipse(S / 2, roof + S * 0.34, w * 0.62, S * 0.05, 0, 0, Math.PI * 2);
  ctx.fill();
  ctx.fillStyle = "#7d3428";
  ctx.beginPath();
  ctx.moveTo(x - S * 0.04, roof + S * 0.1);
  ctx.lineTo(S / 2, roof - S * 0.02);
  ctx.lineTo(x + w + S * 0.04, roof + S * 0.1);
  ctx.closePath();
  ctx.fill();
  ctx.fillStyle = "#f4e7cb";
  ctx.strokeStyle = "#5c4636";
  ctx.lineWidth = Math.max(1, S * 0.015);
  ctx.beginPath();
  ctx.rect(x, roof + S * 0.1, w, S * 0.24);
  ctx.fill();
  ctx.stroke();
  ctx.fillStyle = "#6a442c";
  ctx.beginPath();
  ctx.moveTo(S / 2 - S * 0.035, roof + S * 0.34);
  ctx.lineTo(S / 2 - S * 0.035, roof + S * 0.2);
  ctx.quadraticCurveTo(S / 2, roof + S * 0.14, S / 2 + S * 0.035, roof + S * 0.2);
  ctx.lineTo(S / 2 + S * 0.035, roof + S * 0.34);
  ctx.fill();
  ctx.fillStyle = "#8d6a45";
  ctx.fillRect(x + S * 0.04, roof + S * 0.16, S * 0.045, S * 0.055);
  ctx.fillRect(x + w - S * 0.085, roof + S * 0.16, S * 0.045, S * 0.055);
  ctx.strokeStyle = "#f3e2b0";
  ctx.lineWidth = Math.max(1.5, S * 0.018);
  ctx.beginPath();
  ctx.moveTo(S / 2, roof - S * 0.07);
  ctx.lineTo(S / 2, roof + S * 0.02);
  ctx.moveTo(S / 2 - S * 0.04, roof - S * 0.035);
  ctx.lineTo(S / 2 + S * 0.04, roof - S * 0.035);
  ctx.stroke();
}

function drawShield(ctx, view, S) {
  const [ax, ay] = anchorOf(view, "city", 0);
  ctx.save();
  ctx.translate(ax * S, ay * S);
  const s = S * 0.11;
  ctx.fillStyle = "#9d2d2a";
  ctx.strokeStyle = "#f0d78a";
  ctx.lineWidth = Math.max(1, S * 0.012);
  ctx.beginPath();
  ctx.moveTo(0, -s);
  ctx.lineTo(s * 0.85, -s * 0.45);
  ctx.lineTo(s * 0.7, s * 0.55);
  ctx.quadraticCurveTo(0, s * 1.05, -s * 0.7, s * 0.55);
  ctx.lineTo(-s * 0.85, -s * 0.45);
  ctx.closePath();
  ctx.fill();
  ctx.stroke();
  ctx.restore();
}

function drawHouses(ctx, group, S) {
  const [ax, ay] = anchorOf({ cities: [group], roads: [], fields: [] }, "city", 0);
  const spots = group.length >= 3
    ? [[0, 0.02], [-0.12, 0.08], [0.12, 0.07], [0, -0.1]]
    : group.length === 2
      ? [[-0.07, 0.02], [0.08, 0.06]]
      : [[0, 0.02]];
  for (const [dx, dy] of spots) {
    const x = (ax + dx) * S;
    const y = (ay + dy) * S;
    const w = S * 0.14;
    const h = S * 0.09;
    ctx.fillStyle = "#efe4cf";
    ctx.strokeStyle = "#6d624f";
    ctx.lineWidth = Math.max(1, S * 0.012);
    ctx.beginPath();
    ctx.rect(x - w / 2, y - h / 2, w, h);
    ctx.fill();
    ctx.stroke();
    ctx.fillStyle = "#9a3b2f";
    ctx.beginPath();
    ctx.moveTo(x - w * 0.72, y - h * 0.45);
    ctx.lineTo(x, y - h * 1.35);
    ctx.lineTo(x + w * 0.72, y - h * 0.45);
    ctx.closePath();
    ctx.fill();
  }
}

function drawVillage(ctx, S) {
  ctx.save();
  ctx.translate(S / 2, S / 2);
  ctx.fillStyle = "#efe6cf";
  ctx.strokeStyle = "#6a4930";
  ctx.lineWidth = Math.max(1, S * 0.015);
  ctx.beginPath();
  ctx.arc(0, 0, S * 0.075, 0, Math.PI * 2);
  ctx.fill();
  ctx.stroke();
  ctx.fillStyle = "#9a3b2f";
  ctx.beginPath();
  ctx.moveTo(-S * 0.045, -S * 0.01);
  ctx.lineTo(0, -S * 0.055);
  ctx.lineTo(S * 0.045, -S * 0.01);
  ctx.closePath();
  ctx.fill();
  ctx.restore();
}

function tintImage(img, color) {
  const key = img.src + "|" + color;
  const hit = tintCache.get(key);
  if (hit) return hit;
  const c = document.createElement("canvas");
  c.width = img.naturalWidth;
  c.height = img.naturalHeight;
  const g = c.getContext("2d");
  g.drawImage(img, 0, 0);
  g.globalCompositeOperation = "source-in";
  g.fillStyle = color;
  g.fillRect(0, 0, c.width, c.height);
  tintCache.set(key, c);
  return c;
}

function darkColor(color) {
  const hex = String(color || "").trim();
  if (!hex.startsWith("#") || hex.length < 7) return false;
  const n = parseInt(hex.slice(1, 7), 16);
  const r = (n >> 16) & 255;
  const g = (n >> 8) & 255;
  const b = n & 255;
  return r + g + b < 200;
}

function drawSpriteMeeple(ctx, x, y, color, S, mode, kind) {
  const pack = meepleArt[kind] || meepleArt.road;
  if (!pack || !pack.fill || !pack.line || !pack.fill.complete || !pack.line.complete) return false;
  const ghost = mode === "ghost" || mode === "hot";
  const fillColor = ghost ? (mode === "hot" ? "#fff8e6" : "rgba(255,248,230,.5)") : (color || "#c23b3b");
  const lineColor = ghost ? "#241910" : (darkColor(color) ? "#f6ecd9" : "#1a120c");
  const size = S * 0.3;
  ctx.save();
  ctx.translate(x, y);
  if (!ghost) {
    ctx.fillStyle = "rgba(20, 12, 8, .28)";
    ctx.beginPath();
    ctx.ellipse(0, size * 0.36, size * 0.26, size * 0.07, 0, 0, Math.PI * 2);
    ctx.fill();
  }
  ctx.drawImage(tintImage(pack.fill, fillColor), -size / 2, -size / 2, size, size);
  ctx.drawImage(tintImage(pack.line, lineColor), -size / 2, -size / 2, size, size);
  ctx.restore();
  return true;
}

function drawMeeple(ctx, x, y, color, S, mode, kind) {
  if (drawSpriteMeeple(ctx, x, y, color, S, mode, kind || "road")) return;
  const ghost = mode === "ghost" || mode === "hot";
  const r = S * 0.055;
  ctx.save();
  ctx.translate(x, y);
  if (!ghost) {
    ctx.fillStyle = "rgba(20, 12, 8, .28)";
    ctx.beginPath();
    ctx.ellipse(0, r * 1.15, r * 1.05, r * 0.28, 0, 0, Math.PI * 2);
    ctx.fill();
  }
  ctx.fillStyle = ghost ? (mode === "hot" ? "rgba(255,248,230,.95)" : "rgba(255,248,230,.45)") : color;
  ctx.strokeStyle = ghost ? "#241910" : (String(color).toLowerCase() === "#222222" ? "#f6ecd9" : "rgba(20,12,8,.55)");
  ctx.lineWidth = Math.max(1.25, S * (ghost ? 0.018 : 0.012));
  if (ghost) ctx.setLineDash([S * 0.03, S * 0.022]);
  ctx.beginPath();
  ctx.arc(0, -r * 1.05, r * 0.5, 0, Math.PI * 2);
  ctx.moveTo(-0.22 * r, -0.62 * r);
  ctx.quadraticCurveTo(-1.25 * r, -0.42 * r, -1.05 * r, 0.35 * r);
  ctx.quadraticCurveTo(-0.9 * r, 1.05 * r, 0, 1.12 * r);
  ctx.quadraticCurveTo(0.9 * r, 1.05 * r, 1.05 * r, 0.35 * r);
  ctx.quadraticCurveTo(1.25 * r, -0.42 * r, 0.22 * r, -0.62 * r);
  ctx.closePath();
  ctx.fill();
  ctx.stroke();
  ctx.restore();
}

function drawTile(ctx, view, S, opts = {}) {
  if (!view) return;
  ctx.save();
  if (opts.ghost) ctx.globalAlpha = 0.72;
  const placed = artPlacement(view.id, opts.rot || 0);
  const img = placed && tileImages[placed.file];
  if (img && img.complete && img.naturalWidth > 0) {
    ctx.save();
    ctx.translate(S / 2, S / 2);
    ctx.rotate((placed.twist % 4) * Math.PI / 2);
    ctx.drawImage(img, -S / 2, -S / 2, S, S);
    ctx.restore();
    ctx.strokeStyle = "rgba(36, 28, 16, .55)";
    ctx.lineWidth = Math.max(1, S * 0.015);
    ctx.strokeRect(0.5, 0.5, S - 1, S - 1);
    if (view.shield) drawShield(ctx, view, S);
    if (opts.meeple) {
      const [ax, ay] = anchorOf(view, opts.meeple.kind, opts.meeple.index);
      drawMeeple(ctx, ax * S, ay * S, opts.color || "#c23b3b", S, "", opts.meeple.kind);
    }
    for (const spot of opts.spots || []) {
      if (spot.free !== true) continue;
      const [ax, ay] = anchorOf(view, spot.kind, spot.index);
      drawMeeple(ctx, ax * S, ay * S, opts.color || "#2a2118", S, spot.hot ? "hot" : "ghost", spot.kind);
    }
    ctx.restore();
    return;
  }
  const grass = ctx.createLinearGradient(0, 0, S, S);
  grass.addColorStop(0, "#9cba6e");
  grass.addColorStop(0.55, "#7ea256");
  grass.addColorStop(1, "#6b8f45");
  ctx.fillStyle = grass;
  ctx.fillRect(0, 0, S, S);
  if (opts.quality !== "low") {
    ctx.fillStyle = "rgba(60, 80, 30, .18)";
    for (let i = 0; i < 11; i++) {
      const px = ((i * 47) % 19) / 19;
      const py = ((i * 29) % 17) / 17;
      ctx.beginPath();
      ctx.ellipse(px * S, py * S, S * 0.035, S * 0.012, i, 0, Math.PI * 2);
      ctx.fill();
    }
  }
  for (const group of view.roads || []) drawRoad(ctx, group, S);
  if ((view.roads || []).length >= 3) drawVillage(ctx, S);
  for (const group of view.cities || []) drawCity(ctx, group, S);
  if (view.monastery) drawMonastery(ctx, S);
  if (view.shield) drawShield(ctx, view, S);
  if (opts.meeple) {
    const [ax, ay] = anchorOf(view, opts.meeple.kind, opts.meeple.index);
    drawMeeple(ctx, ax * S, ay * S, opts.color || "#c23b3b", S, "", opts.meeple.kind);
  }
  for (const spot of opts.spots || []) {
    if (spot.free !== true) continue;
    const [ax, ay] = anchorOf(view, spot.kind, spot.index);
    drawMeeple(ctx, ax * S, ay * S, opts.color || "#2a2118", S, spot.hot ? "hot" : "ghost", spot.kind);
  }
  ctx.strokeStyle = "rgba(36, 28, 16, .65)";
  ctx.lineWidth = Math.max(1, S * 0.018);
  ctx.strokeRect(0.5, 0.5, S - 1, S - 1);
  ctx.restore();
}
