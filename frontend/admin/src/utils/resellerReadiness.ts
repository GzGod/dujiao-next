import type { AdminResellerDomain, AdminResellerProfile, ResellerSupplyPricing } from '@/api/types'

export function domainConnectionState(domain?: AdminResellerDomain): string {
  if (!domain) return 'unbound'
  if (domain.status === 'disabled') return 'disabled'
  if (domain.type === 'subdomain') return domain.status === 'active' && domain.verification_status === 'verified' ? 'ready' : 'pending_review'
  if (!domain.dns) return 'unconfigured'
  return ['ready', 'pending_dns', 'pending_tls', 'error'].includes(domain.dns.status) ? domain.dns.status : 'unconfigured'
}

export function canVisitResellerDomain(domain?: AdminResellerDomain, profileStatus = 'active'): boolean {
  return !!domain && profileStatus === 'active' && domain.status === 'active' && domain.verification_status === 'verified' && domainConnectionState(domain) === 'ready'
}

export function preferredResellerDomain(domains: AdminResellerDomain[] = []): AdminResellerDomain | undefined {
  return domains.find(d => canVisitResellerDomain(d) && d.is_primary) || domains.find(d => canVisitResellerDomain(d)) || domains.find(d => d.is_primary) || domains[0]
}

export function supplyRuleSummary(config?: ResellerSupplyPricing | null): { key: string; percent?: string } {
  if (config?.grade) {const percent=({basic:'3',intermediate:'2',advanced:'1'} as Record<string,string>)[config.grade];if(percent)return {key:'costMarkup',percent}}
  if (config?.default_markup_percent != null) return {key:'costMarkup',percent:config.default_markup_percent}
  return {key:'retailBase'}
}

export function resellerTasks(profile: Pick<AdminResellerProfile,'status'|'settlement_status'|'site_name'|'domains'>): string[] {
  const tasks: string[] = []
  if (profile.status === 'pending_review') return ['review']
  if (profile.status === 'rejected' || profile.status === 'disabled') return [profile.status]
  if (!profile.domains?.length) tasks.push('bindDomain')
  else if (!profile.domains.some(d => canVisitResellerDomain(d, profile.status))) tasks.push('fixDomain')
  if (!profile.site_name?.trim()) tasks.push('configureSite')
  if (profile.settlement_status === 'frozen') tasks.push('settlementFrozen')
  return tasks
}

export function isSupplyProductAllowed(config: Pick<ResellerSupplyPricing, 'allowed_product_ids' | 'blocked_product_ids'> | null | undefined, id: number): boolean {
  return (config?.allowed_product_ids == null || config.allowed_product_ids.includes(id)) && !config?.blocked_product_ids?.includes(id)
}
