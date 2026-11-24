import { glassThemeFragmentShader } from './glass-theme-shader'

type Theme = 'dark' | 'light' | 'system'
type Pixel = { x: number; y: number; b: number }
const modes: Theme[] = ['dark', 'light', 'system']

// Same 17 × 17 matrix and dimensions as the reference; the third glyph is a monitor.
function icon(mode: Theme): Pixel[] {
  const pixels: Pixel[] = []
  for (let y = 0; y < 17; y++) {
    for (let x = 0; x < 17; x++) {
      const radius = Math.hypot(x - 8, y - 8)
      let b = 0
      if (mode === 'dark') {
        if (radius <= 6.6) {
          b = Math.hypot(x - 11.3, y - 5.6) > 5.4 ? 1 : 0
          if (!b && (x + y) % 2 === 0) b = 0.2
        }
        if (
          (x === 15 && y <= 2) ||
          (y === 1 && x >= 14) ||
          (x === 11 && y === 0) ||
          (x === 16 && y === 5)
        ) {
          b = 0.8
        }
      } else if (mode === 'light') {
        if (radius <= 3.7) b = 1
        for (let a = 0; a < 8; a++) {
          const angle = (a * Math.PI) / 4
          for (const r of a % 2 ? [5.9, 7.2] : [5.6, 6.6, 7.6]) {
            if (
              x === Math.round(8 + Math.cos(angle) * r) &&
              y === Math.round(8 + Math.sin(angle) * r)
            ) {
              b = 1
            }
          }
        }
      } else if (
        (x >= 1 && x <= 15 && (y === 2 || y === 11)) ||
        ((x === 1 || x === 15) && y >= 2 && y <= 11) ||
        (x === 8 && y >= 12 && y <= 14) ||
        (y === 15 && x >= 5 && x <= 11)
      ) {
        b = 1
      }
      if (b) pixels.push({ x, y, b })
    }
  }
  return pixels.sort(
    (a, b) => Math.atan2(a.y - 8, a.x - 8) - Math.atan2(b.y - 8, b.x - 8)
  )
}
const icons = modes.map(icon)

/** Bounded optical renderer. No page-sized canvas; frames stop once the springs settle. */
export function createGlassThemeRenderer(
  canvas: HTMLCanvasElement,
  initial: Theme,
  initialDark: boolean,
  surface?: () => { x: number; width: number; height: number; radius: number; intensity?: number }
) {
  const gl = canvas.getContext('webgl2', {
    antialias: false,
    premultipliedAlpha: true,
    alpha: true,
  })
  if (!gl) return null
  const shaders: WebGLShader[] = []
  const textures: WebGLTexture[] = []
  const program = gl.createProgram()
  const buffer = gl.createBuffer()
  if (!program || !buffer) return null
  try {
    for (const [type, source] of [
      [
        gl.VERTEX_SHADER,
        '#version 300 es\nin vec2 a; void main(){gl_Position=vec4(a,0.,1.);}',
      ],
      [gl.FRAGMENT_SHADER, glassThemeFragmentShader],
    ] as const) {
      const shader = gl.createShader(type)
      if (!shader) throw new Error('shader unavailable')
      shaders.push(shader)
      gl.shaderSource(shader, source)
      gl.compileShader(shader)
      if (!gl.getShaderParameter(shader, gl.COMPILE_STATUS)) {
        throw new Error('shader unavailable')
      }
      gl.attachShader(program, shader)
    }
    gl.linkProgram(program)
    if (!gl.getProgramParameter(program, gl.LINK_STATUS)) {
      throw new Error('renderer unavailable')
    }
  } catch {
    shaders.forEach((shader) => gl.deleteShader(shader))
    gl.deleteProgram(program)
    gl.deleteBuffer(buffer)
    return null
  }
  gl.useProgram(program)
  gl.bindBuffer(gl.ARRAY_BUFFER, buffer)
  gl.bufferData(
    gl.ARRAY_BUFFER,
    new Float32Array([-1, -1, 3, -1, -1, 3]),
    gl.STATIC_DRAW
  )
  const attribute = gl.getAttribLocation(program, 'a')
  gl.enableVertexAttribArray(attribute)
  gl.vertexAttribPointer(attribute, 2, gl.FLOAT, false, 0, 0)
  const uniforms: Record<string, WebGLUniformLocation | null> = {}
  for (const name of [
    'uScale',
    'uMode',
    'uStrip',
    'uStripLen',
    'uLight',
    'uBackground',
    'uTheta',
    'uHalfW',
    'uRefract',
    'uFrost',
    'uGlowAmt',
    'uRes',
    'uDpr',
    'uC',
    'uTH',
    'uTR',
    'uK',
    'uKH',
    'uKR',
    'uTheme',
    'uTime',
    'uLed',
    'uLift',
    'uBox',
    'uPix',
    'uGlow',
  ]) {
    uniforms[name] = gl.getUniformLocation(program, name)
  }
  const pixelCanvas = document.createElement('canvas')
  const glowCanvas = document.createElement('canvas')
  pixelCanvas.width = 760
  pixelCanvas.height = 344
  glowCanvas.width = 380
  glowCanvas.height = 172
  const pixels = pixelCanvas.getContext('2d')
  const glow = glowCanvas.getContext('2d')
  if (!pixels || !glow) {
    shaders.forEach((shader) => gl.deleteShader(shader))
    gl.deleteProgram(program)
    gl.deleteBuffer(buffer)
    return null
  }
  for (let unit = 0; unit < 2; unit++) {
    const texture = gl.createTexture()
    if (!texture) continue
    textures.push(texture)
    gl.activeTexture(gl.TEXTURE0 + unit)
    gl.bindTexture(gl.TEXTURE_2D, texture)
    gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_MIN_FILTER, gl.LINEAR)
    gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_MAG_FILTER, gl.LINEAR)
    gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_S, gl.CLAMP_TO_EDGE)
    gl.texParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_T, gl.CLAMP_TO_EDGE)
  }
  gl.uniform1i((uniforms.uPix ?? null), 0)
  gl.uniform1i((uniforms.uGlow ?? null), 1)
  let position = modes.indexOf(initial),
    target = position,
    velocity = 0
  let theme = initialDark ? 0 : 1,
    themeTarget = theme
  let phase = theme,
    phaseVelocity = 0
  let previous = 0,
    frame = 0,
    destroyed = false,
    visible = true,
    dragging = false
  const motion = matchMedia('(prefers-reduced-motion: reduce)')
  const fromPixels = icons[position]!
  const particles = Array.from(
    { length: Math.max(...icons.map((points) => points.length)) },
    (_, i) => {
      const point = fromPixels[i % fromPixels.length]!
      return { x: (point.x - 8) * 5.7, y: (point.y - 8) * 5.7, vx: 0, vy: 0 }
    }
  )
  let surfaceX: number | undefined
  let surfaceWidth: number | undefined
  let surfaceVelocity = 0
  const draw = (now: number) => {
    if (destroyed || document.hidden || !visible) return
    const dt = Math.min(1 / 30, Math.max(0, (now - previous) / 1000))
    previous = now
    if (motion.matches) {
      position = target
      velocity = 0
      theme = themeTarget
      phase = themeTarget
    } else {
      if (!dragging) {
        for (let step = 0; step < 4; step++) {
          velocity += ((-129 * (position - target) - 13.6 * velocity) * dt) / 4
          position += (velocity * dt) / 4
        }
      }
      theme += (themeTarget - theme) * Math.min(1, dt * 10)
      phaseVelocity +=
        (-70 * (phase - themeTarget) - 16.73 * phaseVelocity) * dt
      phase += phaseVelocity * dt
    }
    const rect = canvas.getBoundingClientRect()
    const dpr = Math.min(devicePixelRatio || 1, 2)
    const width = Math.max(1, Math.round(rect.width * dpr)),
      height = Math.max(1, Math.round(rect.height * dpr))
    if (canvas.width !== width || canvas.height !== height) {
      canvas.width = width
      canvas.height = height
      gl.viewport(0, 0, width, height)
    }
    const geometry = surface?.()
    let surfaceMoving = false
    if (geometry) {
      surfaceX ??= geometry.x
      const delta = geometry.x - surfaceX
      surfaceMoving = Math.abs(delta) > .05
      surfaceWidth ??= geometry.width
      surfaceMoving ||= Math.abs(geometry.width-surfaceWidth)>.05
      if (motion.matches) { surfaceX=geometry.x; surfaceWidth=geometry.width }
      else {
        for(let step=0;step<4;step++) {
          surfaceVelocity += (210*(geometry.x-surfaceX)-27*surfaceVelocity)*dt/4
          surfaceX += surfaceVelocity*dt/4
        }
        surfaceWidth += (geometry.width-surfaceWidth)*Math.min(1,dt*16)
      }
      surfaceMoving ||= Math.abs(surfaceVelocity)>.05
      geometry.x = surfaceX
      geometry.width = surfaceWidth
    }
    const scale = geometry ? 1 : (rect.width - 24) / 640
    const cx = rect.width / 2,
      cy = rect.height / 2
    const boxX = cx - 380,
      boxY = cy - 172
    const kx = geometry ? geometry.x + geometry.width / 2 + 12 : cx + (position - 1) * 208
    pixels.clearRect(0, 0, 760, 344)
    pixels.fillStyle = '#fff'
    const clamped = Math.max(0, Math.min(2, position)),
      low = Math.floor(clamped),
      high = Math.min(2, low + 1),
      fraction = clamped - low
    let moving = false
    particles.forEach((particle, i) => {
      const a = icons[low]![i % icons[low]!.length]!,
        b = icons[high]![i % icons[high]!.length]!
      const stagger = (i % 11) / 30
      const t = Math.max(0, Math.min(1, (fraction - stagger) / 0.65))
      const ease = t * t * (3 - 2 * t),
        arc = Math.sin(Math.PI * ease)
      const ax = (a.x - 8) * 5.7,
        ay = (a.y - 8) * 5.7,
        bx = (b.x - 8) * 5.7,
        by = (b.y - 8) * 5.7
      const len = Math.hypot(bx - ax, by - ay) || 1
      const curl = ((i % 7) - 3) * 3.5 * 1.62 * arc
      const x = ax + (bx - ax) * ease - ((by - ay) / len) * curl,
        y = ay + (by - ay) * ease + ((bx - ax) / len) * curl
      if (motion.matches) {
        particle.x = x
        particle.y = y
        particle.vx = 0
        particle.vy = 0
      } else {
        particle.vx += (-210 * (particle.x - x) - 21 * particle.vx) * dt
        particle.vy += (-210 * (particle.y - y) - 21 * particle.vy) * dt
        particle.x += particle.vx * dt
        particle.y += particle.vy * dt
      }
      moving ||=
        Math.abs(particle.x - x) +
          Math.abs(particle.y - y) +
          Math.abs(particle.vx) +
          Math.abs(particle.vy) >
        0.05
      pixels.globalAlpha = Math.min(
        1,
        (a.b + (b.b - a.b) * ease) * (1 + 0.3 * arc)
      )
      const size = 4.6 * (1 - 0.45 * arc)
      pixels.fillRect(
        kx - boxX + particle.x - size / 2,
        cy - boxY + particle.y - size / 2,
        size,
        size
      )
    })
    pixels.globalAlpha = 1
    glow.clearRect(0, 0, 380, 172)
    glow.filter = 'blur(2px)'
    glow.drawImage(pixelCanvas, 0, 0, 380, 172)
    glow.filter = 'none'
    // Dim glyphs are visible behind the moving lens, like the original optional track icons.
    icons.forEach((points, index) =>
      points.forEach((point) => {
        pixels.globalAlpha = 0.12 * point.b
        pixels.fillRect(
          cx + (index - 1) * 208 - boxX + (point.x - 8) * 5.7 - 2.3,
          cy - boxY + (point.y - 8) * 5.7 - 2.3,
          4.6,
          4.6
        )
      })
    )
    pixels.globalAlpha = 1
    if (geometry) { pixels.clearRect(0,0,760,344); glow.clearRect(0,0,380,172) }
    for (let unit = 0; unit < 2; unit++) {
      gl.activeTexture(gl.TEXTURE0 + unit)
      gl.bindTexture(gl.TEXTURE_2D, textures[unit] ?? null)
      gl.texImage2D(
        gl.TEXTURE_2D,
        0,
        gl.RGBA,
        gl.RGBA,
        gl.UNSIGNED_BYTE,
        unit === 0 ? pixelCanvas : glowCanvas
      )
    }
    gl.uniform2f((uniforms.uRes ?? null), width, height)
    gl.uniform1f((uniforms.uDpr ?? null), dpr)
    gl.uniform1f((uniforms.uScale ?? null), scale)
    gl.uniform2f((uniforms.uC ?? null), cx, cy)
    if (geometry) { pixels.clearRect(0,0,760,344); glow.clearRect(0,0,380,172) }
    gl.uniform2f((uniforms.uTH ?? null), geometry ? (rect.width-24)/2 : 320, geometry ? (rect.height-24)/2 : 112)
    gl.uniform1f((uniforms.uTR ?? null), geometry ? geometry.radius : 72)
    const stretch = Math.min(Math.abs(velocity) * 6.5, 16)
    gl.uniform2f((uniforms.uK ?? null), kx, cy)
    gl.uniform2f((uniforms.uKH ?? null), geometry ? geometry.width/2 : 99 + stretch, geometry ? geometry.height/2 : 99 - stretch * 0.45)
    gl.uniform1f((uniforms.uKR ?? null), geometry ? geometry.radius : 58)
    gl.uniform1f((uniforms.uTheme ?? null), theme)
    gl.uniform1f((uniforms.uTime ?? null), now / 1000)
    gl.uniform1f(
      (uniforms.uLed ?? null),
      (geometry ? geometry.intensity ?? .65 : 1.89) + Math.min(Math.abs(velocity) * 0.05, 0.18)
    )
    gl.uniform1f((uniforms.uLift ?? null), 0)
    gl.uniform3f(
      (uniforms.uLight ?? null),
      0.435 + (0.999 - 0.435) * theme,
      0.525 + (0.659 - 0.525) * theme,
      1 + (0.18 - 1) * theme
    )
    gl.uniform3f(
      (uniforms.uBackground ?? null),
      theme * 0.94 + (1 - theme) * 0.055,
      theme * 0.955 + (1 - theme) * 0.07,
      theme * 0.975 + (1 - theme) * 0.1
    )
    gl.uniform1f((uniforms.uTheta ?? null), Math.PI / 2 - Math.PI * phase)
    gl.uniform1f((uniforms.uHalfW ?? null), 0.92)
    gl.uniform1f((uniforms.uMode ?? null), 0)
    gl.uniform2f((uniforms.uStrip ?? null), kx, cy + (geometry ? geometry.height/2 : 112))
    gl.uniform1f((uniforms.uStripLen ?? null), geometry ? Math.min(geometry.width*.48,96) : 120)
    gl.uniform1f((uniforms.uRefract ?? null), geometry ? 22 : 40)
    gl.uniform1f((uniforms.uFrost ?? null), geometry ? .5 : .76)
    gl.uniform1f((uniforms.uGlowAmt ?? null), geometry ? .38 : .64)
    gl.uniform4f((uniforms.uBox ?? null), boxX, boxY, 760, 344)
    gl.drawArrays(gl.TRIANGLES, 0, 3)
    if (
      !motion.matches &&
      (surfaceMoving || dragging ||
        moving ||
        Math.abs(position - target) +
          Math.abs(velocity) +
          Math.abs(theme - themeTarget) +
          Math.abs(phase - themeTarget) +
          Math.abs(phaseVelocity) >
          0.002)
    ) {
      frame = requestAnimationFrame(draw)
    } else frame = 0
  }
  const wake = () => {
    cancelAnimationFrame(frame)
    previous = performance.now()
    draw(previous)
  }
  const observer = new ResizeObserver(wake)
  observer.observe(canvas)
  const intersection = new IntersectionObserver((entries) => {
    visible = entries[0]?.isIntersecting ?? true
    if (visible) wake()
    else cancelAnimationFrame(frame)
  })
  intersection.observe(canvas)
  const visibility = () => {
    if (document.hidden) cancelAnimationFrame(frame)
    else wake()
  }
  document.addEventListener('visibilitychange', visibility)
  motion.addEventListener('change', wake)
  wake()
  return {
    update(next: Theme, dark: boolean) {
      target = modes.indexOf(next)
      themeTarget = dark ? 0 : 1
      wake()
    },
    drag(value: number) {
      dragging = true
      position = Math.max(0, Math.min(2, value))
      velocity = 0
      wake()
    },
    release() {
      dragging = false
      wake()
    },
    destroy() {
      destroyed = true
      cancelAnimationFrame(frame)
      observer.disconnect()
      intersection.disconnect()
      document.removeEventListener('visibilitychange', visibility)
      motion.removeEventListener('change', wake)
      textures.forEach((texture) => gl.deleteTexture(texture))
      shaders.forEach((shader) => gl.deleteShader(shader))
      gl.deleteProgram(program)
      gl.deleteBuffer(buffer)
    },
  }
}
