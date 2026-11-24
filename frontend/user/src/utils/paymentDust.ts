export type PaymentDust = { stop: () => void; reverse: (destination?: HTMLElement) => Promise<void> }
type Fragment = { x: number; y: number; r: number; g: number; b: number; a: number; seed: number }

/** Capture non-sensitive labels at their actual DOM positions, then funnel their pixels into the CTA. */
export function dissolvePaymentPanel(element: HTMLElement, canvas: HTMLCanvasElement): PaymentDust {
  const bounds = element.getBoundingClientRect(), width = Math.max(1, bounds.width), height = Math.max(1, bounds.height)
  const ratio = Math.min(window.devicePixelRatio || 1, 2)
  canvas.width = Math.round(width * ratio); canvas.height = Math.round(height * ratio)
  const ctx = canvas.getContext('2d')
  if (!ctx) return { stop: () => {}, reverse: async () => {} }
  ctx.scale(ratio, ratio)
  const snapshot = document.createElement('canvas'); snapshot.width = Math.ceil(width); snapshot.height = Math.ceil(height)
  const ink = snapshot.getContext('2d')!
  const button = element.querySelector('.xck__summary > button')?.getBoundingClientRect()
  const origin = { x: button ? button.left - bounds.left + button.width / 2 : width / 2, y: Math.min(height - 20, button ? button.top - bounds.top + button.height / 2 : height * .88) }
  for (const node of element.querySelectorAll<HTMLElement>('*')) {
    const r = node.getBoundingClientRect(), style = getComputedStyle(node)
    if (!r.width || !r.height || style.display === 'none' || style.visibility === 'hidden') continue
    const x = r.left - bounds.left, y = r.top - bounds.top
    if (node.matches('button,input,.xck__mode-tabs,.xck__guest-note')) {
      ink.fillStyle = style.backgroundColor === 'rgba(0, 0, 0, 0)' ? 'rgba(115,145,190,.08)' : style.backgroundColor
      ink.beginPath(); ink.roundRect(x,y,r.width,r.height,Math.min(parseFloat(style.borderRadius)||8,r.height/2)); ink.fill()
      ink.strokeStyle = style.borderColor; ink.lineWidth = .6; ink.stroke()
    }
    ink.fillStyle = style.color; ink.font = `${style.fontWeight} ${style.fontSize} ${style.fontFamily}`; ink.textBaseline = 'top'
    // Deliberately exclude field values, passwords, and editable user content.
    for (const child of node.childNodes) if (child.nodeType === Node.TEXT_NODE && child.textContent?.trim()) {
      const range = document.createRange(); range.selectNodeContents(child)
      const rect = range.getBoundingClientRect()
      ink.fillText(child.textContent.trim(),rect.left-bounds.left,rect.top-bounds.top)
    }
  }
  const data=ink.getImageData(0,0,snapshot.width,snapshot.height).data
  const fragments: Fragment[]=[]
  const step=Math.max(2,Math.ceil(Math.sqrt(width*height/22000)))
  for(let y=0;y<height;y+=step)for(let x=0;x<width;x+=step){const i=(y*snapshot.width+x)*4;if(data[i+3]!>20)fragments.push({x,y,r:data[i]!,g:data[i+1]!,b:data[i+2]!,a:data[i+3]!/255,seed:Math.random()})}
  let raf=0,start=0,stop=false,settling=0,finish:(()=>void)|undefined
  const smooth=(t:number)=>t*t*(3-2*t)
  const draw=(time:number)=>{
    if(stop)return
    start ||= time
    const elapsed=(time-start)/1000
    const release=settling?Math.max(0,Math.min(1,(time-settling)/1050)):0
    ctx.clearRect(0,0,width,height)
    ctx.globalCompositeOperation='source-over'
    for(const p of fragments){
      const delay=(1-p.y/height)*.22+p.seed*.06
      const t=smooth(Math.max(0,Math.min(1,(elapsed-delay)/1.1)))
      const bend=Math.sin(t*Math.PI)
      const x=p.x+(origin.x-p.x)*t+Math.sin(p.y*.013+elapsed*2)*bend*14
      const y=p.y+(origin.y-p.y)*t-bend*20
      const streak=1+Math.pow(t,3)*18
      ctx.globalAlpha=p.a*(1-release)*(1-t*.5)
      ctx.fillStyle=`rgb(${p.r},${p.g},${p.b})`
      ctx.fillRect(x,y,Math.max(.7,step*(1-t*.8)),streak)
    }
    ctx.globalAlpha=1
    ctx.globalCompositeOperation='lighter'
    const intensity=Math.min(1,Math.max(0,(elapsed-.25)*1.2))*(1-release*.85)
    for(let i=0;i<76;i++){
      const basePhase=(elapsed*.65+i*.618)%1
      const phase=settling ? Math.max(0,1-Math.min(1,release*1.25+i/76*.12)) : basePhase
      const spread=(i/75-.5)*width*.85
      const x=origin.x+spread*Math.pow(1-phase,1.8)+Math.sin(phase*4+i)*6*(1-phase)
      const y=origin.y-height*.8*(1-phase)
      const tail=18+phase*36
      const hue=[205,235,275,40][i%4]
      if(!Number.isFinite(x)||!Number.isFinite(y)||!Number.isFinite(tail))continue
      const g=ctx.createLinearGradient(x,y-tail,x,y+tail);g.addColorStop(0,`hsla(${hue},90%,75%,0)`);g.addColorStop(.8,`hsla(${hue},95%,85%,${intensity*.25})`);g.addColorStop(1,`hsla(${hue},90%,75%,0)`)
      ctx.strokeStyle=g;ctx.lineWidth=.6;ctx.beginPath();ctx.moveTo(x,y-tail);ctx.quadraticCurveTo(x+(origin.x-x)*.2,y,origin.x+(x-origin.x)*.8,y+tail);ctx.stroke()
    }
    const glow=ctx.createRadialGradient(origin.x,origin.y,0,origin.x,origin.y,width*.24)
    glow.addColorStop(0,`rgba(225,240,255,${intensity*.65})`);glow.addColorStop(.12,`rgba(160,190,255,${intensity*.25})`);glow.addColorStop(1,'rgba(90,130,255,0)');ctx.fillStyle=glow;ctx.fillRect(0,0,width,height)
    if(release>=1){finish?.();finish=undefined;return}
    raf=requestAnimationFrame(draw)
  }
  raf=requestAnimationFrame(draw)
  return { stop:()=>{stop=true;cancelAnimationFrame(raf);ctx.clearRect(0,0,width,height);finish?.();finish=undefined},reverse:()=>new Promise(resolve=>{if(stop){resolve();return}settling=performance.now();finish=resolve}) }
}
