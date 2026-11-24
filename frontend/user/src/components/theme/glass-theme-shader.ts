// Optical shader adapted from Zsolt Kacso’s Glass Light Toggle:
// https://experiments.thisiswhitespace.com/standalone/glass-light-toggle/main.js
// Track widened to three positions; page background replaced with local transparency.
export const glassThemeFragmentShader = `#version 300 es
precision highp float;
out vec4 o;
uniform vec2 uRes; uniform float uDpr; uniform float uScale;
uniform vec2 uC; uniform vec2 uTH; uniform float uTR;
uniform vec2 uK; uniform vec2 uKH; uniform float uKR;
uniform float uTheme, uTime, uLed, uLift;
uniform vec4 uBox;
uniform sampler2D uPix, uGlow;

uniform vec3 uLight;
uniform vec3 uBackground;
uniform float uTheta, uHalfW, uRefract, uFrost, uGlowAmt, uMode, uStripLen;
uniform vec2 uStrip;
#define AMBER uLight
#define PEACH mix(uLight, vec3(1.0), 0.32)
float gPixA = 0.;

// One glow model for both themes. On dark, light adds. On a bright surface adding does
// nothing, so emit as a saturated tint with the same falloff: same perceived strength.
vec3 glow(vec3 col, float amt){
  vec3 dark = col + uLight * amt;
  vec3 lit  = mix(col, uLight * 1.04 + 0.03, clamp(1. - exp(-amt * 1.3), 0., 0.88));
  return mix(dark, lit, uTheme);
}

float sdRB(vec2 p, vec2 b, float r){ vec2 q = abs(p) - b + r; return length(max(q,0.)) + min(max(q.x,q.y),0.) - r; }
float hash(vec2 p){ p = fract(p*vec2(443.897,441.423)); p += dot(p, p.yx+19.19); return fract((p.x+p.y)*p.x); }

float tex(sampler2D s, vec2 p){
  vec2 uv = (p - uBox.xy) / uBox.zw;
  if (uv.x < 0. || uv.y < 0. || uv.x > 1. || uv.y > 1.) return 0.;
  return texture(s, uv).a;
}

// The light strip hugs the glass from outside and travels around its perimeter.
// uTheta = angular centre of the strip, uHalfW = angular half-length.
float ledSd(vec2 p){
  if (uMode > 0.5) {
    // Track mode: strip runs along the inside wall of the container, centred on uStrip.
    float rad = sdRB(p - uC, uTH, uTR) + 6.5;
    float tang = max(length(p - uStrip) - uStripLen, 0.);
    return length(vec2(rad, tang)) - 1.5;
  }
  vec2 q = p - uK;
  float sk = sdRB(q, uKH, uKR);
  float a = atan(q.y, q.x);
  float da = abs(atan(sin(a - uTheta), cos(a - uTheta)));
  float tang = max(da - uHalfW, 0.) * 104.;
  float rad = sk - 6.5;
  return length(vec2(rad, tang)) - 1.5;
}

// light from the LED as it washes the track floor
float ledFloor(vec2 p){
  float ls = max(ledSd(p), 0.);
  return (exp(-ls/14.)*0.85 + exp(-ls/55.)*0.32) * uLed;
}

// The world under the glass: track floor + LED wash + pixel matrix.
vec3 floorScene(vec2 p, float frost, vec2 ca){
  float th = uTheme;
  vec3 base = mix(vec3(0.030), vec3(0.900), th);
  base *= 1. + (p.y - uC.y)/uTH.y * mix(0.05, 0.012, th);
  float sOff = sdRB(p - uC - vec2(0., 7.), uTH, uTR);
  float inner = smoothstep(-14., 3., sOff);
  base = mix(base * (1. - inner*0.6), base - inner*0.085, th);

  float g = ledFloor(p);
  base = glow(base, g*0.55);

  vec3 a;
  a.r = mix(tex(uPix, p - ca), tex(uGlow, p - ca)*1.35, frost);
  a.g = mix(tex(uPix, p     ), tex(uGlow, p     )*1.35, frost);
  a.b = mix(tex(uPix, p + ca), tex(uGlow, p + ca)*1.35, frost);
  float lum = dot(base, vec3(0.299, 0.587, 0.114));
  float inkT = smoothstep(0.40, 0.56, lum);
  vec3 pc = mix(vec3(1.0, 0.965, 0.925), vec3(0.11, 0.105, 0.10), inkT);
  pc = mix(pc + AMBER*g*0.9, pc + vec3(0.25,0.08,0.0)*g, th);
  gPixA = clamp(a.g, 0., 1.);
  vec3 col = mix(base, pc, clamp(a, 0., 1.));
  col += vec3(1.0, 0.78, 0.55) * tex(uGlow, p) * uGlowAmt * (1. - th);
  return col;
}

void main(){
  vec2 praw = vec2(gl_FragCoord.x, uRes.y - gl_FragCoord.y) / uDpr;
  vec2 p = uC + (praw - uC) / uScale; // design space: everything below is authored at desktop size
  vec2 vp = uRes / uDpr;
  float th = uTheme;

  // ---- page
  vec3 col = uBackground;

  float st = sdRB(p - uC, uTH, uTR);
  float sh = 1. - smoothstep(-10., 34., sdRB(p - uC - vec2(0., 14.), uTH, uTR));
  col *= 1. - sh * mix(0.5, 0.17, th);
  float sh2 = 1. - smoothstep(-2., 4., sdRB(p - uC - vec2(0., 1.5), uTH, uTR));
  col *= 1. - sh2 * mix(0.3, 0.08, th);

  // LED spill onto the page
  float ls = max(ledSd(p), 0.);
  float spill = (exp(-ls/18.)*0.5 + exp(-ls/85.)*0.34) * uLed;
  col = glow(col, spill*0.6);

  // ---- track
  if (st < 2.) {
    vec3 tc = floorScene(p, 0., vec2(0.));
    float sk0 = sdRB(p - uK - vec2(0., 7. + uLift*5.), uKH, uKR);
    float ksh = 1. - smoothstep(-8., 20. + uLift*8., sk0);
    tc *= 1. - ksh * mix(0.6, 0.2, th);
    float rim = 1. - smoothstep(0., 1.3, abs(st + 0.75));
    float bottom = smoothstep(-uTH.y*0.6, uTH.y, p.y - uC.y);
    tc += rim * mix(mix(0.035, 0.09, bottom), mix(-0.07, 0.07, bottom), th);
    col = mix(tc, col, smoothstep(-0.6, 0.6, st));
  }

  // ---- glass knob
  float sk = sdRB(p - uK, uKH, uKR);
  if (sk < 2.) {
    vec2 e = vec2(0.5, 0.);
    vec2 n = vec2(sdRB(p + e.xy - uK, uKH, uKR) - sdRB(p - e.xy - uK, uKH, uKR),
                  sdRB(p + e.yx - uK, uKH, uKR) - sdRB(p - e.yx - uK, uKH, uKR));
    n = n / max(length(n), 1e-4);
    float d = max(-sk, 0.);
    float t = clamp(d / 26., 0., 1.);
    float slope = pow(1. - t, 2.6);

    // refraction: bevel bends rays inward, centre magnifies slightly
    vec2 off = -n * slope * uRefract - (p - uK) * 0.055;
    float frost = 0.06 + uFrost * slope;
    vec3 g = floorScene(p + off, frost, n * slope * 3.2 + (p - uK) * 0.006);

    float vy = clamp((p.y - (uK.y - uKH.y)) / (2. * uKH.y), 0., 1.);
    vec3 dGlass = g * 0.84 + vec3(0.135) * (1. - vy * 0.55);
    vec3 lGlass = mix(g, vec3(0.99), 0.5 * (1. - gPixA * 0.8));
    vec3 kc = mix(dGlass, lGlass, th);

    // LED light entering the glass from below and scattering inside it
    float lq = max(ledSd(p), 0.);
    float sc = (exp(-lq / 24.) * 0.8 + exp(-lq / 64.) * 0.2) * uLed;
    kc = glow(kc, sc * 0.8);
    // light trapped in the bevel (total internal reflection), strongest near the source
    float rg = exp(-d / 2.2) * exp(-lq / 40.) * uLed;
    kc = glow(kc, rg * 1.3);

    // specular
    vec2 Ld = normalize(vec2(-0.35, -1.0));
    float ndl = dot(n, Ld);
    kc += pow(max(ndl, 0.), 2.) * exp(-d / 1.25) * mix(0.42, 0.7, th);
    kc += max(ndl, 0.) * slope * mix(0.07, 0.16, th);
    kc *= 1. - max(-ndl, 0.) * slope * mix(0.28, 0.07, th);
    kc += exp(-d / 0.7) * mix(0.07, 0.12, th);

    col = mix(kc, col, smoothstep(-0.7, 0.7, sk));
  }

  // ---- the light source itself
  float lsd = ledSd(p);
  float lm = 1. - smoothstep(-0.6, 0.6, lsd);
  vec3 lc = mix(uLight * 1.05, mix(uLight, vec3(1.0), 0.65), (1. - smoothstep(-1.4, -0.2, lsd)));
  col = mix(col, lc * (0.8 + 0.35 * uLed), lm);
  col = glow(col, exp(-max(lsd, 0.) / 4.5) * 0.55 * uLed * (1. - lm));

  col += (hash(gl_FragCoord.xy + fract(uTime) * 91.7) - 0.5) * 0.022;
  float alpha = max(1. - smoothstep(-0.5, 1., st), clamp(spill * 0.65 + sh * 0.12, 0., 0.65));
  // Fade the spill before the canvas boundary so its rectangular footprint is invisible.
  float edge = min(min(praw.x, vp.x - praw.x), min(praw.y, vp.y - praw.y));
  alpha *= smoothstep(0., 10., edge);
  // Match the WebGL premultiplied compositor, including mobile WebKit.
  o = vec4(clamp(col, 0., 1.) * alpha, alpha);
}`
