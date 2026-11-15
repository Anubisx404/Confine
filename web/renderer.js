(function (global) {
  "use strict";

  const ELEV_SCALE = 2.4;
  const DEFAULT_PITCH_3D = 0.84;
  const DEFAULT_YAW_3D = -0.45;
  const FOV_Y = (42 * Math.PI) / 180;
  const MIN_ZOOM = 0.12;
  const MAX_ZOOM = 2400.0;
  const LRU_CAPACITY = 120;
  const CODE_LINE_HEIGHT = 16;
  const CODE_HEADER_HEIGHT = 26;
  const CODE_GUTTER_WIDTH = 44;
  const MONO_FONT = "\"Commit Mono\", \"Iosevka\", \"Cascadia Mono\", Consolas, monospace";
  const UI_FONT = "\"Atkinson Hyperlegible\", \"Segoe UI\", sans-serif";

  const LANG_COLORS = {
    Go: "#00add8",
    Rust: "#dea584",
    TypeScript: "#3178c6",
    JavaScript: "#f1e05a",
    Dart: "#00b4ab",
    Python: "#3572a5",
    C: "#555555",
    "C++": "#f34b7d",
    "C#": "#178600",
    Java: "#b07219",
    HTML: "#e34c26",
    CSS: "#563d7c",
    JSON: "#cbcb41",
    YAML: "#cb171e",
    Markdown: "#083fa1",
    SQL: "#e38c00",
    Shell: "#89e051",
    TOML: "#9c4221",
    XML: "#0060ac",
    Text: "#8b949e"
  };

  const GRID_VERT_SRC = [
    "#version 300 es",
    "precision highp float;",
    "layout(location = 0) in vec2 aPos;",
    "uniform mat4 uViewProj;",
    "out vec2 vWorldXY;",
    "void main() {",
    "  vWorldXY = aPos;",
    "  gl_Position = uViewProj * vec4(aPos.x, aPos.y, -0.05, 1.0);",
    "}"
  ].join("\n");

  const GRID_FRAG_SRC = [
    "#version 300 es",
    "precision highp float;",
    "in vec2 vWorldXY;",
    "uniform float uExtrudeMix;",
    "uniform float uFocusMix;",
    "out vec4 outColor;",
    "void main() {",
    "  vec2 cell = floor(vWorldXY / 40.0);",
    "  float checker = mod(cell.x + cell.y, 2.0);",
    "  vec3 baseCol = mix(vec3(0.0667, 0.0745, 0.0902), vec3(0.0863, 0.0941, 0.1137), checker);",
    "  vec2 minorCoord = vWorldXY / 20.0;",
    "  vec2 minorDeriv = fwidth(minorCoord);",
    "  vec2 minorGrid = abs(fract(minorCoord - 0.5) - 0.5) / max(minorDeriv, vec2(1e-4));",
    "  float minorLine = 1.0 - min(min(minorGrid.x, minorGrid.y), 1.0);",
    "  vec2 majorCoord = vWorldXY / 100.0;",
    "  vec2 majorDeriv = fwidth(majorCoord);",
    "  vec2 majorGrid = abs(fract(majorCoord - 0.5) - 0.5) / max(majorDeriv, vec2(1e-4));",
    "  float majorLine = 1.0 - min(min(majorGrid.x, majorGrid.y), 1.0);",
    "  vec3 col = baseCol;",
    "  col = mix(col, vec3(0.118, 0.125, 0.145), minorLine * 0.45);",
    "  col = mix(col, vec3(0.165, 0.173, 0.196), majorLine * 0.65);",
    "  float dist = length(vWorldXY - vec2(500.0, 325.0));",
    "  float fade = 1.0 - smoothstep(1400.0, 4800.0, dist);",
    "  col = mix(vec3(0.0667, 0.0745, 0.0902), col, mix(0.92, fade, uExtrudeMix));",
    "  if (uFocusMix > 0.001) {",
    "    col = mix(col, vec3(0.055, 0.060, 0.075), uFocusMix * 0.78);",
    "  }",
    "  outColor = vec4(col, 1.0);",
    "}"
  ].join("\n");

  const BLOCK_VERT_SRC = [
    "#version 300 es",
    "precision highp float;",
    "layout(location = 0) in vec3 aPos;",
    "layout(location = 1) in vec3 aNormal;",
    "layout(location = 2) in vec4 aRect;",
    "layout(location = 3) in vec2 aElev;",
    "layout(location = 4) in vec4 aColor;",
    "layout(location = 5) in vec4 aFlags;",
    "uniform mat4 uViewProj;",
    "uniform float uExtrudeMix;",
    "uniform float uElevScale;",
    "out vec3 vLocalPos;",
    "out vec3 vWorldNormal;",
    "out vec3 vWorldSize;",
    "out vec4 vColor;",
    "out vec4 vFlags;",
    "void main() {",
    "  float flatZ = aElev.x * 0.002;",
    "  float flatH = 0.001;",
    "  float extZ = aElev.x * uElevScale;",
    "  float extH = max(aElev.y * uElevScale, 0.1);",
    "  float z = mix(flatZ, extZ, uExtrudeMix);",
    "  float h = mix(flatH, extH, uExtrudeMix);",
    "  vec3 worldPos = vec3(",
    "    aRect.x + aPos.x * aRect.z,",
    "    aRect.y + aPos.y * aRect.w,",
    "    z + aPos.z * h",
    "  );",
    "  vLocalPos = aPos;",
    "  vWorldNormal = aNormal;",
    "  vWorldSize = vec3(aRect.z, aRect.w, h);",
    "  vColor = aColor;",
    "  vFlags = aFlags;",
    "  gl_Position = uViewProj * vec4(worldPos, 1.0);",
    "}"
  ].join("\n");

  const BLOCK_FRAG_SRC = [
    "#version 300 es",
    "precision highp float;",
    "in vec3 vLocalPos;",
    "in vec3 vWorldNormal;",
    "in vec3 vWorldSize;",
    "in vec4 vColor;",
    "in vec4 vFlags;",
    "uniform float uExtrudeMix;",
    "uniform float uTime;",
    "uniform float uHasSearch;",
    "uniform float uFocusMix;",
    "out vec4 outColor;",
    "void main() {",
    "  float isDir = vFlags.x;",
    "  float isSelected = vFlags.y;",
    "  float isHovered = vFlags.z;",
    "  float searchHeat = vFlags.w;",
    "  vec3 modCol = vColor.rgb;",
    "  vec3 dirFill = mix(vec3(0.094, 0.102, 0.122), modCol, 0.22);",
    "  vec3 fileFill = mix(vec3(0.118, 0.129, 0.153), modCol, 0.34);",
    "  vec3 baseCol = mix(fileFill, dirFill, isDir);",
    "  vec3 lightDir = normalize(vec3(-0.42, -0.58, 0.70));",
    "  float ndotl = max(dot(normalize(vWorldNormal), lightDir), 0.0);",
    "  float topFactor = step(0.5, vWorldNormal.z);",
    "  float shade3D = mix(0.52 + 0.38 * ndotl, 0.96 + 0.08 * ndotl, topFactor);",
    "  float ao = mix(0.76, 1.04, vLocalPos.z);",
    "  float lighting = mix(1.0, shade3D * mix(ao, 1.0, topFactor), uExtrudeMix);",
    "  vec3 col = baseCol * lighting;",
    "  if (uHasSearch > 0.5 && searchHeat <= 0.001) {",
    "    col *= mix(0.62, 0.80, isDir);",
    "  }",
    "  if (searchHeat > 0.001) {",
    "    float pulse = 0.85 + 0.15 * sin(uTime * 4.5);",
    "    vec3 heatGlow = mix(vec3(0.898, 0.663, 0.235), vec3(0.960, 0.780, 0.360), clamp(searchHeat * 0.85, 0.0, 1.0));",
    "    col = mix(col, heatGlow * pulse, 0.34 + 0.22 * clamp(searchHeat, 0.0, 1.0));",
    "  }",
    "  if (isHovered > 0.5) {",
    "    col = mix(col, vec3(0.910, 0.894, 0.863), 0.11);",
    "  }",
    "  if (isSelected > 0.5) {",
    "    col = mix(col, vec3(0.898, 0.663, 0.235), 0.18);",
    "  }",
    "  vec3 worldCoord = vLocalPos * vWorldSize;",
    "  vec3 fw = max(fwidth(worldCoord), vec3(1e-5));",
    "  vec3 edgeDist = min(vLocalPos, 1.0 - vLocalPos) * vWorldSize;",
    "  vec3 pxVec = edgeDist / fw;",
    "  float pxDist = 999.0;",
    "  if (abs(vWorldNormal.z) > 0.5) {",
    "    pxDist = min(pxVec.x, pxVec.y);",
    "  } else if (abs(vWorldNormal.x) > 0.5) {",
    "    pxDist = min(pxVec.y, pxVec.z);",
    "  } else {",
    "    pxDist = min(pxVec.x, pxVec.z);",
    "  }",
    "  float borderWidth = 1.15;",
    "  vec3 borderCol = mix(col * 0.55, mix(col * 0.70, modCol * 1.15, 0.45), isDir);",
    "  if (searchHeat > 0.001) {",
    "    borderWidth = 2.0;",
    "    borderCol = vec3(0.953, 0.761, 0.349);",
    "  }",
    "  if (isHovered > 0.5) {",
    "    borderWidth = 2.0;",
    "    borderCol = vec3(0.820, 0.790, 0.730);",
    "  }",
    "  if (isSelected > 0.5) {",
    "    borderWidth = 2.6;",
    "    borderCol = vec3(0.898, 0.663, 0.235);",
    "  }",
    "  float feather = isSelected < 0.5 ? (0.5 + uFocusMix * 9.0) : 0.5;",
    "  float borderMask = 1.0 - smoothstep(borderWidth - feather, borderWidth + feather, pxDist);",
    "  if (isSelected < 0.5 && uFocusMix > 0.001) {",
    "    borderMask *= (1.0 - 0.85 * uFocusMix);",
    "  }",
    "  col = mix(col, borderCol, borderMask);",
    "  if (uFocusMix > 0.001 && isSelected < 0.5) {",
    "    float luma = dot(col, vec3(0.299, 0.587, 0.114));",
    "    vec3 muted = mix(vec3(luma) * 0.34, vec3(0.055, 0.060, 0.075), 0.74);",
    "    col = mix(col, muted, uFocusMix);",
    "  }",
    "  outColor = vec4(col, 1.0);",
    "}"
  ].join("\n");

  function buildUnitCubeData() {
    const faces = [
      { n: [0, 0, 1], v: [[0, 0, 1], [0, 1, 1], [1, 1, 1], [0, 0, 1], [1, 1, 1], [1, 0, 1]] },
      { n: [0, 0, -1], v: [[0, 0, 0], [1, 0, 0], [1, 1, 0], [0, 0, 0], [1, 1, 0], [0, 1, 0]] },
      { n: [0, 1, 0], v: [[0, 1, 0], [1, 1, 0], [1, 1, 1], [0, 1, 0], [1, 1, 1], [0, 1, 1]] },
      { n: [0, -1, 0], v: [[0, 0, 0], [0, 0, 1], [1, 0, 1], [0, 0, 0], [1, 0, 1], [1, 0, 0]] },
      { n: [1, 0, 0], v: [[1, 0, 0], [1, 0, 1], [1, 1, 1], [1, 0, 0], [1, 1, 1], [1, 1, 0]] },
      { n: [-1, 0, 0], v: [[0, 0, 0], [0, 1, 0], [0, 1, 1], [0, 0, 0], [0, 1, 1], [0, 0, 1]] }
    ];
    const data = new Float32Array(36 * 6);
    let o = 0;
    for (let f = 0; f < faces.length; f += 1) {
      const face = faces[f];
      for (let i = 0; i < 6; i += 1) {
        data[o++] = face.v[i][0];
        data[o++] = face.v[i][1];
        data[o++] = face.v[i][2];
        data[o++] = face.n[0];
        data[o++] = face.n[1];
        data[o++] = face.n[2];
      }
    }
    return data;
  }

  function clamp(val, min, max) {
    return val < min ? min : val > max ? max : val;
  }

  function lerp(a, b, t) {
    return a + (b - a) * t;
  }

  function easeOutCubic(t) {
    const u = 1 - clamp(t, 0, 1);
    return 1 - u * u * u;
  }

  function multiplyMat4(a, b) {
    const out = new Float32Array(16);
    for (let col = 0; col < 4; col += 1) {
      const b0 = b[col * 4 + 0];
      const b1 = b[col * 4 + 1];
      const b2 = b[col * 4 + 2];
      const b3 = b[col * 4 + 3];
      out[col * 4 + 0] = a[0] * b0 + a[4] * b1 + a[8] * b2 + a[12] * b3;
      out[col * 4 + 1] = a[1] * b0 + a[5] * b1 + a[9] * b2 + a[13] * b3;
      out[col * 4 + 2] = a[2] * b0 + a[6] * b1 + a[10] * b2 + a[14] * b3;
      out[col * 4 + 3] = a[3] * b0 + a[7] * b1 + a[11] * b2 + a[15] * b3;
    }
    return out;
  }

  function invertMat4(m) {
    const inv = new Float32Array(16);
    inv[0] = m[5] * m[10] * m[15] - m[5] * m[11] * m[14] - m[9] * m[6] * m[15] + m[9] * m[7] * m[14] + m[13] * m[6] * m[11] - m[13] * m[7] * m[10];
    inv[4] = -m[4] * m[10] * m[15] + m[4] * m[11] * m[14] + m[8] * m[6] * m[15] - m[8] * m[7] * m[14] - m[12] * m[6] * m[11] + m[12] * m[7] * m[10];
    inv[8] = m[4] * m[9] * m[15] - m[4] * m[11] * m[13] - m[8] * m[5] * m[15] + m[8] * m[7] * m[13] + m[12] * m[5] * m[11] - m[12] * m[7] * m[9];
    inv[12] = -m[4] * m[9] * m[14] + m[4] * m[10] * m[13] + m[8] * m[5] * m[14] - m[8] * m[6] * m[13] - m[12] * m[5] * m[10] + m[12] * m[6] * m[9];
    inv[1] = -m[1] * m[10] * m[15] + m[1] * m[11] * m[14] + m[9] * m[2] * m[15] - m[9] * m[3] * m[14] - m[13] * m[2] * m[11] + m[13] * m[3] * m[10];
    inv[5] = m[0] * m[10] * m[15] - m[0] * m[11] * m[14] - m[8] * m[2] * m[15] + m[8] * m[3] * m[14] + m[12] * m[2] * m[11] - m[12] * m[3] * m[10];
    inv[9] = -m[0] * m[9] * m[15] + m[0] * m[11] * m[13] + m[8] * m[1] * m[15] - m[8] * m[3] * m[13] - m[12] * m[1] * m[11] + m[12] * m[3] * m[9];
    inv[13] = m[0] * m[9] * m[14] - m[0] * m[10] * m[13] - m[8] * m[1] * m[14] + m[8] * m[2] * m[13] + m[12] * m[1] * m[10] - m[12] * m[2] * m[9];
    inv[2] = m[1] * m[6] * m[15] - m[1] * m[7] * m[14] - m[5] * m[2] * m[15] + m[5] * m[3] * m[14] + m[13] * m[2] * m[7] - m[13] * m[3] * m[6];
    inv[6] = -m[0] * m[6] * m[15] + m[0] * m[7] * m[14] + m[4] * m[2] * m[15] - m[4] * m[3] * m[14] - m[12] * m[2] * m[7] + m[12] * m[3] * m[6];
    inv[10] = m[0] * m[5] * m[15] - m[0] * m[7] * m[13] - m[4] * m[1] * m[15] + m[4] * m[3] * m[13] + m[12] * m[1] * m[7] - m[12] * m[3] * m[5];
    inv[14] = -m[0] * m[5] * m[14] + m[0] * m[6] * m[13] + m[4] * m[1] * m[14] - m[4] * m[2] * m[13] - m[12] * m[1] * m[6] + m[12] * m[2] * m[5];
    inv[3] = -m[1] * m[6] * m[11] + m[1] * m[7] * m[10] + m[5] * m[2] * m[11] - m[5] * m[3] * m[10] - m[9] * m[2] * m[7] + m[9] * m[3] * m[6];
    inv[7] = m[0] * m[6] * m[11] - m[0] * m[7] * m[10] - m[4] * m[2] * m[11] + m[4] * m[3] * m[10] + m[8] * m[2] * m[7] - m[8] * m[3] * m[6];
    inv[11] = -m[0] * m[5] * m[11] + m[0] * m[7] * m[9] + m[4] * m[1] * m[11] - m[4] * m[3] * m[9] - m[8] * m[1] * m[7] + m[8] * m[3] * m[5];
    inv[15] = m[0] * m[5] * m[10] - m[0] * m[6] * m[9] - m[4] * m[1] * m[10] + m[4] * m[2] * m[9] + m[8] * m[1] * m[6] - m[8] * m[2] * m[5];
    let det = m[0] * inv[0] + m[1] * inv[4] + m[2] * inv[8] + m[3] * inv[12];
    if (Math.abs(det) < 1e-12) return null;
    det = 1.0 / det;
    for (let i = 0; i < 16; i += 1) inv[i] *= det;
    return inv;
  }

  function unprojectNDC(invVP, ndcX, ndcY, ndcZ) {
    const x = invVP[0] * ndcX + invVP[4] * ndcY + invVP[8] * ndcZ + invVP[12];
    const y = invVP[1] * ndcX + invVP[5] * ndcY + invVP[9] * ndcZ + invVP[13];
    const z = invVP[2] * ndcX + invVP[6] * ndcY + invVP[10] * ndcZ + invVP[14];
    const w = invVP[3] * ndcX + invVP[7] * ndcY + invVP[11] * ndcZ + invVP[15];
    const invW = Math.abs(w) > 1e-9 ? 1.0 / w : 1.0;
    return [x * invW, y * invW, z * invW];
  }

  function intersectRayAABB(ox, oy, oz, invDx, invDy, invDz, minX, minY, minZ, maxX, maxY, maxZ) {
    const tx1 = (minX - ox) * invDx;
    const tx2 = (maxX - ox) * invDx;
    let tmin = Math.min(tx1, tx2);
    let tmax = Math.max(tx1, tx2);

    const ty1 = (minY - oy) * invDy;
    const ty2 = (maxY - oy) * invDy;
    tmin = Math.max(tmin, Math.min(ty1, ty2));
    tmax = Math.min(tmax, Math.max(ty1, ty2));

    const tz1 = (minZ - oz) * invDz;
    const tz2 = (maxZ - oz) * invDz;
    tmin = Math.max(tmin, Math.min(tz1, tz2));
    tmax = Math.min(tmax, Math.max(tz1, tz2));

    if (tmax >= Math.max(tmin, 0.0)) {
      return tmin >= 0.0 ? tmin : tmax;
    }
    return -1.0;
  }

  function parseHexRgba(color) {
    if (!color || typeof color !== "string") return [0.35, 0.55, 0.85, 1.0];
    let hex = color.trim();
    if (hex[0] === "#") hex = hex.slice(1);
    if (hex.length === 6) {
      const r = parseInt(hex.slice(0, 2), 16) / 255;
      const g = parseInt(hex.slice(2, 4), 16) / 255;
      const b = parseInt(hex.slice(4, 6), 16) / 255;
      if (Number.isFinite(r) && Number.isFinite(g) && Number.isFinite(b)) {
        return [r, g, b, 1.0];
      }
    }
    return [0.35, 0.55, 0.85, 1.0];
  }

  function compileShader(gl, type, source) {
    const shader = gl.createShader(type);
    gl.shaderSource(shader, source);
    gl.compileShader(shader);
    if (!gl.getShaderParameter(shader, gl.COMPILE_STATUS)) {
      const log = gl.getShaderInfoLog(shader);
      gl.deleteShader(shader);
      throw new Error("WebGL2 shader compile error: " + log);
    }
    return shader;
  }

  function createProgram(gl, vertSrc, fragSrc) {
    const vs = compileShader(gl, gl.VERTEX_SHADER, vertSrc);
    const fs = compileShader(gl, gl.FRAGMENT_SHADER, fragSrc);
    const prog = gl.createProgram();
    gl.attachShader(prog, vs);
    gl.attachShader(prog, fs);
    gl.linkProgram(prog);
    gl.deleteShader(vs);
    gl.deleteShader(fs);
    if (!gl.getProgramParameter(prog, gl.LINK_STATUS)) {
      const log = gl.getProgramInfoLog(prog);
      gl.deleteProgram(prog);
      throw new Error("WebGL2 program link error: " + log);
    }
    return prog;
  }

  function truncateText(ctx, text, maxW) {
    if (maxW <= 4) return "";
    if (ctx.measureText(text).width <= maxW) return text;
    const ellipsis = "...";
    const ellW = ctx.measureText(ellipsis).width;
    if (maxW <= ellW) return "";
    let lo = 0;
    let hi = text.length;
    while (lo < hi) {
      const mid = (lo + hi + 1) >> 1;
      if (ctx.measureText(text.slice(0, mid)).width + ellW <= maxW) {
        lo = mid;
      } else {
        hi = mid - 1;
      }
    }
    return text.slice(0, lo) + ellipsis;
  }

  function roundRectPath(ctx, x, y, w, h, r) {
    const rad = Math.min(r, w * 0.5, h * 0.5);
    ctx.beginPath();
    ctx.moveTo(x + rad, y);
    ctx.lineTo(x + w - rad, y);
    ctx.quadraticCurveTo(x + w, y, x + w, y + rad);
    ctx.lineTo(x + w, y + h - rad);
    ctx.quadraticCurveTo(x + w, y + h, x + w - rad, y + h);
    ctx.lineTo(x + rad, y + h);
    ctx.quadraticCurveTo(x, y + h, x, y + h - rad);
    ctx.lineTo(x, y + rad);
    ctx.quadraticCurveTo(x, y, x + rad, y);
    ctx.closePath();
  }

  function rectsOverlap(a, b) {
    return !(a.x + a.w <= b.x || b.x + b.w <= a.x || a.y + a.h <= b.y || b.y + b.h <= a.y);
  }

  class ConfineRenderer {
    constructor(glCanvas, overlayCanvas, callbacks) {
      this.glCanvas = glCanvas;
      this.overlayCanvas = overlayCanvas;
      this.callbacks = callbacks || {};
      this.ctx2d = overlayCanvas ? overlayCanvas.getContext("2d") : null;

      this.vpW = 800;
      this.vpH = 600;
      this.dpr = 1;

      this.tree = null;
      this.nodes = [];
      this.nodesByPath = new Map();
      this.nodesById = new Map();
      this.childrenByIdx = [];
      this.subtreeMaxTopZ = new Float32Array(0);
      this.effZ = new Float32Array(0);
      this.effH = new Float32Array(0);
      this.flagsData = new Float32Array(0);
      this.canvasRect = { x: 0, y: 0, w: 1000, h: 650 };
      this.lastRootPath = null;
      this.hasLoadedOnce = false;

      this.selectedRelPath = null;
      this.selectedLine = null;
      this.hoveredRelPath = null;
      this.hoveredLine = null;
      this.focusedFileRelPath = null;
      this.lastFocusedRelPath = null;
      this.curFocusMix = 0.0;
      this.tgtFocusMix = 0.0;
      this.needsWebGL = true;
      this.surroundMidCanvas = null;
      this.surroundMidCtx = null;
      this.surroundLowCanvas = null;
      this.surroundLowCtx = null;
      this._surroundCacheKey = "";
      this._surroundCacheReady = false;
      this.initSurroundCanvases();

      this.fileHitsMap = new Map();
      this.matchedLinesMap = new Map();
      this.activeQuery = "";
      this.hasSearch = false;

      this.fileCache = new Map();
      this.pendingFiles = new Set();
      this.scrollOffsets = new Map();
      this.lod2HitRegions = [];

      this.is3D = false;
      this.last3DPitch = DEFAULT_PITCH_3D;
      this.last3DYaw = DEFAULT_YAW_3D;

      this.curCenterX = 500;
      this.curCenterY = 325;
      this.curCenterZ = 0;
      this.curZoom = 1;
      this.curYaw = 0;
      this.curPitch = 0;
      this.curExtrude = 0;

      this.tgtCenterX = 500;
      this.tgtCenterY = 325;
      this.tgtCenterZ = 0;
      this.tgtZoom = 1;
      this.tgtYaw = 0;
      this.tgtPitch = 0;
      this.tgtExtrude = 0;

      this.tween = null;
      this.dirtyFlags = false;
      this.needsRender = true;
      this.rafId = 0;
      this.lastFrameTime = performance.now();
      this.destroyed = false;

      this.dragState = null;
      this.lastViewProj = null;
      this.lastInvViewProj = null;

      this.gl = null;
      this.glReady = false;
      this.motionQuery =
        typeof window !== "undefined" && typeof window.matchMedia === "function"
          ? window.matchMedia("(prefers-reduced-motion: reduce)")
          : null;
      this.initWebGL();
      this.bindEvents();
      this.resize();
      this.startLoop();
    }

    initSurroundCanvases() {
      if (typeof document === "undefined" || typeof document.createElement !== "function") return;
      const mid = document.createElement("canvas");
      const low = document.createElement("canvas");
      this.surroundMidCanvas = mid;
      this.surroundMidCtx = mid.getContext ? mid.getContext("2d") : null;
      this.surroundLowCanvas = low;
      this.surroundLowCtx = low.getContext ? low.getContext("2d") : null;
    }

    ensureSurroundPyramid() {
      if (!this.surroundMidCanvas || !this.surroundLowCanvas || !this.surroundMidCtx || !this.surroundLowCtx) {
        return false;
      }
      const midW = Math.max(64, Math.floor(this.vpW / 4));
      const midH = Math.max(48, Math.floor(this.vpH / 4));
      const lowW = Math.max(32, Math.floor(this.vpW / 8));
      const lowH = Math.max(24, Math.floor(this.vpH / 8));
      if (this.surroundMidCanvas.width !== midW || this.surroundMidCanvas.height !== midH) {
        this.surroundMidCanvas.width = midW;
        this.surroundMidCanvas.height = midH;
      }
      if (this.surroundLowCanvas.width !== lowW || this.surroundLowCanvas.height !== lowH) {
        this.surroundLowCanvas.width = lowW;
        this.surroundLowCanvas.height = lowH;
      }
      return true;
    }

    invalidateSurroundCache() {
      this._surroundCacheKey = "";
      this._surroundCacheReady = false;
    }

    initWebGL() {
      if (!this.glCanvas) return;
      const gl = this.glCanvas.getContext("webgl2", {
        antialias: true,
        alpha: false,
        preserveDrawingBuffer: true,
        powerPreference: "high-performance"
      });
      if (!gl) return;
      this.gl = gl;

      try {
        this.gridProgram = createProgram(gl, GRID_VERT_SRC, GRID_FRAG_SRC);
        this.gridUniforms = {
          uViewProj: gl.getUniformLocation(this.gridProgram, "uViewProj"),
          uExtrudeMix: gl.getUniformLocation(this.gridProgram, "uExtrudeMix"),
          uFocusMix: gl.getUniformLocation(this.gridProgram, "uFocusMix")
        };

        this.gridVao = gl.createVertexArray();
        gl.bindVertexArray(this.gridVao);
        this.gridVbo = gl.createBuffer();
        gl.bindBuffer(gl.ARRAY_BUFFER, this.gridVbo);
        gl.bufferData(
          gl.ARRAY_BUFFER,
          new Float32Array([-5000, -5000, 6000, -5000, -5000, 6000, 6000, 6000]),
          gl.STATIC_DRAW
        );
        gl.enableVertexAttribArray(0);
        gl.vertexAttribPointer(0, 2, gl.FLOAT, false, 8, 0);

        this.blockProgram = createProgram(gl, BLOCK_VERT_SRC, BLOCK_FRAG_SRC);
        this.blockUniforms = {
          uViewProj: gl.getUniformLocation(this.blockProgram, "uViewProj"),
          uExtrudeMix: gl.getUniformLocation(this.blockProgram, "uExtrudeMix"),
          uElevScale: gl.getUniformLocation(this.blockProgram, "uElevScale"),
          uTime: gl.getUniformLocation(this.blockProgram, "uTime"),
          uHasSearch: gl.getUniformLocation(this.blockProgram, "uHasSearch"),
          uFocusMix: gl.getUniformLocation(this.blockProgram, "uFocusMix")
        };

        this.blockVao = gl.createVertexArray();
        gl.bindVertexArray(this.blockVao);

        this.cubeVbo = gl.createBuffer();
        gl.bindBuffer(gl.ARRAY_BUFFER, this.cubeVbo);
        gl.bufferData(gl.ARRAY_BUFFER, buildUnitCubeData(), gl.STATIC_DRAW);
        gl.enableVertexAttribArray(0);
        gl.vertexAttribPointer(0, 3, gl.FLOAT, false, 24, 0);
        gl.enableVertexAttribArray(1);
        gl.vertexAttribPointer(1, 3, gl.FLOAT, false, 24, 12);

        this.staticInstanceVbo = gl.createBuffer();
        gl.bindBuffer(gl.ARRAY_BUFFER, this.staticInstanceVbo);
        gl.enableVertexAttribArray(2);
        gl.vertexAttribPointer(2, 4, gl.FLOAT, false, 40, 0);
        gl.vertexAttribDivisor(2, 1);

        gl.enableVertexAttribArray(3);
        gl.vertexAttribPointer(3, 2, gl.FLOAT, false, 40, 16);
        gl.vertexAttribDivisor(3, 1);

        gl.enableVertexAttribArray(4);
        gl.vertexAttribPointer(4, 4, gl.FLOAT, false, 40, 24);
        gl.vertexAttribDivisor(4, 1);

        this.flagsInstanceVbo = gl.createBuffer();
        gl.bindBuffer(gl.ARRAY_BUFFER, this.flagsInstanceVbo);
        gl.enableVertexAttribArray(5);
        gl.vertexAttribPointer(5, 4, gl.FLOAT, false, 16, 0);
        gl.vertexAttribDivisor(5, 1);

        gl.bindVertexArray(null);
        this.glReady = true;
      } catch (err) {
        this.glReady = false;
      }
    }

    bindEvents() {
      this._onPointerDown = (e) => this.handlePointerDown(e);
      this._onPointerMove = (e) => this.handlePointerMove(e);
      this._onPointerUp = (e) => this.handlePointerUp(e);
      this._onWheel = (e) => this.handleWheel(e);
      this._onDblClick = (e) => this.handleDblClick(e);
      this._onContextMenu = (e) => e.preventDefault();

      const targets = [];
      if (this.glCanvas) targets.push(this.glCanvas);
      if (this.overlayCanvas && this.overlayCanvas !== this.glCanvas) targets.push(this.overlayCanvas);
      this._eventTargets = targets;

      for (let i = 0; i < targets.length; i += 1) {
        const el = targets[i];
        el.addEventListener("pointerdown", this._onPointerDown);
        el.addEventListener("pointermove", this._onPointerMove);
        el.addEventListener("wheel", this._onWheel, { passive: false });
        el.addEventListener("dblclick", this._onDblClick);
        el.addEventListener("contextmenu", this._onContextMenu);
      }
      window.addEventListener("pointermove", this._onPointerMove);
      window.addEventListener("pointerup", this._onPointerUp);
      window.addEventListener("pointercancel", this._onPointerUp);
    }

    setTree(treeResponse) {
      if (!treeResponse) return;
      const prevRoot = this.lastRootPath;
      const nextRoot = treeResponse.rootPath || "";
      const rootChanged = prevRoot !== null && prevRoot !== nextRoot;

      this.tree = treeResponse;
      this.canvasRect = treeResponse.canvasRect || { x: 0, y: 0, w: 1000, h: 650 };
      const rawNodes = Array.isArray(treeResponse.nodes) ? treeResponse.nodes : [];
      this.nodes = rawNodes;
      this.nodesByPath.clear();
      this.nodesById.clear();

      const count = rawNodes.length;
      this.childrenByIdx = new Array(count);
      for (let i = 0; i < count; i += 1) {
        this.childrenByIdx[i] = [];
        const n = rawNodes[i];
        this.nodesByPath.set(n.relPath || "", n);
        this.nodesById.set(n.id, i);
      }

      for (let i = 0; i < count; i += 1) {
        const n = rawNodes[i];
        if (i === 0 && n.parentId === n.id) continue;
        const pIdx = this.nodesById.get(n.parentId);
        if (pIdx !== undefined && pIdx !== i) {
          this.childrenByIdx[pIdx].push(i);
        }
      }

      this.effZ = new Float32Array(count);
      this.effH = new Float32Array(count);
      this.subtreeMaxTopZ = new Float32Array(count);
      const staticBuf = new Float32Array(count * 10);
      this.flagsData = new Float32Array(count * 4);

      for (let i = 0; i < count; i += 1) {
        const n = rawNodes[i];
        let zVal = Number.isFinite(n.z) ? n.z : (n.depth || 0) * 1.4;
        const hVal = Number.isFinite(n.height) ? Math.max(n.height, 0.2) : 1.2;
        if (i > 0) {
          const pIdx = this.nodesById.get(n.parentId);
          if (pIdx !== undefined && pIdx !== i) {
            const pTop = this.effZ[pIdx] + this.effH[pIdx];
            if (zVal < pTop + 0.05) {
              zVal = pTop + 0.08;
            }
          }
        }
        this.effZ[i] = zVal;
        this.effH[i] = hVal;
        this.subtreeMaxTopZ[i] = zVal + hVal;

        const b = n.bounds || { x: 0, y: 0, w: 0, h: 0 };
        const sOff = i * 10;
        staticBuf[sOff + 0] = b.x || 0;
        staticBuf[sOff + 1] = b.y || 0;
        staticBuf[sOff + 2] = Math.max(b.w || 0, 0.001);
        staticBuf[sOff + 3] = Math.max(b.h || 0, 0.001);
        staticBuf[sOff + 4] = zVal;
        staticBuf[sOff + 5] = hVal;

        let rgba;
        if (Array.isArray(n.colorRgba) && n.colorRgba.length >= 3) {
          rgba = [
            n.colorRgba[0] / 255,
            n.colorRgba[1] / 255,
            n.colorRgba[2] / 255,
            n.colorRgba.length > 3 ? n.colorRgba[3] / 255 : 1.0
          ];
        } else {
          rgba = parseHexRgba(n.color);
        }
        staticBuf[sOff + 6] = rgba[0];
        staticBuf[sOff + 7] = rgba[1];
        staticBuf[sOff + 8] = rgba[2];
        staticBuf[sOff + 9] = rgba[3];
      }

      for (let i = count - 1; i >= 0; i -= 1) {
        const ch = this.childrenByIdx[i];
        let maxTop = this.subtreeMaxTopZ[i];
        for (let k = 0; k < ch.length; k += 1) {
          const cTop = this.subtreeMaxTopZ[ch[k]];
          if (cTop > maxTop) maxTop = cTop;
        }
        this.subtreeMaxTopZ[i] = maxTop;
      }

      if (this.glReady && this.gl) {
        const gl = this.gl;
        gl.bindBuffer(gl.ARRAY_BUFFER, this.staticInstanceVbo);
        gl.bufferData(gl.ARRAY_BUFFER, staticBuf.length > 0 ? staticBuf : 40, gl.STATIC_DRAW);
      }

      if (rootChanged) {
        this.fileCache.clear();
        this.pendingFiles.clear();
        this.scrollOffsets.clear();
        this.selectedRelPath = null;
        this.selectedLine = null;
        this.hoveredRelPath = null;
        this.hoveredLine = null;
        this.focusedFileRelPath = null;
        this.lastFocusedRelPath = null;
        this.tgtFocusMix = 0.0;
        this.curFocusMix = 0.0;
      } else {
        if (this.selectedRelPath !== null && !this.nodesByPath.has(this.selectedRelPath)) {
          this.selectedRelPath = null;
          this.selectedLine = null;
        }
        if (this.focusedFileRelPath !== null && !this.nodesByPath.has(this.focusedFileRelPath)) {
          this.focusedFileRelPath = null;
          this.lastFocusedRelPath = null;
          this.tgtFocusMix = 0.0;
          this.curFocusMix = 0.0;
        }
        if (this.hoveredRelPath !== null && !this.nodesByPath.has(this.hoveredRelPath)) {
          this.hoveredRelPath = null;
          this.hoveredLine = null;
        }
      }

      this.lastRootPath = nextRoot;
      this.invalidateSurroundCache();
      this.rebuildFlagsBuffer(true);

      if (!this.hasLoadedOnce || rootChanged || count === 0) {
        this.hasLoadedOnce = true;
        this.fitView(false);
      } else {
        this.requestRender();
      }
    }

    rebuildFlagsBuffer(fullAlloc) {
      const count = this.nodes.length;
      if (this.flagsData.length !== count * 4) {
        this.flagsData = new Float32Array(count * 4);
        fullAlloc = true;
      }
      for (let i = 0; i < count; i += 1) {
        const n = this.nodes[i];
        const fOff = i * 4;
        this.flagsData[fOff + 0] = n.isDir ? 1.0 : 0.0;
        this.flagsData[fOff + 1] = this.selectedRelPath !== null && n.relPath === this.selectedRelPath ? 1.0 : 0.0;
        this.flagsData[fOff + 2] = this.hoveredRelPath !== null && n.relPath === this.hoveredRelPath ? 1.0 : 0.0;
        const hits = this.fileHitsMap.get(n.relPath) || 0;
        this.flagsData[fOff + 3] = hits > 0 ? Math.min(1.0, 0.3 + Math.log2(hits + 1) * 0.25) : 0.0;
      }

      if (this.glReady && this.gl && count > 0) {
        const gl = this.gl;
        gl.bindBuffer(gl.ARRAY_BUFFER, this.flagsInstanceVbo);
        if (fullAlloc) {
          gl.bufferData(gl.ARRAY_BUFFER, this.flagsData, gl.DYNAMIC_DRAW);
        } else {
          gl.bufferSubData(gl.ARRAY_BUFFER, 0, this.flagsData);
        }
      }
      this.dirtyFlags = false;
      this.requestRender();
    }

    setMode3D(is3D) {
      const next = Boolean(is3D);
      this.is3D = next;
      const pitchTarget = next ? this.last3DPitch : 0.0;
      const yawTarget = next ? this.last3DYaw : 0.0;
      const extrudeTarget = next ? 1.0 : 0.0;
      const zTarget = next ? this.computeFocusCenterZ(this.tgtCenterX, this.tgtCenterY, this.tgtZoom, 1.0) : 0.0;

      this.startTween({
        centerX: this.tgtCenterX,
        centerY: this.tgtCenterY,
        centerZ: zTarget,
        zoom: this.tgtZoom,
        yaw: yawTarget,
        pitch: pitchTarget,
        extrude: extrudeTarget
      }, 380);
    }

    fitView(animate = true) {
      this.clearFileFocus(true);
      const rect = this.canvasRect || { x: 0, y: 0, w: 1000, h: 650 };
      const rectX = Number.isFinite(rect.x) ? rect.x : 0;
      const rectY = Number.isFinite(rect.y) ? rect.y : 0;
      const rectW = Number.isFinite(rect.w) && rect.w > 0 ? rect.w : 1000;
      const rectH = Number.isFinite(rect.h) && rect.h > 0 ? rect.h : 650;
      const cx = rectX + rectW * 0.5;
      const cy = rectY + rectH * 0.5;
      const pitchCos = Math.max(Math.cos(this.tgtPitch), 0.35);
      const fitW = (this.vpW * 0.90) / Math.max(rectW, 1);
      const fitH = (this.vpH * 0.90) / Math.max(rectH * pitchCos, 1);
      const zoom = clamp(Math.min(fitW, fitH), MIN_ZOOM, MAX_ZOOM);

      const target = {
        centerX: cx,
        centerY: cy,
        centerZ: 0,
        zoom: zoom,
        yaw: this.tgtYaw,
        pitch: this.tgtPitch,
        extrude: this.tgtExtrude
      };
      if (animate) {
        this.startTween(target, 420);
      } else {
        this.snapToTarget(target);
      }
    }

    flyToNode(relPath, line = null, animate = true) {
      const node = this.nodesByPath.get(relPath || "");
      if (!node || !node.bounds) return;
      const b = node.bounds;
      const bx = Number.isFinite(b.x) ? b.x : 0;
      const by = Number.isFinite(b.y) ? b.y : 0;
      const bw = Number.isFinite(b.w) && b.w > 0 ? b.w : 1;
      const bh = Number.isFinite(b.h) && b.h > 0 ? b.h : 1;
      const cx = bx + bw * 0.5;
      const cy = by + bh * 0.5;
      const pitchCos = Math.max(Math.cos(this.tgtPitch), 0.35);

      let targetZoom;
      if (line !== null && line !== undefined && !node.isDir) {
        const zForW = 380 / Math.max(bw, 1e-3);
        const zForH = 280 / Math.max(bh * pitchCos, 1e-3);
        targetZoom = clamp(Math.max(zForW, zForH), MIN_ZOOM, MAX_ZOOM);

        const expectedScreenH = Math.max(260, bh * targetZoom * pitchCos);
        const bodyH = Math.max(80, expectedScreenH - CODE_HEADER_HEIGHT);
        const targetScroll = Math.max(0, (line - 1) * CODE_LINE_HEIGHT - bodyH * 0.42);
        this.scrollOffsets.set(relPath, targetScroll);
        this.ensureFileRequested(relPath);
      } else {
        const zW = (this.vpW * 0.76) / Math.max(bw, 1e-3);
        const zH = (this.vpH * 0.76) / Math.max(bh * pitchCos, 1e-3);
        targetZoom = clamp(Math.min(zW, zH), MIN_ZOOM, MAX_ZOOM);
      }

      const idx = this.nodesById.get(node.id);
      const topZ = idx !== undefined ? (this.effZ[idx] + this.effH[idx]) * ELEV_SCALE : 0;
      const targetCenterZ = this.is3D ? topZ * this.tgtExtrude * clamp((targetZoom - 1.5) / 8.0, 0, 1) : 0;

      const target = {
        centerX: cx,
        centerY: cy,
        centerZ: targetCenterZ,
        zoom: targetZoom,
        yaw: this.tgtYaw,
        pitch: this.tgtPitch,
        extrude: this.tgtExtrude
      };
      if (animate) {
        this.startTween(target, 440);
      } else {
        this.snapToTarget(target);
      }
    }

    setSearchHighlights(fileHitsMap, matchedLinesMap, activeQuery) {
      this.fileHitsMap.clear();
      if (fileHitsMap && typeof fileHitsMap === "object") {
        if (fileHitsMap instanceof Map) {
          fileHitsMap.forEach((v, k) => {
            if (v > 0) this.fileHitsMap.set(k, v);
          });
        } else {
          const keys = Object.keys(fileHitsMap);
          for (let i = 0; i < keys.length; i += 1) {
            const k = keys[i];
            if (fileHitsMap[k] > 0) this.fileHitsMap.set(k, fileHitsMap[k]);
          }
        }
      }

      this.matchedLinesMap.clear();
      if (matchedLinesMap && typeof matchedLinesMap === "object") {
        const entries = matchedLinesMap instanceof Map
          ? Array.from(matchedLinesMap.entries())
          : Object.keys(matchedLinesMap).map((k) => [k, matchedLinesMap[k]]);
        for (let i = 0; i < entries.length; i += 1) {
          const path = entries[i][0];
          const val = entries[i][1];
          if (val instanceof Set) {
            this.matchedLinesMap.set(path, new Set(val));
          } else if (Array.isArray(val)) {
            this.matchedLinesMap.set(path, new Set(val));
          }
        }
      }

      this.activeQuery = activeQuery ? String(activeQuery) : "";
      this.hasSearch = this.activeQuery.trim().length > 0 && this.fileHitsMap.size > 0;
      this.invalidateSurroundCache();
      this.rebuildFlagsBuffer(false);
    }

    isFileFocused() {
      return Boolean(this.focusedFileRelPath && this.tgtFocusMix > 0.5);
    }

    focusFile(relPath, line = null, animate = true) {
      const key = relPath !== undefined && relPath !== null ? String(relPath) : "";
      const node = this.nodesByPath.get(key);
      if (!node || node.isDir) {
        this.clearFileFocus(false);
        if (node) {
          this.setSelected(node.relPath, null);
          this.flyToNode(node.relPath, null, animate);
        }
        return;
      }

      const targetLine = line !== null && line !== undefined && Number(line) > 0 ? Number(line) : null;
      this.selectedRelPath = node.relPath;
      this.selectedLine = targetLine;
      this.focusedFileRelPath = node.relPath;
      this.lastFocusedRelPath = node.relPath;
      this.tgtFocusMix = 1.0;
      this.invalidateSurroundCache();

      this.ensureFileRequested(node.relPath);
      this.rebuildFlagsBuffer(false);
      this.flyToNode(node.relPath, targetLine, animate);

      if (targetLine !== null) {
        const expectedH = clamp(Math.max(240, this.vpH * 0.80), 240, Math.min(this.vpH - 52, 780));
        const bodyH = Math.max(80, expectedH - CODE_HEADER_HEIGHT);
        const lineTop = (targetLine - 1) * CODE_LINE_HEIGHT;
        const targetScroll = Math.max(0, lineTop - bodyH * 0.42);
        this.scrollOffsets.set(node.relPath, targetScroll);
      }

      if (this.callbacks && typeof this.callbacks.onFocusChange === "function") {
        this.callbacks.onFocusChange(node.relPath);
      }
      this.requestRender();
    }

    clearFileFocus(notify = false) {
      const hadFocus = Boolean(this.focusedFileRelPath || this.tgtFocusMix > 0);
      this.focusedFileRelPath = null;
      this.tgtFocusMix = 0.0;
      this.invalidateSurroundCache();
      if (hadFocus) {
        this.rebuildFlagsBuffer(false);
        this.requestRender();
        if (notify && this.callbacks && typeof this.callbacks.onFocusChange === "function") {
          this.callbacks.onFocusChange(null);
        }
      }
    }

    setSelected(relPath, line = null) {
      const prevFocused = this.focusedFileRelPath;
      const prevTgtFocus = this.tgtFocusMix;
      this.selectedRelPath = relPath !== undefined && relPath !== null ? String(relPath) : null;
      this.selectedLine = line !== null && line !== undefined ? Number(line) : null;

      if (this.selectedRelPath !== null) {
        const selNode = this.nodesByPath.get(this.selectedRelPath);
        if (!selNode || selNode.isDir) {
          this.focusedFileRelPath = null;
          this.tgtFocusMix = 0.0;
        } else if (this.focusedFileRelPath && this.focusedFileRelPath !== this.selectedRelPath) {
          this.focusedFileRelPath = this.selectedRelPath;
          this.lastFocusedRelPath = this.selectedRelPath;
          this.tgtFocusMix = 1.0;
        }
      } else {
        this.focusedFileRelPath = null;
        this.tgtFocusMix = 0.0;
      }

      if (prevFocused !== this.focusedFileRelPath || prevTgtFocus !== this.tgtFocusMix) {
        this.invalidateSurroundCache();
      }

      if (this.selectedRelPath !== null && this.selectedLine !== null) {
        const cached = this.fileCache.get(this.selectedRelPath);
        const totalLines = cached && cached.lines ? cached.lines.length : 0;
        if (totalLines > 0) {
          for (let i = 0; i < this.lod2HitRegions.length; i += 1) {
            const reg = this.lod2HitRegions[i];
            if (reg.node.relPath === this.selectedRelPath) {
              const lineTop = (this.selectedLine - 1) * reg.lineH;
              const curScroll = this.scrollOffsets.get(this.selectedRelPath) || 0;
              if (lineTop < curScroll || lineTop + reg.lineH > curScroll + reg.bodyH) {
                const maxScroll = Math.max(0, totalLines * reg.lineH - reg.bodyH);
                const nextScroll = clamp(lineTop - reg.bodyH * 0.4, 0, maxScroll);
                this.scrollOffsets.set(this.selectedRelPath, nextScroll);
              }
              break;
            }
          }
        }
      }

      this.rebuildFlagsBuffer(false);
    }

    getCameraState() {
      return {
        centerX: this.curCenterX,
        centerY: this.curCenterY,
        zoom: this.curZoom,
        yaw: this.curYaw,
        pitch: this.curPitch,
        is3D: this.is3D
      };
    }

    setCameraState(state, animate = true) {
      if (!state || typeof state !== "object") return;
      const nextIs3D = typeof state.is3D === "boolean" ? state.is3D : this.is3D;
      this.is3D = nextIs3D;
      const yaw = Number.isFinite(state.yaw) ? state.yaw : (nextIs3D ? this.last3DYaw : 0.0);
      const pitch = Number.isFinite(state.pitch) ? clamp(state.pitch, 0.0, 1.38) : (nextIs3D ? this.last3DPitch : 0.0);
      if (nextIs3D && pitch > 0.05) {
        this.last3DPitch = pitch;
        this.last3DYaw = yaw;
      }
      const cx = Number.isFinite(state.centerX) ? state.centerX : this.tgtCenterX;
      const cy = Number.isFinite(state.centerY) ? state.centerY : this.tgtCenterY;
      const zoom = Number.isFinite(state.zoom) ? clamp(state.zoom, MIN_ZOOM, MAX_ZOOM) : this.tgtZoom;
      const extrude = nextIs3D ? 1.0 : 0.0;
      const cz = nextIs3D ? this.computeFocusCenterZ(cx, cy, zoom, extrude) : 0.0;

      const target = {
        centerX: cx,
        centerY: cy,
        centerZ: cz,
        zoom: zoom,
        yaw: yaw,
        pitch: pitch,
        extrude: extrude
      };
      if (animate) {
        this.startTween(target, 380);
      } else {
        this.snapToTarget(target);
      }
    }

    resize() {
      const targetEl = this.glCanvas || this.overlayCanvas;
      if (!targetEl) return;
      const rect = targetEl.getBoundingClientRect();
      const w = Math.max(1, Math.floor(rect.width || targetEl.clientWidth || 800));
      const h = Math.max(1, Math.floor(rect.height || targetEl.clientHeight || 600));
      const dpr = Math.min(window.devicePixelRatio || 1, 2.5);

      this.vpW = w;
      this.vpH = h;
      this.dpr = dpr;

      const bw = Math.max(1, Math.round(w * dpr));
      const bh = Math.max(1, Math.round(h * dpr));

      if (this.glCanvas) {
        if (this.glCanvas.width !== bw || this.glCanvas.height !== bh) {
          this.glCanvas.width = bw;
          this.glCanvas.height = bh;
        }
      }
      if (this.overlayCanvas) {
        if (this.overlayCanvas.width !== bw || this.overlayCanvas.height !== bh) {
          this.overlayCanvas.width = bw;
          this.overlayCanvas.height = bh;
        }
      }
      if (this.glReady && this.gl) {
        this.gl.viewport(0, 0, bw, bh);
      }
      this.invalidateSurroundCache();
      this.requestRender();
    }

    destroy() {
      this.destroyed = true;
      if (this.rafId) {
        cancelAnimationFrame(this.rafId);
        this.rafId = 0;
      }
      if (this._eventTargets) {
        for (let i = 0; i < this._eventTargets.length; i += 1) {
          const el = this._eventTargets[i];
          el.removeEventListener("pointerdown", this._onPointerDown);
          el.removeEventListener("pointermove", this._onPointerMove);
          el.removeEventListener("wheel", this._onWheel);
          el.removeEventListener("dblclick", this._onDblClick);
          el.removeEventListener("contextmenu", this._onContextMenu);
        }
      }
      window.removeEventListener("pointermove", this._onPointerMove);
      window.removeEventListener("pointerup", this._onPointerUp);
      window.removeEventListener("pointercancel", this._onPointerUp);

      if (this.glReady && this.gl) {
        const gl = this.gl;
        if (this.gridVbo) gl.deleteBuffer(this.gridVbo);
        if (this.cubeVbo) gl.deleteBuffer(this.cubeVbo);
        if (this.staticInstanceVbo) gl.deleteBuffer(this.staticInstanceVbo);
        if (this.flagsInstanceVbo) gl.deleteBuffer(this.flagsInstanceVbo);
        if (this.gridVao) gl.deleteVertexArray(this.gridVao);
        if (this.blockVao) gl.deleteVertexArray(this.blockVao);
        if (this.gridProgram) gl.deleteProgram(this.gridProgram);
        if (this.blockProgram) gl.deleteProgram(this.blockProgram);
      }
      if (this.glCanvas) {
        this.glCanvas.style.cursor = "";
      }
      this.surroundMidCanvas = null;
      this.surroundMidCtx = null;
      this.surroundLowCanvas = null;
      this.surroundLowCtx = null;
      this.fileCache.clear();
      this.pendingFiles.clear();
    }

    prefersReducedMotion() {
      return Boolean(this.motionQuery && this.motionQuery.matches);
    }

    startTween(target, durationMs) {
      if (this.prefersReducedMotion() || durationMs <= 0) {
        this.snapToTarget(target);
        return;
      }
      this.tgtCenterX = target.centerX;
      this.tgtCenterY = target.centerY;
      this.tgtCenterZ = target.centerZ;
      this.tgtZoom = target.zoom;
      this.tgtYaw = target.yaw;
      this.tgtPitch = target.pitch;
      this.tgtExtrude = target.extrude;

      this.tween = {
        startTime: performance.now(),
        duration: durationMs,
        from: {
          centerX: this.curCenterX,
          centerY: this.curCenterY,
          centerZ: this.curCenterZ,
          zoom: this.curZoom,
          yaw: this.curYaw,
          pitch: this.curPitch,
          extrude: this.curExtrude
        },
        to: target
      };
      this.requestRender();
    }

    snapToTarget(target) {
      this.tween = null;
      this.curCenterX = this.tgtCenterX = target.centerX;
      this.curCenterY = this.tgtCenterY = target.centerY;
      this.curCenterZ = this.tgtCenterZ = target.centerZ;
      this.curZoom = this.tgtZoom = target.zoom;
      this.curYaw = this.tgtYaw = target.yaw;
      this.curPitch = this.tgtPitch = target.pitch;
      this.curExtrude = this.tgtExtrude = target.extrude;
      this.requestRender();
    }

    computeFocusCenterZ(wx, wy, zoom, extrude) {
      if (extrude <= 0.01 || this.nodes.length === 0) return 0.0;
      const scaleFactor = clamp((zoom - 1.5) / 10.0, 0.0, 1.0);
      if (scaleFactor <= 0.001) return 0.0;
      let deepestIdx = -1;
      let deepestDepth = -1;
      const stack = [0];
      while (stack.length > 0) {
        const idx = stack.pop();
        const n = this.nodes[idx];
        const b = n.bounds;
        if (!b) continue;
        if (wx >= b.x && wx <= b.x + b.w && wy >= b.y && wy <= b.y + b.h) {
          if (n.depth >= deepestDepth) {
            deepestDepth = n.depth;
            deepestIdx = idx;
          }
          const ch = this.childrenByIdx[idx];
          for (let k = 0; k < ch.length; k += 1) {
            stack.push(ch[k]);
          }
        }
      }
      if (deepestIdx < 0) return 0.0;
      const topZ = (this.effZ[deepestIdx] + this.effH[deepestIdx]) * ELEV_SCALE;
      return topZ * extrude * scaleFactor;
    }

    buildCameraMatrices(cx, cy, cz, zoom, yaw, pitch) {
      const vpW = Math.max(1, this.vpW);
      const vpH = Math.max(1, this.vpH);
      const tanHalfFov = Math.tan(FOV_Y * 0.5);
      const cameraDist = (vpH * 0.5) / (Math.max(zoom, MIN_ZOOM) * tanHalfFov);
      const aspect = vpW / vpH;
      const near = Math.max(0.08, cameraDist * 0.02);
      const far = Math.max(cameraDist * 16.0 + 4000.0, 6000.0);

      const cY = Math.cos(yaw);
      const sY = Math.sin(yaw);
      const cP = Math.cos(pitch);
      const sP = Math.sin(pitch);

      const V = new Float32Array([
        cY, cP * sY, -sP * sY, 0,
        sY, -cP * cY, sP * cY, 0,
        0, sP, cP, 0,
        -cY * cx - sY * cy,
        -cP * sY * cx + cP * cY * cy - sP * cz,
        sP * sY * cx - sP * cY * cy - cP * cz - cameraDist,
        1
      ]);

      const f = 1.0 / tanHalfFov;
      const nf = 1.0 / (near - far);
      const P = new Float32Array([
        f / aspect, 0, 0, 0,
        0, f, 0, 0,
        0, 0, (far + near) * nf, -1,
        0, 0, 2 * far * near * nf, 0
      ]);

      const viewProj = multiplyMat4(P, V);
      const invViewProj = invertMat4(viewProj);
      return { viewProj: viewProj, invViewProj: invViewProj, cameraDist: cameraDist };
    }

    projectPoint(vp, wx, wy, wz) {
      const cx = vp[0] * wx + vp[4] * wy + vp[8] * wz + vp[12];
      const cy = vp[1] * wx + vp[5] * wy + vp[9] * wz + vp[13];
      const cw = vp[3] * wx + vp[7] * wy + vp[11] * wz + vp[15];
      if (cw <= 0.0005) return null;
      const ndcX = cx / cw;
      const ndcY = cy / cw;
      return {
        x: (ndcX * 0.5 + 0.5) * this.vpW,
        y: (0.5 - ndcY * 0.5) * this.vpH,
        w: cw
      };
    }

    screenToLocalXY(clientX, clientY) {
      const el = this.glCanvas || this.overlayCanvas;
      const rect = el.getBoundingClientRect();
      return {
        x: clientX - rect.left,
        y: clientY - rect.top
      };
    }

    screenRayToPlane(sx, sy, planeZ, invVP) {
      const inv = invVP || this.lastInvViewProj;
      if (!inv) {
        return {
          x: this.curCenterX + (sx - this.vpW * 0.5) / this.curZoom,
          y: this.curCenterY + (sy - this.vpH * 0.5) / this.curZoom
        };
      }
      const ndcX = (sx / Math.max(1, this.vpW)) * 2.0 - 1.0;
      const ndcY = 1.0 - (sy / Math.max(1, this.vpH)) * 2.0;
      const nearPt = unprojectNDC(inv, ndcX, ndcY, -1.0);
      const farPt = unprojectNDC(inv, ndcX, ndcY, 1.0);
      const dz = farPt[2] - nearPt[2];
      if (Math.abs(dz) < 1e-6) {
        return { x: nearPt[0], y: nearPt[1] };
      }
      const t = (planeZ - nearPt[2]) / dz;
      return {
        x: nearPt[0] + (farPt[0] - nearPt[0]) * t,
        y: nearPt[1] + (farPt[1] - nearPt[1]) * t
      };
    }

    findLod2RegionAt(sx, sy) {
      for (let i = this.lod2HitRegions.length - 1; i >= 0; i -= 1) {
        const r = this.lod2HitRegions[i];
        if (r.isScreenAligned) {
          if (sx >= r.cardX && sx <= r.cardX + r.cardW && sy >= r.cardY && sy <= r.cardY + r.cardH) {
            return { region: r, lx: sx - r.cardX, ly: sy - r.cardY };
          }
        } else if (r.invTransform) {
          const m = r.invTransform;
          const dx = sx - r.e;
          const dy = sy - r.f;
          const lx = m[0] * dx + m[1] * dy;
          const ly = m[2] * dx + m[3] * dy;
          if (lx >= 0 && lx <= r.localW && ly >= 0 && ly <= r.localH) {
            return { region: r, lx: lx, ly: ly };
          }
        }
      }
      return null;
    }

    pickAt(sx, sy, includeLine) {
      if (includeLine) {
        const lodHit = this.findLod2RegionAt(sx, sy);
        if (lodHit) {
          const r = lodHit.region;
          let hitLine = null;
          if (lodHit.ly >= r.headerH && r.totalLines > 0) {
            const relY = lodHit.ly - r.headerH + r.scrollPx;
            const lineIdx = Math.floor(relY / r.lineH) + 1;
            if (lineIdx >= 1 && lineIdx <= r.totalLines) {
              hitLine = lineIdx;
            }
          }
          return { node: r.node, line: hitLine };
        }
      }

      if (this.nodes.length === 0 || !this.lastInvViewProj) {
        return { node: null, line: null };
      }

      const ndcX = (sx / Math.max(1, this.vpW)) * 2.0 - 1.0;
      const ndcY = 1.0 - (sy / Math.max(1, this.vpH)) * 2.0;
      const nearPt = unprojectNDC(this.lastInvViewProj, ndcX, ndcY, -1.0);
      const farPt = unprojectNDC(this.lastInvViewProj, ndcX, ndcY, 1.0);
      const ox = nearPt[0];
      const oy = nearPt[1];
      const oz = nearPt[2];
      const dx = farPt[0] - ox;
      const dy = farPt[1] - oy;
      const dz = farPt[2] - oz;
      const invDx = Math.abs(dx) > 1e-9 ? 1.0 / dx : 1e9;
      const invDy = Math.abs(dy) > 1e-9 ? 1.0 / dy : 1e9;
      const invDz = Math.abs(dz) > 1e-9 ? 1.0 / dz : 1e9;

      const mix = this.curExtrude;
      let bestIdx = -1;
      let bestT = Infinity;
      let bestDepth = -1;

      const stack = [0];
      while (stack.length > 0) {
        const idx = stack.pop();
        const n = this.nodes[idx];
        const b = n.bounds;
        if (!b) continue;

        const maxSubZ = mix > 0.002 ? this.subtreeMaxTopZ[idx] * ELEV_SCALE * mix : 0.5;
        const subHit = intersectRayAABB(ox, oy, oz, invDx, invDy, invDz, b.x, b.y, -0.1, b.x + b.w, b.y + b.h, maxSubZ);
        if (subHit < 0 || subHit > bestT + 0.5) continue;

        const minZ = mix > 0.002 ? this.effZ[idx] * ELEV_SCALE * mix : n.depth * 0.002;
        const h = mix > 0.002 ? Math.max(this.effH[idx] * ELEV_SCALE * mix, 0.05) : 0.001;
        const t = intersectRayAABB(ox, oy, oz, invDx, invDy, invDz, b.x, b.y, minZ, b.x + b.w, b.y + b.h, minZ + h);
        if (t >= 0) {
          if (t < bestT - 1e-4 || (Math.abs(t - bestT) <= 1e-4 && n.depth > bestDepth)) {
            bestT = t;
            bestIdx = idx;
            bestDepth = n.depth;
          }
        }

        const ch = this.childrenByIdx[idx];
        for (let k = 0; k < ch.length; k += 1) {
          stack.push(ch[k]);
        }
      }

      if (bestIdx >= 0) {
        return { node: this.nodes[bestIdx], line: null };
      }
      return { node: null, line: null };
    }

    handlePointerDown(e) {
      const pos = this.screenToLocalXY(e.clientX, e.clientY);
      const isOrbit = e.button === 2 || (e.button === 0 && e.shiftKey);

      if (e.button === 0 && !e.shiftKey) {
        const lodHit = this.findLod2RegionAt(pos.x, pos.y);
        if (lodHit) {
          const r = lodHit.region;
          const maxScroll = Math.max(0, r.totalLines * r.lineH - r.bodyH);
          if (maxScroll > 0 && lodHit.lx >= r.localW - 14 && lodHit.ly >= r.headerH) {
            this.dragState = {
              mode: "scrollbar",
              region: r,
              maxScroll: maxScroll,
              startY: pos.y,
              startScroll: r.scrollPx,
              didDrag: true
            };
            this.tween = null;
            return;
          }
        }
      }

      if (e.button !== 0 && e.button !== 2) return;
      this.tween = null;
      this.dragState = {
        mode: isOrbit ? "orbit" : "pan",
        startX: pos.x,
        startY: pos.y,
        lastX: pos.x,
        lastY: pos.y,
        didDrag: false,
        pointerId: e.pointerId
      };
    }

    handlePointerMove(e) {
      const pos = this.screenToLocalXY(e.clientX, e.clientY);

      if (this.dragState) {
        const ds = this.dragState;
        if (ds.mode === "scrollbar") {
          const dy = pos.y - ds.startY;
          const trackH = Math.max(20, ds.region.bodyH - 8);
          const ratio = (ds.region.totalLines * ds.region.lineH) / trackH;
          const nextScroll = clamp(ds.startScroll + dy * ratio, 0, ds.maxScroll);
          this.scrollOffsets.set(ds.region.node.relPath, nextScroll);
          if (this.isFileFocused() && this.curFocusMix >= 0.99) {
            this.requestOverlayRender();
          } else {
            this.requestRender();
          }
          return;
        }

        const dx = pos.x - ds.lastX;
        const dy = pos.y - ds.lastY;
        const totalDist = Math.hypot(pos.x - ds.startX, pos.y - ds.startY);
        if (totalDist > 4) ds.didDrag = true;

        if (ds.didDrag) {
          if (ds.mode === "orbit") {
            if (!this.is3D) {
              this.is3D = true;
              this.tgtExtrude = 1.0;
              if (this.tgtPitch < 0.05) {
                this.tgtPitch = 0.25;
                this.curPitch = 0.25;
              }
              if (this.callbacks && typeof this.callbacks.onModeChange === "function") {
                this.callbacks.onModeChange(true);
              }
            }
            this.tgtYaw += dx * 0.0065;
            this.tgtPitch = clamp(this.tgtPitch + dy * 0.0055, 0.06, 1.38);
            this.last3DYaw = this.tgtYaw;
            this.last3DPitch = this.tgtPitch;
            this.requestRender();
          } else if (ds.mode === "pan") {
            const mats = this.buildCameraMatrices(
              this.tgtCenterX,
              this.tgtCenterY,
              this.tgtCenterZ,
              this.tgtZoom,
              this.tgtYaw,
              this.tgtPitch
            );
            const p0 = this.screenRayToPlane(ds.lastX, ds.lastY, this.tgtCenterZ, mats.invViewProj);
            const p1 = this.screenRayToPlane(pos.x, pos.y, this.tgtCenterZ, mats.invViewProj);
            const stepX = clamp(p0.x - p1.x, -400, 400);
            const stepY = clamp(p0.y - p1.y, -400, 400);
            this.tgtCenterX += stepX;
            this.tgtCenterY += stepY;
            this.tgtCenterZ = this.is3D ? this.computeFocusCenterZ(this.tgtCenterX, this.tgtCenterY, this.tgtZoom, this.tgtExtrude) : 0.0;
            this.requestRender();
          }
        }
        ds.lastX = pos.x;
        ds.lastY = pos.y;
        return;
      }

      if (pos.x < 0 || pos.y < 0 || pos.x > this.vpW || pos.y > this.vpH) {
        if (this.glCanvas) this.glCanvas.style.cursor = "";
        if (this.hoveredRelPath !== null || this.hoveredLine !== null) {
          this.hoveredRelPath = null;
          this.hoveredLine = null;
          this.rebuildFlagsBuffer(false);
          if (this.callbacks && typeof this.callbacks.onHoverNode === "function") {
            this.callbacks.onHoverNode(null, null);
          }
        }
        return;
      }

      if (this.isFileFocused()) {
        const lodHit = this.findLod2RegionAt(pos.x, pos.y);
        let nextPath = null;
        let nextLine = null;
        let nextNode = null;
        let nextCursor = "pointer";
        if (lodHit) {
          const r = lodHit.region;
          nextNode = r.node;
          nextPath = r.node ? r.node.relPath : null;
          if (r.unfocusRect &&
              lodHit.lx >= r.unfocusRect.x &&
              lodHit.lx <= r.unfocusRect.x + r.unfocusRect.w &&
              lodHit.ly >= r.unfocusRect.y &&
              lodHit.ly <= r.unfocusRect.y + r.unfocusRect.h) {
            nextCursor = "pointer";
          } else if (lodHit.ly >= r.headerH) {
            nextCursor = "text";
            if (r.totalLines > 0) {
              const relY = lodHit.ly - r.headerH + r.scrollPx;
              const lineIdx = Math.floor(relY / r.lineH) + 1;
              if (lineIdx >= 1 && lineIdx <= r.totalLines) {
                nextLine = lineIdx;
              }
            }
          } else {
            nextCursor = "default";
          }
        }
        if (this.glCanvas && this.glCanvas.style.cursor !== nextCursor) {
          this.glCanvas.style.cursor = nextCursor;
        }
        if (nextPath !== this.hoveredRelPath || nextLine !== this.hoveredLine) {
          const samePath = nextPath === this.hoveredRelPath;
          this.hoveredRelPath = nextPath;
          this.hoveredLine = nextLine;
          if (this.isFileFocused() && this.curFocusMix >= 0.99) {
            this.requestOverlayRender();
          } else if (samePath) {
            this.requestRender();
          } else {
            this.rebuildFlagsBuffer(false);
          }
          if (this.callbacks && typeof this.callbacks.onHoverNode === "function") {
            this.callbacks.onHoverNode(nextNode, nextLine);
          }
        }
        return;
      }

      if (this.glCanvas && this.glCanvas.style.cursor !== "") {
        this.glCanvas.style.cursor = "";
      }

      const hit = this.pickAt(pos.x, pos.y, true);
      const nextPath = hit.node ? hit.node.relPath : null;
      const nextLine = hit.line;
      if (nextPath !== this.hoveredRelPath || nextLine !== this.hoveredLine) {
        this.hoveredRelPath = nextPath;
        this.hoveredLine = nextLine;
        this.rebuildFlagsBuffer(false);
        if (this.callbacks && typeof this.callbacks.onHoverNode === "function") {
          this.callbacks.onHoverNode(hit.node, nextLine);
        }
      }
    }

    handlePointerUp(e) {
      if (!this.dragState) return;
      const ds = this.dragState;
      this.dragState = null;

      if (!ds.didDrag && ds.mode === "pan" && e.button === 0) {
        const pos = this.screenToLocalXY(e.clientX, e.clientY);
        const lodHit = this.findLod2RegionAt(pos.x, pos.y);
        if (lodHit) {
          const r = lodHit.region;
          if (r.unfocusRect &&
              lodHit.lx >= r.unfocusRect.x &&
              lodHit.lx <= r.unfocusRect.x + r.unfocusRect.w &&
              lodHit.ly >= r.unfocusRect.y &&
              lodHit.ly <= r.unfocusRect.y + r.unfocusRect.h) {
            this.clearFileFocus(true);
            return;
          }

          let hitLine = null;
          if (lodHit.ly >= r.headerH && r.totalLines > 0) {
            const relY = lodHit.ly - r.headerH + r.scrollPx;
            const lineIdx = Math.floor(relY / r.lineH) + 1;
            if (lineIdx >= 1 && lineIdx <= r.totalLines) {
              hitLine = lineIdx;
            }
          }

          if (r.node && !r.node.isDir) {
            if (!this.isFileFocused() || this.focusedFileRelPath !== r.node.relPath) {
              this.focusFile(r.node.relPath, hitLine, true);
            } else {
              const sameSelectedPath = this.selectedRelPath === r.node.relPath;
              this.selectedRelPath = r.node.relPath;
              this.selectedLine = hitLine;
              if (sameSelectedPath && this.curFocusMix >= 0.99) {
                this.requestOverlayRender();
              } else {
                this.rebuildFlagsBuffer(false);
              }
            }
            if (this.callbacks && typeof this.callbacks.onSelectNode === "function") {
              this.callbacks.onSelectNode(r.node, hitLine, true);
            }
            return;
          }
        }

        if (this.isFileFocused()) {
          this.clearFileFocus(true);
          return;
        }

        const hit = this.pickAt(pos.x, pos.y, true);
        if (hit.node && !hit.node.isDir) {
          this.focusFile(hit.node.relPath, hit.line, true);
        } else {
          this.clearFileFocus(false);
          const nextPath = hit.node ? hit.node.relPath : null;
          this.selectedRelPath = nextPath;
          this.selectedLine = hit.line;
          this.rebuildFlagsBuffer(false);
        }
        if (this.callbacks && typeof this.callbacks.onSelectNode === "function") {
          this.callbacks.onSelectNode(hit.node, hit.line, true);
        }
      }
    }

    handleDblClick(e) {
      const pos = this.screenToLocalXY(e.clientX, e.clientY);
      if (this.isFileFocused()) {
        const lodHit = this.findLod2RegionAt(pos.x, pos.y);
        if (!lodHit) {
          this.clearFileFocus(true);
        }
        return;
      }
      const hit = this.pickAt(pos.x, pos.y, false);
      if (hit.node) {
        if (!hit.node.isDir) {
          this.focusFile(hit.node.relPath, null, true);
        } else {
          this.clearFileFocus(false);
          this.selectedRelPath = hit.node.relPath;
          this.selectedLine = null;
          this.rebuildFlagsBuffer(false);
          this.flyToNode(hit.node.relPath, null, true);
        }
        if (this.callbacks && typeof this.callbacks.onSelectNode === "function") {
          this.callbacks.onSelectNode(hit.node, null, true);
        }
      } else {
        this.fitView(true);
      }
    }

    handleWheel(e) {
      e.preventDefault();
      const pos = this.screenToLocalXY(e.clientX, e.clientY);
      const deltaY = e.deltaMode === 1 ? e.deltaY * 18 : (e.deltaMode === 2 ? e.deltaY * 240 : e.deltaY);

      if (this.isFileFocused() && !e.ctrlKey && !e.metaKey) {
        const lodHit = this.findLod2RegionAt(pos.x, pos.y);
        if (lodHit) {
          const r = lodHit.region;
          const maxScroll = Math.max(0, r.totalLines * r.lineH - r.bodyH);
          if (maxScroll > 0) {
            const curScroll = this.scrollOffsets.get(r.node.relPath) || 0;
            const nextScroll = clamp(curScroll + deltaY * 0.85, 0, maxScroll);
            this.scrollOffsets.set(r.node.relPath, nextScroll);
            if (this.curFocusMix >= 0.99) {
              this.requestOverlayRender();
            } else {
              this.requestRender();
            }
          }
          return;
        }
      }

      if (e.altKey) {
        const lodHit = this.findLod2RegionAt(pos.x, pos.y);
        if (lodHit) {
          const r = lodHit.region;
          const maxScroll = Math.max(0, r.totalLines * r.lineH - r.bodyH);
          if (maxScroll > 0) {
            const curScroll = this.scrollOffsets.get(r.node.relPath) || 0;
            const nextScroll = clamp(curScroll + deltaY * 0.85, 0, maxScroll);
            this.scrollOffsets.set(r.node.relPath, nextScroll);
            this.requestRender();
            return;
          }
        }
      }

      this.tween = null;
      const matsBefore = this.buildCameraMatrices(
        this.tgtCenterX,
        this.tgtCenterY,
        this.tgtCenterZ,
        this.tgtZoom,
        this.tgtYaw,
        this.tgtPitch
      );
      const worldUnder = this.screenRayToPlane(pos.x, pos.y, this.tgtCenterZ, matsBefore.invViewProj);

      const scale = Math.exp(-clamp(deltaY, -300, 300) * 0.0014);
      const nextZoom = clamp(this.tgtZoom * scale, MIN_ZOOM, MAX_ZOOM);

      const matsAfter = this.buildCameraMatrices(
        this.tgtCenterX,
        this.tgtCenterY,
        this.tgtCenterZ,
        nextZoom,
        this.tgtYaw,
        this.tgtPitch
      );
      const worldAfter = this.screenRayToPlane(pos.x, pos.y, this.tgtCenterZ, matsAfter.invViewProj);

      this.tgtCenterX += worldUnder.x - worldAfter.x;
      this.tgtCenterY += worldUnder.y - worldAfter.y;
      this.tgtZoom = nextZoom;
      this.tgtCenterZ = this.is3D ? this.computeFocusCenterZ(this.tgtCenterX, this.tgtCenterY, this.tgtZoom, this.tgtExtrude) : 0.0;
      this.requestRender();
    }

    ensureFileRequested(relPath) {
      if (!relPath) return;
      if (this.fileCache.has(relPath)) {
        const existing = this.fileCache.get(relPath);
        this.fileCache.delete(relPath);
        this.fileCache.set(relPath, existing);
        return;
      }
      if (this.pendingFiles.has(relPath)) return;
      if (!this.callbacks || typeof this.callbacks.onRequestFile !== "function") return;

      this.pendingFiles.add(relPath);
      Promise.resolve(this.callbacks.onRequestFile(relPath))
        .then((fileData) => {
          this.pendingFiles.delete(relPath);
          if (this.destroyed || !fileData) return;
          if (this.fileCache.size >= LRU_CAPACITY) {
            const oldestKey = this.fileCache.keys().next().value;
            this.fileCache.delete(oldestKey);
          }
          this.fileCache.set(relPath, {
            language: fileData.language || "Text",
            lines: Array.isArray(fileData.lines) ? fileData.lines : [],
            tokenized: new Map()
          });
          if (this.isFileFocused() && this.curFocusMix >= 0.99) {
            this.requestOverlayRender();
          } else {
            this.requestRender();
          }
        })
        .catch(() => {
          this.pendingFiles.delete(relPath);
        });
    }

    requestRender() {
      this.needsRender = true;
      this.needsWebGL = true;
    }

    requestOverlayRender() {
      this.needsRender = true;
    }

    startLoop() {
      const tick = (now) => {
        if (this.destroyed) return;
        const dt = Math.min(0.05, Math.max(0.001, (now - this.lastFrameTime) / 1000));
        this.lastFrameTime = now;

        let animating = false;
        let camSettled = false;
        if (this.tween) {
          const t = (now - this.tween.startTime) / Math.max(1, this.tween.duration);
          const ease = easeOutCubic(t);
          const f = this.tween.from;
          const to = this.tween.to;
          this.curCenterX = lerp(f.centerX, to.centerX, ease);
          this.curCenterY = lerp(f.centerY, to.centerY, ease);
          this.curCenterZ = lerp(f.centerZ, to.centerZ, ease);
          this.curZoom = lerp(f.zoom, to.zoom, ease);
          this.curYaw = lerp(f.yaw, to.yaw, ease);
          this.curPitch = lerp(f.pitch, to.pitch, ease);
          this.curExtrude = lerp(f.extrude, to.extrude, ease);
          if (t >= 1) {
            this.tween = null;
            camSettled = true;
          } else {
            animating = true;
          }
          this.needsRender = true;
        } else if (this.prefersReducedMotion()) {
          const moved =
            this.curCenterX !== this.tgtCenterX ||
            this.curCenterY !== this.tgtCenterY ||
            this.curCenterZ !== this.tgtCenterZ ||
            this.curZoom !== this.tgtZoom ||
            this.curYaw !== this.tgtYaw ||
            this.curPitch !== this.tgtPitch ||
            this.curExtrude !== this.tgtExtrude;
          this.curCenterX = this.tgtCenterX;
          this.curCenterY = this.tgtCenterY;
          this.curCenterZ = this.tgtCenterZ;
          this.curZoom = this.tgtZoom;
          this.curYaw = this.tgtYaw;
          this.curPitch = this.tgtPitch;
          this.curExtrude = this.tgtExtrude;
          if (moved) {
            camSettled = true;
            this.needsRender = true;
          }
        } else {
          const k = 1 - Math.exp(-dt * 18);
          const diff =
            Math.abs(this.curCenterX - this.tgtCenterX) +
            Math.abs(this.curCenterY - this.tgtCenterY) +
            Math.abs(this.curCenterZ - this.tgtCenterZ) +
            Math.abs(this.curZoom - this.tgtZoom) / Math.max(1, this.curZoom) +
            Math.abs(this.curYaw - this.tgtYaw) +
            Math.abs(this.curPitch - this.tgtPitch) +
            Math.abs(this.curExtrude - this.tgtExtrude);
          if (diff > 1e-4) {
            this.curCenterX = lerp(this.curCenterX, this.tgtCenterX, k);
            this.curCenterY = lerp(this.curCenterY, this.tgtCenterY, k);
            this.curCenterZ = lerp(this.curCenterZ, this.tgtCenterZ, k);
            this.curZoom = lerp(this.curZoom, this.tgtZoom, k);
            this.curYaw = lerp(this.curYaw, this.tgtYaw, k);
            this.curPitch = lerp(this.curPitch, this.tgtPitch, k);
            this.curExtrude = lerp(this.curExtrude, this.tgtExtrude, k);
            animating = true;
            this.needsRender = true;
          } else {
            if (diff > 0) camSettled = true;
            this.curCenterX = this.tgtCenterX;
            this.curCenterY = this.tgtCenterY;
            this.curCenterZ = this.tgtCenterZ;
            this.curZoom = this.tgtZoom;
            this.curYaw = this.tgtYaw;
            this.curPitch = this.tgtPitch;
            this.curExtrude = this.tgtExtrude;
          }
        }

        const prevFocusMix = this.curFocusMix;
        let focusAnimating = false;
        const fk = 1 - Math.exp(-dt * 15);
        if (!this.prefersReducedMotion() && Math.abs(this.curFocusMix - this.tgtFocusMix) > 1e-3) {
          this.curFocusMix = lerp(this.curFocusMix, this.tgtFocusMix, fk);
          focusAnimating = true;
        } else {
          this.curFocusMix = this.tgtFocusMix;
          if (this.curFocusMix <= 0.001) {
            this.curFocusMix = 0.0;
            if (!this.focusedFileRelPath) {
              this.lastFocusedRelPath = null;
            }
          }
        }

        const focusChanged = prevFocusMix !== this.curFocusMix;
        const anyAnimating = animating || focusAnimating;
        if (anyAnimating || camSettled || focusChanged || this.hasSearch || this.dirtyFlags) {
          this.needsRender = true;
          this.needsWebGL = true;
        }

        if (this.needsRender) {
          this.renderFrame(now / 1000);
          this.needsRender = anyAnimating || this.hasSearch;
        }

        this.rafId = requestAnimationFrame(tick);
      };
      this.rafId = requestAnimationFrame(tick);
    }

    renderFrame(timeSec) {
      const mats = this.buildCameraMatrices(
        this.curCenterX,
        this.curCenterY,
        this.curCenterZ,
        this.curZoom,
        this.curYaw,
        this.curPitch
      );
      this.lastViewProj = mats.viewProj;
      this.lastInvViewProj = mats.invViewProj;

      if (this.needsWebGL || this.dirtyFlags || this.hasSearch) {
        this.renderWebGL(mats.viewProj, timeSec);
        this.needsWebGL = false;
      }
      this.renderOverlay(mats.viewProj);
    }

    renderWebGL(viewProj, timeSec) {
      if (!this.glReady || !this.gl) return;
      const gl = this.gl;
      gl.clearColor(0.0667, 0.0745, 0.0902, 1.0);
      gl.clear(gl.COLOR_BUFFER_BIT | gl.DEPTH_BUFFER_BIT);

      gl.disable(gl.CULL_FACE);
      gl.disable(gl.DEPTH_TEST);
      gl.depthMask(false);

      gl.useProgram(this.gridProgram);
      gl.bindVertexArray(this.gridVao);
      gl.uniformMatrix4fv(this.gridUniforms.uViewProj, false, viewProj);
      gl.uniform1f(this.gridUniforms.uExtrudeMix, this.curExtrude);
      gl.uniform1f(this.gridUniforms.uFocusMix, this.curFocusMix || 0.0);
      gl.drawArrays(gl.TRIANGLE_STRIP, 0, 4);

      if (this.nodes.length === 0) return;

      gl.depthMask(true);
      gl.enable(gl.DEPTH_TEST);
      gl.depthFunc(gl.LEQUAL);

      gl.useProgram(this.blockProgram);
      gl.bindVertexArray(this.blockVao);
      gl.uniformMatrix4fv(this.blockUniforms.uViewProj, false, viewProj);
      gl.uniform1f(this.blockUniforms.uExtrudeMix, this.curExtrude);
      gl.uniform1f(this.blockUniforms.uElevScale, ELEV_SCALE);
      gl.uniform1f(this.blockUniforms.uTime, timeSec);
      gl.uniform1f(this.blockUniforms.uHasSearch, this.hasSearch ? 1.0 : 0.0);
      gl.uniform1f(this.blockUniforms.uFocusMix, this.curFocusMix || 0.0);

      gl.drawArraysInstanced(gl.TRIANGLES, 0, 36, this.nodes.length);
      gl.bindVertexArray(null);
    }

    buildNodeScreenItem(viewProj, n) {
      if (!n || !n.bounds) return null;
      const b = n.bounds;
      const idx = this.nodesById.get(n.id);
      const mix = this.curExtrude;
      const topZ = (idx !== undefined && mix > 0.002) ? (this.effZ[idx] + this.effH[idx]) * ELEV_SCALE * mix : n.depth * 0.002 + 0.001;
      const p0 = this.projectPoint(viewProj, b.x, b.y, topZ);
      const p1 = this.projectPoint(viewProj, b.x + b.w, b.y, topZ);
      const p2 = this.projectPoint(viewProj, b.x + b.w, b.y + b.h, topZ);
      const p3 = this.projectPoint(viewProj, b.x, b.y + b.h, topZ);
      if (!p0 || !p1 || !p2 || !p3) {
        const fw = Math.min(280, this.vpW * 0.35);
        const fh = Math.min(200, this.vpH * 0.35);
        const fx = (this.vpW - fw) * 0.5;
        const fy = (this.vpH - fh) * 0.5;
        return {
          node: n,
          idx: idx !== undefined ? idx : -1,
          p0: { x: fx, y: fy, w: 1 },
          p1: { x: fx + fw, y: fy, w: 1 },
          p2: { x: fx + fw, y: fy + fh, w: 1 },
          p3: { x: fx, y: fy + fh, w: 1 },
          minX: fx,
          minY: fy,
          maxX: fx + fw,
          maxY: fy + fh,
          screenW: fw,
          screenH: fh,
          avgW: 1
        };
      }
      const minX = Math.min(p0.x, p1.x, p2.x, p3.x);
      const maxX = Math.max(p0.x, p1.x, p2.x, p3.x);
      const minY = Math.min(p0.y, p1.y, p2.y, p3.y);
      const maxY = Math.max(p0.y, p1.y, p2.y, p3.y);
      return {
        node: n,
        idx: idx !== undefined ? idx : -1,
        p0: p0,
        p1: p1,
        p2: p2,
        p3: p3,
        minX: minX,
        minY: minY,
        maxX: maxX,
        maxY: maxY,
        screenW: Math.hypot(p1.x - p0.x, p1.y - p0.y),
        screenH: Math.hypot(p3.x - p0.x, p3.y - p0.y),
        avgW: (p0.w + p1.w + p2.w + p3.w) * 0.25
      };
    }

    computeFocusedCardRect(item) {
      const baseMinX = item && Number.isFinite(item.minX) ? item.minX : this.vpW * 0.25;
      const baseMinY = item && Number.isFinite(item.minY) ? item.minY : this.vpH * 0.25;
      const baseRawW = Math.max((item && item.screenW) || 0, ((item && item.maxX) || 0) - baseMinX, 160);
      const baseRawH = Math.max((item && item.screenH) || 0, ((item && item.maxY) || 0) - baseMinY, 110);
      const baseW = Math.min(baseRawW, Math.max(120, this.vpW - 20));
      const baseH = Math.min(baseRawH, Math.max(90, this.vpH - 20));
      const baseX = clamp(baseMinX, 10, Math.max(10, this.vpW - baseW - 10));
      const baseY = clamp(baseMinY, 10, Math.max(10, this.vpH - baseH - 10));

      const maxFocusW = Math.max(280, this.vpW - 36);
      const maxFocusH = Math.max(200, this.vpH - 54);
      const focusW = clamp(Math.max(baseW, this.vpW * 0.76), Math.min(340, maxFocusW), Math.min(maxFocusW, 1020));
      const focusH = clamp(Math.max(baseH, this.vpH * 0.82), Math.min(240, maxFocusH), Math.min(maxFocusH, 820));
      const focusX = Math.round((this.vpW - focusW) * 0.5);
      const focusY = Math.round(clamp((this.vpH - focusH) * 0.5 - 6, 16, Math.max(16, this.vpH - focusH - 36)));

      const mixF = clamp(this.curFocusMix, 0, 1);
      const w = lerp(baseW, focusW, mixF);
      const h = lerp(baseH, focusH, mixF);
      const x = clamp(lerp(baseX, focusX, mixF), 10, Math.max(10, this.vpW - w - 10));
      const y = clamp(lerp(baseY, focusY, mixF), 10, Math.max(10, this.vpH - h - 10));
      return { x: x, y: y, w: w, h: h };
    }

    isRectOccludedByFocus(item, focusRect) {
      if (!focusRect || this.curFocusMix <= 0.85) return false;
      const minX = Number.isFinite(item.minX) ? item.minX : item.p0.x;
      const maxX = Number.isFinite(item.maxX) ? item.maxX : minX + (item.screenW || 0);
      const minY = Number.isFinite(item.minY) ? item.minY : item.p0.y;
      const maxY = Number.isFinite(item.maxY) ? item.maxY : minY + (item.screenH || 0);
      return (
        minX >= focusRect.x &&
        maxX <= focusRect.x + focusRect.w &&
        minY >= focusRect.y &&
        maxY <= focusRect.y + focusRect.h
      );
    }

    drawSurroundRects(targetCtx, sx, sy, lod0Candidates, lod1Queue, lod2Queue, focusCardRect) {
      const files = [];
      for (let i = 0; i < lod1Queue.length; i += 1) {
        if (!this.isRectOccludedByFocus(lod1Queue[i], focusCardRect)) {
          files.push(lod1Queue[i]);
        }
      }
      for (let i = 0; i < lod2Queue.length; i += 1) {
        if (!this.isRectOccludedByFocus(lod2Queue[i], focusCardRect)) {
          files.push(lod2Queue[i]);
        }
      }
      files.sort((a, b) => (b.screenW * b.screenH) - (a.screenW * a.screenH));
      const maxFiles = Math.min(files.length, 120);

      for (let i = 0; i < maxFiles; i += 1) {
        const item = files[i];
        const minX = Number.isFinite(item.minX) ? item.minX : item.p0.x;
        const maxX = Number.isFinite(item.maxX) ? item.maxX : minX + item.screenW;
        const minY = Number.isFinite(item.minY) ? item.minY : item.p0.y;
        const maxY = Number.isFinite(item.maxY) ? item.maxY : minY + item.screenH;
        const rx = minX * sx;
        const ry = minY * sy;
        const rw = Math.max(2, (maxX - minX) * sx);
        const rh = Math.max(2, (maxY - minY) * sy);

        targetCtx.fillStyle = "rgba(21, 24, 30, 0.88)";
        targetCtx.fillRect(rx, ry, rw, rh);

        const hdrH = clamp(rh * 0.22, 2, Math.max(2, CODE_HEADER_HEIGHT * sy));
        targetCtx.fillStyle = "rgba(31, 36, 45, 0.92)";
        targetCtx.fillRect(rx, ry, rw, hdrH);

        const accent = (item.node && item.node.color) || "#6e7681";
        targetCtx.fillStyle = accent;
        targetCtx.fillRect(rx, ry, rw, Math.max(1.25, hdrH * 0.35));

        if (rw >= 6 && rh >= 7) {
          const innerTop = ry + hdrH + Math.max(1, 2 * sy);
          const availH = rh - hdrH - Math.max(2, 4 * sy);
          const barCount = Math.min(4, Math.max(1, Math.floor(availH / Math.max(2, 5 * sy))));
          const barStep = availH / barCount;
          const barH = clamp(barStep * 0.55, 1, 3.5);
          targetCtx.fillStyle = "rgba(182, 178, 169, 0.24)";
          for (let b = 0; b < barCount; b += 1) {
            const frac = 0.32 + (((item.idx * 37 + b * 23) & 63) / 63) * 0.54;
            const bw = Math.max(2, (rw - 4) * frac);
            targetCtx.fillRect(rx + 2, innerTop + b * barStep, bw, barH);
          }
        }

        targetCtx.strokeStyle = "#343944";
        targetCtx.lineWidth = 0.75;
        targetCtx.strokeRect(rx, ry, rw, rh);
      }

      const dirs = [];
      for (let i = 0; i < lod0Candidates.length; i += 1) {
        if (!this.isRectOccludedByFocus(lod0Candidates[i], focusCardRect)) {
          dirs.push(lod0Candidates[i]);
        }
      }
      dirs.sort((a, b) => (b.screenW * b.screenH) - (a.screenW * a.screenH));
      const maxDirs = Math.min(dirs.length, 60);
      const pillH = Math.max(3, 18 * sy);
      for (let i = 0; i < maxDirs; i += 1) {
        const d = dirs[i];
        const px = clamp(d.p0.x + 4, 4, Math.max(4, this.vpW - 40)) * sx;
        const py = clamp(d.p0.y + 4, 4, Math.max(4, this.vpH - 22)) * sy;
        const pw = clamp(Math.min(d.screenW - 4, 180) * sx, 8, 60);
        targetCtx.fillStyle = "rgba(17, 19, 23, 0.90)";
        targetCtx.fillRect(px, py, pw, pillH);
        targetCtx.fillStyle = (d.node && d.node.color) || "#9b968c";
        targetCtx.fillRect(px + Math.max(1, 3 * sx), py + Math.max(0.5, pillH * 0.25), Math.max(2, pillH * 0.5), Math.max(2, pillH * 0.5));
      }
    }

    drawSurroundFast(lod0Candidates, lod1Queue, lod2Queue, focusCardRect) {
      const midW = this.surroundMidCanvas.width;
      const midH = this.surroundMidCanvas.height;
      const lowW = this.surroundLowCanvas.width;
      const lowH = this.surroundLowCanvas.height;
      const midCtx = this.surroundMidCtx;
      const lowCtx = this.surroundLowCtx;
      const sx = midW / Math.max(1, this.vpW);
      const sy = midH / Math.max(1, this.vpH);

      midCtx.setTransform(1, 0, 0, 1, 0, 0);
      midCtx.clearRect(0, 0, midW, midH);
      this.drawSurroundRects(midCtx, sx, sy, lod0Candidates, lod1Queue, lod2Queue, focusCardRect);

      lowCtx.setTransform(1, 0, 0, 1, 0, 0);
      lowCtx.clearRect(0, 0, lowW, lowH);
      lowCtx.imageSmoothingEnabled = true;
      lowCtx.imageSmoothingQuality = "high";
      lowCtx.drawImage(this.surroundMidCanvas, 0, 0, midW, midH, 0, 0, lowW, lowH);
    }

    renderOverlay(viewProj) {
      const ctx = this.ctx2d;
      if (!ctx) return;

      ctx.setTransform(this.dpr, 0, 0, this.dpr, 0, 0);
      ctx.clearRect(0, 0, this.vpW, this.vpH);
      this.lod2HitRegions = [];

      if (this.nodes.length === 0) return;

      const activeFocusPath = this.focusedFileRelPath || (this.curFocusMix > 0.01 ? this.lastFocusedRelPath : null);
      const focusActive = Boolean(activeFocusPath && this.curFocusMix > 0.01);

      const camKey = focusActive ? (
        this.vpW + "|" + this.vpH + "|" +
        this.curCenterX.toFixed(3) + "|" + this.curCenterY.toFixed(3) + "|" + this.curCenterZ.toFixed(3) + "|" +
        this.curZoom.toFixed(5) + "|" + this.curYaw.toFixed(4) + "|" + this.curPitch.toFixed(4) + "|" +
        this.curExtrude.toFixed(4) + "|" + activeFocusPath + "|" +
        this.curFocusMix.toFixed(3) + "|" +
        (this.hasSearch ? this.activeQuery : "") + "|" + (this.selectedRelPath || "")
      ) : "";

      if (focusActive && this._surroundCacheReady && this._surroundCacheKey === camKey) {
        if (this.ensureSurroundPyramid()) {
          ctx.save();
          ctx.globalAlpha = clamp(1.0 - this.curFocusMix * 0.76, 0.16, 1.0);
          ctx.imageSmoothingEnabled = true;
          ctx.drawImage(
            this.surroundLowCanvas,
            0, 0, this.surroundLowCanvas.width, this.surroundLowCanvas.height,
            0, 0, this.vpW, this.vpH
          );
          ctx.restore();
        }

        ctx.save();
        ctx.fillStyle = "rgba(11, 13, 17, " + (0.62 * this.curFocusMix).toFixed(3) + ")";
        ctx.fillRect(0, 0, this.vpW, this.vpH);
        ctx.restore();

        const fNode = this.nodesByPath.get(activeFocusPath);
        if (fNode && !fNode.isDir) {
          const cachedFocusedItem = this.buildNodeScreenItem(viewProj, fNode);
          if (cachedFocusedItem) {
            ctx.save();
            ctx.globalAlpha = 1.0;
            this.drawLod2Tile(ctx, cachedFocusedItem, true, true);
            ctx.restore();
          }
        }
        return;
      }

      const lod0Candidates = [];
      const lod1Queue = [];
      const lod2Queue = [];
      let focusedItem = null;
      const mix = this.curExtrude;

      const stack = [0];
      while (stack.length > 0) {
        const idx = stack.pop();
        const n = this.nodes[idx];
        const b = n.bounds;
        if (!b || b.w <= 0 || b.h <= 0) continue;

        const maxSubZ = mix > 0.002 ? this.subtreeMaxTopZ[idx] * ELEV_SCALE * mix : 0.01;
        const topZ = mix > 0.002 ? (this.effZ[idx] + this.effH[idx]) * ELEV_SCALE * mix : n.depth * 0.002 + 0.001;

        const p0 = this.projectPoint(viewProj, b.x, b.y, topZ);
        const p1 = this.projectPoint(viewProj, b.x + b.w, b.y, topZ);
        const p2 = this.projectPoint(viewProj, b.x + b.w, b.y + b.h, topZ);
        const p3 = this.projectPoint(viewProj, b.x, b.y + b.h, topZ);

        if (!p0 || !p1 || !p2 || !p3) {
          if (n.isDir) {
            const ch = this.childrenByIdx[idx];
            for (let k = ch.length - 1; k >= 0; k -= 1) stack.push(ch[k]);
          }
          continue;
        }

        const minX = Math.min(p0.x, p1.x, p2.x, p3.x);
        const maxX = Math.max(p0.x, p1.x, p2.x, p3.x);
        const minY = Math.min(p0.y, p1.y, p2.y, p3.y);
        const maxY = Math.max(p0.y, p1.y, p2.y, p3.y);

        const padMargin = n.isDir ? Math.max(80, maxSubZ * 2.5) : 40;
        if (maxX < -padMargin || minX > this.vpW + padMargin || maxY < -padMargin || minY > this.vpH + padMargin) {
          continue;
        }

        const screenW = Math.hypot(p1.x - p0.x, p1.y - p0.y);
        const screenH = Math.hypot(p3.x - p0.x, p3.y - p0.y);

        if (n.isDir) {
          if (screenW < 6 && screenH < 6) continue;
          if (screenW >= 42 && screenH >= 22 && n.depth > 0) {
            lod0Candidates.push({
              node: n,
              idx: idx,
              p0: p0,
              p1: p1,
              p2: p2,
              p3: p3,
              minX: minX,
              minY: minY,
              maxX: maxX,
              maxY: maxY,
              screenW: screenW,
              screenH: screenH
            });
          }
          const ch = this.childrenByIdx[idx];
          for (let k = ch.length - 1; k >= 0; k -= 1) stack.push(ch[k]);
        } else if (focusActive && n.relPath === activeFocusPath) {
          focusedItem = {
            node: n,
            idx: idx,
            p0: p0,
            p1: p1,
            p2: p2,
            p3: p3,
            minX: minX,
            minY: minY,
            maxX: maxX,
            maxY: maxY,
            screenW: screenW,
            screenH: screenH,
            avgW: (p0.w + p1.w + p2.w + p3.w) * 0.25
          };
        } else {
          if (screenW >= 240 && screenH >= 140) {
            lod2Queue.push({
              node: n,
              idx: idx,
              p0: p0,
              p1: p1,
              p2: p2,
              p3: p3,
              minX: minX,
              minY: minY,
              maxX: maxX,
              maxY: maxY,
              screenW: screenW,
              screenH: screenH,
              avgW: (p0.w + p1.w + p2.w + p3.w) * 0.25
            });
          } else if (screenW >= 54 && screenH >= 28) {
            lod1Queue.push({
              node: n,
              idx: idx,
              p0: p0,
              p1: p1,
              p2: p2,
              p3: p3,
              minX: minX,
              minY: minY,
              maxX: maxX,
              maxY: maxY,
              screenW: screenW,
              screenH: screenH
            });
          }
        }
      }

      if (focusActive && !focusedItem) {
        const fNode = this.nodesByPath.get(activeFocusPath);
        if (fNode && !fNode.isDir) {
          focusedItem = this.buildNodeScreenItem(viewProj, fNode);
        }
      }

      if (focusActive) {
        const focusCardRect = focusedItem ? this.computeFocusedCardRect(focusedItem) : null;
        const surroundAlpha = clamp(1.0 - this.curFocusMix * 0.76, 0.16, 1.0);
        if (this.ensureSurroundPyramid()) {
          this.drawSurroundFast(lod0Candidates, lod1Queue, lod2Queue, focusCardRect);
          this._surroundCacheKey = camKey;
          this._surroundCacheReady = true;
          ctx.save();
          ctx.globalAlpha = surroundAlpha;
          ctx.imageSmoothingEnabled = true;
          ctx.drawImage(
            this.surroundLowCanvas,
            0, 0, this.surroundLowCanvas.width, this.surroundLowCanvas.height,
            0, 0, this.vpW, this.vpH
          );
          ctx.restore();
        } else {
          ctx.save();
          ctx.globalAlpha = surroundAlpha;
          this.drawSurroundRects(ctx, 1, 1, lod0Candidates, lod1Queue, lod2Queue, focusCardRect);
          ctx.restore();
          this._surroundCacheKey = camKey;
          this._surroundCacheReady = true;
        }

        ctx.save();
        ctx.fillStyle = "rgba(11, 13, 17, " + (0.62 * this.curFocusMix).toFixed(3) + ")";
        ctx.fillRect(0, 0, this.vpW, this.vpH);
        ctx.restore();

        if (focusedItem) {
          ctx.save();
          ctx.globalAlpha = 1.0;
          this.drawLod2Tile(ctx, focusedItem, true, true);
          ctx.restore();
        }
        return;
      }

      this.drawLod0DirLabels(ctx, lod0Candidates);
      for (let i = 0; i < lod1Queue.length; i += 1) {
        this.drawLod1Tile(ctx, lod1Queue[i]);
      }

      lod2Queue.sort((a, b) => {
        const aFocus = (a.node.relPath === this.selectedRelPath ? 2 : 0) + (a.node.relPath === this.hoveredRelPath ? 1 : 0);
        const bFocus = (b.node.relPath === this.selectedRelPath ? 2 : 0) + (b.node.relPath === this.hoveredRelPath ? 1 : 0);
        if (aFocus !== bFocus) return aFocus - bFocus;
        return b.avgW - a.avgW;
      });
      for (let i = 0; i < lod2Queue.length; i += 1) {
        this.drawLod2Tile(ctx, lod2Queue[i], false, true);
      }
    }

    drawLod0DirLabels(ctx, candidates) {
      if (candidates.length === 0) return;

      candidates.sort((a, b) => {
        const aSel = a.node.relPath === this.selectedRelPath ? 1 : 0;
        const bSel = b.node.relPath === this.selectedRelPath ? 1 : 0;
        if (aSel !== bSel) return bSel - aSel;
        if (a.node.depth !== b.node.depth) return a.node.depth - b.node.depth;
        return b.screenW * b.screenH - a.screenW * a.screenH;
      });

      const placed = [];
      ctx.font = "600 10.5px " + MONO_FONT;
      ctx.textBaseline = "middle";

      let drawn = 0;
      for (let i = 0; i < candidates.length && drawn < 85; i += 1) {
        const c = candidates[i];
        const n = c.node;
        const hits = this.countDirSearchHits(c.idx);
        const maxLabelW = Math.min(240, Math.max(48, c.screenW - 8));
        const labelText = truncateText(ctx, n.name || n.relPath, maxLabelW - (hits > 0 ? 44 : 18));
        if (!labelText) continue;

        const textW = ctx.measureText(labelText).width;
        const badgeText = hits > 0 ? String(hits) : "";
        const badgeW = hits > 0 ? ctx.measureText(badgeText).width + 10 : 0;
        const tagW = Math.min(c.screenW - 4, textW + 20 + badgeW);
        const tagH = 18;
        if (tagW < 28) continue;

        const px = Math.round(clamp(c.p0.x + 4, 4, this.vpW - tagW - 4)) + 0.5;
        const py = Math.round(clamp(c.p0.y + 4, 4, this.vpH - tagH - 4)) + 0.5;
        if (px + tagW < c.minX || px > c.maxX || py + tagH < c.minY || py > c.maxY) continue;

        const box = { x: px - 2, y: py - 2, w: tagW + 4, h: tagH + 4 };
        let overlaps = false;
        for (let j = 0; j < placed.length; j += 1) {
          if (rectsOverlap(box, placed[j])) {
            overlaps = true;
            break;
          }
        }
        if (overlaps) continue;
        placed.push(box);
        drawn += 1;

        const isSel = n.relPath === this.selectedRelPath;
        ctx.save();
        roundRectPath(ctx, px, py, tagW, tagH, 2);
        ctx.fillStyle = isSel ? "rgba(33, 29, 21, 0.94)" : "rgba(17, 19, 23, 0.90)";
        ctx.fill();
        ctx.strokeStyle = isSel ? "#e5a93c" : "#343944";
        ctx.lineWidth = 1;
        ctx.stroke();

        ctx.fillStyle = isSel ? "#e5a93c" : (n.color || "#9b968c");
        ctx.fillRect(Math.round(px + 6), Math.round(py + tagH * 0.5 - 2.5), 5, 5);

        ctx.fillStyle = "#e8e4dc";
        ctx.fillText(labelText, px + 14, py + tagH * 0.5 + 0.5);

        if (hits > 0) {
          const bx = px + tagW - badgeW - 3;
          const by = py + 2.5;
          roundRectPath(ctx, bx, by, badgeW, tagH - 5, 2);
          ctx.fillStyle = "rgba(229, 169, 60, 0.20)";
          ctx.fill();
          ctx.strokeStyle = "rgba(229, 169, 60, 0.45)";
          ctx.stroke();
          ctx.fillStyle = "#e5a93c";
          ctx.fillText(badgeText, bx + 5, py + tagH * 0.5 + 0.5);
        }
        ctx.restore();
      }
    }

    countDirSearchHits(dirIdx) {
      if (!this.hasSearch) return 0;
      let total = 0;
      const stack = [dirIdx];
      while (stack.length > 0) {
        const idx = stack.pop();
        const n = this.nodes[idx];
        if (!n.isDir) {
          total += this.fileHitsMap.get(n.relPath) || 0;
        } else {
          const ch = this.childrenByIdx[idx];
          for (let k = 0; k < ch.length; k += 1) stack.push(ch[k]);
        }
      }
      return total;
    }

    applyQuadTransform(ctx, item) {
      const w = Math.max(1, item.screenW);
      const h = Math.max(1, item.screenH);
      const a = (item.p1.x - item.p0.x) / w;
      const b = (item.p1.y - item.p0.y) / w;
      const c = (item.p3.x - item.p0.x) / h;
      const d = (item.p3.y - item.p0.y) / h;
      const det = a * d - b * c;
      if (Math.abs(det) < 1e-4) return false;
      ctx.transform(a, b, c, d, item.p0.x, item.p0.y);
      return true;
    }

    drawLod1Tile(ctx, item) {
      const n = item.node;
      const w = item.screenW;
      const h = item.screenH;

      ctx.save();
      ctx.beginPath();
      ctx.moveTo(item.p0.x, item.p0.y);
      ctx.lineTo(item.p1.x, item.p1.y);
      ctx.lineTo(item.p2.x, item.p2.y);
      ctx.lineTo(item.p3.x, item.p3.y);
      ctx.closePath();
      ctx.clip();

      if (!this.applyQuadTransform(ctx, item)) {
        ctx.restore();
        return;
      }

      const headerH = clamp(h * 0.20, 14, 20);
      ctx.fillStyle = "rgba(17, 19, 23, 0.88)";
      ctx.fillRect(0, 0, w, headerH);
      ctx.fillStyle = "#282c35";
      ctx.fillRect(0, headerH - 1, w, 1);

      const lang = n.stats && n.stats.language ? n.stats.language : "Text";
      const dotCol = LANG_COLORS[lang] || n.color || "#e5a93c";
      ctx.fillStyle = dotCol;
      ctx.fillRect(5, headerH * 0.5 - 2.5, 5, 5);

      const hits = this.fileHitsMap.get(n.relPath) || 0;
      const hitStr = hits > 0 ? String(hits) : "";
      ctx.font = "600 10.5px " + MONO_FONT;
      ctx.textBaseline = "middle";
      const hitBoxW = hits > 0 ? ctx.measureText(hitStr).width + 10 : 0;

      const label = truncateText(ctx, n.name || "", w - 18 - hitBoxW);
      if (label) {
        ctx.fillStyle = "#e8e4dc";
        ctx.fillText(label, 14, headerH * 0.5 + 0.5);
      }

      if (hits > 0 && w > 42) {
        const bx = w - hitBoxW - 3;
        roundRectPath(ctx, bx, 2.5, hitBoxW, headerH - 5, 2);
        ctx.fillStyle = "rgba(229, 169, 60, 0.22)";
        ctx.fill();
        ctx.fillStyle = "#e5a93c";
        ctx.fillText(hitStr, bx + 5, headerH * 0.5 + 0.5);
      }

      const bodyY = headerH + 3;
      const bodyH = h - bodyY - 3;
      if (bodyH >= 8) {
        const preview = Array.isArray(n.previewLines) && n.previewLines.length > 0 ? n.previewLines : null;
        const barCount = preview ? preview.length : clamp(Math.floor((n.stats && n.stats.lineCount) || 16), 6, 40);
        const stepY = bodyH / Math.max(1, barCount);
        const barH = clamp(stepY * 0.62, 1.0, 2.6);
        const maxBarW = Math.max(4, w - 12);
        const matched = this.matchedLinesMap.get(n.relPath);
        const totalLines = Math.max(1, (n.stats && n.stats.lineCount) || barCount);

        ctx.fillStyle = "rgba(232, 228, 220, 0.20)";
        for (let i = 0; i < barCount; i += 1) {
          const val = preview ? preview[i] : ((Math.sin(i * 1.7 + (n.id || 1)) * 0.5 + 0.5) * 75 + 15);
          if (val <= 2) continue;
          const bw = (clamp(val, 5, 100) / 100) * maxBarW;
          const by = bodyY + i * stepY;
          if (by + barH > h - 2) break;
          ctx.fillRect(6, by, bw, barH);
        }

        if (matched && matched.size > 0) {
          ctx.fillStyle = "#e5a93c";
          matched.forEach((lineNum) => {
            const frac = clamp((lineNum - 1) / totalLines, 0, 1);
            const my = bodyY + frac * (bodyH - 2);
            ctx.fillRect(3, my, w - 6, Math.max(2.0, barH + 0.6));
          });
        }
      }

      ctx.restore();
    }

    drawLod2Tile(ctx, item, isFocusedFile = false, registerHitRegion = true) {
      const n = item.node;
      this.ensureFileRequested(n.relPath);
      const cached = this.fileCache.get(n.relPath);
      const lang = (cached && cached.language) || (n.stats && n.stats.language) || "Text";
      const lines = cached ? cached.lines : null;
      const totalLines = lines ? lines.length : 0;

      const isTilted = Math.abs(this.curPitch) >= 0.14 || Math.abs(this.curYaw) >= 0.14;
      const focusRender = Boolean(isFocusedFile && this.curFocusMix > 0.01);
      const useScreenCard = focusRender || isTilted;
      let cardX = 0;
      let cardY = 0;
      let localW = item.screenW;
      let localH = item.screenH;
      let invTransform = null;
      let txE = item.p0.x;
      let txF = item.p0.y;

      ctx.save();
      if (focusRender) {
        const rect = this.computeFocusedCardRect(item);
        localW = rect.w;
        localH = rect.h;
        cardX = rect.x;
        cardY = rect.y;

        ctx.fillStyle = "rgba(8, 10, 14, 0.52)";
        roundRectPath(ctx, cardX - 4, cardY - 2, localW + 8, localH + 10, 5);
        ctx.fill();
        roundRectPath(ctx, cardX, cardY, localW, localH, 3);
        ctx.fillStyle = "rgba(17, 19, 24, 0.98)";
        ctx.fill();
        ctx.strokeStyle = "#e5a93c";
        ctx.lineWidth = 1.75;
        ctx.stroke();
        ctx.clip();
        ctx.translate(cardX, cardY);
      } else if (isTilted) {
        const cx = (item.p0.x + item.p1.x + item.p2.x + item.p3.x) * 0.25;
        const cy = (item.p0.y + item.p1.y + item.p2.y + item.p3.y) * 0.25;
        const rawW = Math.max(item.screenW, item.maxX - item.minX);
        const rawH = Math.max(item.screenH, item.maxY - item.minY);
        localW = clamp(rawW * 0.94, 240, Math.min(this.vpW - 20, 780));
        localH = clamp(rawH * 0.94, 148, Math.min(this.vpH - 20, 640));
        cardX = clamp(cx - localW * 0.5, 10, Math.max(10, this.vpW - localW - 10));
        cardY = clamp(cy - localH * 0.5, 10, Math.max(10, this.vpH - localH - 10));

        roundRectPath(ctx, cardX, cardY, localW, localH, 2);
        ctx.fillStyle = "rgba(19, 22, 27, 0.96)";
        ctx.fill();
        ctx.strokeStyle = n.relPath === this.selectedRelPath ? "#e5a93c" : "#343944";
        ctx.lineWidth = n.relPath === this.selectedRelPath ? 1.75 : 1;
        ctx.stroke();
        ctx.clip();
        ctx.translate(cardX, cardY);
      } else {
        ctx.beginPath();
        ctx.moveTo(item.p0.x, item.p0.y);
        ctx.lineTo(item.p1.x, item.p1.y);
        ctx.lineTo(item.p2.x, item.p2.y);
        ctx.lineTo(item.p3.x, item.p3.y);
        ctx.closePath();
        ctx.clip();

        const a = (item.p1.x - item.p0.x) / localW;
        const b = (item.p1.y - item.p0.y) / localW;
        const c = (item.p3.x - item.p0.x) / localH;
        const d = (item.p3.y - item.p0.y) / localH;
        const det = a * d - b * c;
        if (Math.abs(det) < 1e-4) {
          ctx.restore();
          return;
        }
        ctx.transform(a, b, c, d, item.p0.x, item.p0.y);
        invTransform = [d / det, -c / det, -b / det, a / det];

        ctx.fillStyle = "rgba(19, 22, 27, 0.94)";
        ctx.fillRect(0, 0, localW, localH);
      }

      const headerH = CODE_HEADER_HEIGHT;
      const bodyY = headerH;
      const bodyH = Math.max(1, localH - headerH);
      const maxScroll = Math.max(0, totalLines * CODE_LINE_HEIGHT - bodyH + 12);
      const scrollPx = clamp(this.scrollOffsets.get(n.relPath) || 0, 0, maxScroll);
      if (this.scrollOffsets.has(n.relPath)) {
        this.scrollOffsets.set(n.relPath, scrollPx);
      }

      ctx.fillStyle = "rgba(22, 24, 29, 0.98)";
      ctx.fillRect(0, 0, localW, headerH);
      ctx.fillStyle = "#2b2f38";
      ctx.fillRect(0, headerH - 1, localW, 1);

      const dotCol = LANG_COLORS[lang] || n.color || "#e5a93c";
      ctx.fillStyle = dotCol;
      ctx.fillRect(8, headerH * 0.5 - 3, 6, 6);

      const hits = this.fileHitsMap.get(n.relPath) || 0;
      const sloc = n.stats && n.stats.sourceLineCount ? n.stats.sourceLineCount : totalLines;
      const metaText = lang + (sloc ? " · " + sloc + " SLOC" : "");
      ctx.font = "500 10.5px " + UI_FONT;
      ctx.textBaseline = "middle";
      const metaW = ctx.measureText(metaText).width + 12;
      const hitText = hits > 0 ? hits + " hits" : "";
      const hitW = hits > 0 ? ctx.measureText(hitText).width + 14 : 0;

      const unfocusLabel = "Unfocus · Esc";
      ctx.font = "600 10.5px " + MONO_FONT;
      const unfocusW = (focusRender && localW >= 260) ? ctx.measureText(unfocusLabel).width + 14 : 0;
      const rightMargin = unfocusW > 0 ? unfocusW + 10 : 0;

      ctx.font = "600 11.5px " + MONO_FONT;
      const rawTitle = focusRender ? (n.relPath || n.name) : (n.name || n.relPath);
      const titleText = truncateText(ctx, rawTitle, localW - 28 - metaW - hitW - rightMargin);
      ctx.fillStyle = "#e8e4dc";
      ctx.fillText(titleText, 20, headerH * 0.5 + 0.5);

      ctx.font = "500 10.5px " + UI_FONT;
      ctx.fillStyle = "#9a958c";
      ctx.fillText(metaText, localW - metaW - hitW - rightMargin + 4, headerH * 0.5 + 0.5);

      if (hits > 0) {
        const hx = localW - hitW - rightMargin + 2;
        roundRectPath(ctx, hx, 5, hitW - 6, headerH - 10, 2);
        ctx.fillStyle = "rgba(229, 169, 60, 0.20)";
        ctx.fill();
        ctx.fillStyle = "#e5a93c";
        ctx.fillText(hitText, hx + 5, headerH * 0.5 + 0.5);
      }

      let unfocusRect = null;
      if (unfocusW > 0) {
        const ux = localW - unfocusW - 6;
        const uy = 4;
        const uh = headerH - 8;
        unfocusRect = { x: ux, y: uy, w: unfocusW, h: uh };
        roundRectPath(ctx, ux, uy, unfocusW, uh, 2);
        ctx.fillStyle = "rgba(229, 169, 60, 0.14)";
        ctx.fill();
        ctx.strokeStyle = "rgba(229, 169, 60, 0.52)";
        ctx.lineWidth = 1;
        ctx.stroke();
        ctx.fillStyle = "#e5a93c";
        ctx.font = "600 10.5px " + MONO_FONT;
        ctx.textBaseline = "middle";
        ctx.fillText(unfocusLabel, ux + 7, headerH * 0.5 + 0.5);
      }

      if (registerHitRegion) {
        this.lod2HitRegions.push({
          node: n,
          isScreenAligned: useScreenCard,
          cardX: cardX,
          cardY: cardY,
          cardW: localW,
          cardH: localH,
          localW: localW,
          localH: localH,
          headerH: headerH,
          bodyY: bodyY,
          bodyH: bodyH,
          lineH: CODE_LINE_HEIGHT,
          scrollPx: scrollPx,
          totalLines: totalLines,
          invTransform: invTransform,
          e: txE,
          f: txF,
          unfocusRect: unfocusRect
        });
      }

      ctx.save();
      ctx.beginPath();
      ctx.rect(0, bodyY, localW, bodyH);
      ctx.clip();

      if (!lines) {
        ctx.font = "500 11.5px " + MONO_FONT;
        ctx.fillStyle = "#9a958c";
        ctx.fillText("Loading source...", 14, bodyY + 20);
      } else {
        const gutterW = Math.min(CODE_GUTTER_WIDTH, Math.max(32, localW * 0.16));
        ctx.fillStyle = "rgba(17, 19, 23, 0.80)";
        ctx.fillRect(0, bodyY, gutterW, bodyH);
        ctx.fillStyle = "#2b2f38";
        ctx.fillRect(gutterW, bodyY, 1, bodyH);

        const startIdx = Math.max(0, Math.floor(scrollPx / CODE_LINE_HEIGHT));
        const endIdx = Math.min(totalLines, Math.ceil((scrollPx + bodyH) / CODE_LINE_HEIGHT) + 1);
        const matchedSet = this.matchedLinesMap.get(n.relPath);
        const isSelFile = n.relPath === this.selectedRelPath;
        const isHovFile = n.relPath === this.hoveredRelPath;
        const tokenizer = global.ConfineTokenizer;

        ctx.font = "400 11.5px " + MONO_FONT;
        ctx.textBaseline = "middle";

        for (let i = startIdx; i < endIdx; i += 1) {
          const lineNum = i + 1;
          const rowY = bodyY + i * CODE_LINE_HEIGHT - scrollPx;
          const textY = rowY + CODE_LINE_HEIGHT * 0.5;

          const isSelLine = isSelFile && this.selectedLine === lineNum;
          const isMatched = matchedSet ? matchedSet.has(lineNum) : false;
          const isHovLine = isHovFile && this.hoveredLine === lineNum;

          if (isSelLine) {
            ctx.fillStyle = "rgba(229, 169, 60, 0.18)";
            ctx.fillRect(0, rowY, localW, CODE_LINE_HEIGHT);
            ctx.fillStyle = "#e5a93c";
            ctx.fillRect(0, rowY, 2.5, CODE_LINE_HEIGHT);
          } else if (isMatched) {
            ctx.fillStyle = "rgba(229, 169, 60, 0.12)";
            ctx.fillRect(0, rowY, localW, CODE_LINE_HEIGHT);
            ctx.fillStyle = "#d4943a";
            ctx.fillRect(0, rowY, 2.5, CODE_LINE_HEIGHT);
          } else if (isHovLine) {
            ctx.fillStyle = "rgba(232, 228, 220, 0.06)";
            ctx.fillRect(0, rowY, localW, CODE_LINE_HEIGHT);
          }

          ctx.fillStyle = isSelLine ? "#e5a93c" : (isMatched ? "#e5a93c" : "#6d6a63");
          const numStr = String(lineNum);
          const numW = ctx.measureText(numStr).width;
          ctx.fillText(numStr, gutterW - numW - 6, textY);

          const rawLine = lines[i] || "";
          if (rawLine.length > 0) {
            const expanded = rawLine.replace(/\t/g, "  ");
            let drawX = gutterW + 8;
            const maxX = localW - 10;

            if (tokenizer) {
              let toks = cached.tokenized.get(i);
              if (!toks) {
                toks = tokenizer.tokenizeLine(expanded.slice(0, 320), lang);
                cached.tokenized.set(i, toks);
              }
              for (let t = 0; t < toks.length; t += 1) {
                if (drawX >= maxX) break;
                const tok = toks[t];
                ctx.fillStyle = tokenizer.tokenColor(tok.type);
                const wTok = ctx.measureText(tok.text).width;
                if (drawX + wTok > maxX) {
                  const clipped = truncateText(ctx, tok.text, maxX - drawX);
                  if (clipped) ctx.fillText(clipped, drawX, textY);
                  break;
                }
                ctx.fillText(tok.text, drawX, textY);
                drawX += wTok;
              }
            } else {
              ctx.fillStyle = "#e8e4dc";
              ctx.fillText(truncateText(ctx, expanded, maxX - drawX), drawX, textY);
            }
          }
        }

        if (maxScroll > 0 && bodyH > 24) {
          const trackH = bodyH - 8;
          const totalH = totalLines * CODE_LINE_HEIGHT;
          const thumbH = Math.max(18, (bodyH / totalH) * trackH);
          const thumbY = bodyY + 4 + (scrollPx / maxScroll) * (trackH - thumbH);
          ctx.fillStyle = "rgba(232, 228, 220, 0.26)";
          roundRectPath(ctx, localW - 7, thumbY, 4, thumbH, 1.5);
          ctx.fill();
        }
      }

      ctx.restore();
      ctx.restore();
    }
  }

  global.ConfineRenderer = ConfineRenderer;
})(typeof window !== "undefined" ? window : globalThis);
