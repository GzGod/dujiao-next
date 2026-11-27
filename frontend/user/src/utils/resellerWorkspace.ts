export function domainStage(domain: {type?:string; status?:string; verification_status?:string; dns?:{status?:string}}): number {
  if (domain.status === 'disabled' || domain.verification_status === 'rejected') return -1
  if (domain.type === 'subdomain') return domain.status === 'active' && domain.verification_status === 'verified' ? 3 : 0
  if (domain.dns?.status === 'ready' && domain.status === 'active' && domain.verification_status === 'verified') return 3
  if (domain.dns?.status === 'pending_tls') return 2
  return domain.dns ? 1 : 0
}
export function isStorefrontReady(domain: Parameters<typeof domainStage>[0]): boolean { return domainStage(domain) === 3 }

// Presentation only: order/supply calculations remain authoritative on the server.
export function estimatedMargin(retail: string, supply: string): string {
  const cents = (value: string) => {
    const match = /^(\d+)(?:\.(\d{1,2}))?$/.exec(value)
    return match ? BigInt(match[1]!) * 100n + BigInt((match[2] || '').padEnd(2,'0')) : null
  }
  const a=cents(retail), b=cents(supply)
  if(a===null || b===null) return '—'
  const diff=a-b, absolute=diff<0n?-diff:diff
  return `${diff<0n?'-':''}${absolute/100n}.${String(absolute%100n).padStart(2,'0')}`
}
