import { describe, expect, it } from 'vitest'
import { isSupplyProductAllowed, canVisitResellerDomain, domainConnectionState, resellerTasks, supplyRuleSummary } from './resellerReadiness'
import type { AdminResellerDomain } from '@/api/types'
const domain = {id:1,domain:'shop.example.test',type:'custom',status:'active',verification_status:'verified',is_primary:true,dns:{status:'pending_dns'}} as AdminResellerDomain

describe('reseller actual readiness',()=>{
 it('does not equate administrative approval with DNS readiness',()=>{
  expect(domainConnectionState(domain)).toBe('pending_dns')
  expect(canVisitResellerDomain(domain)).toBe(false)
  expect(canVisitResellerDomain({...domain,dns:{...domain.dns!,status:'ready'}})).toBe(true)
  expect(canVisitResellerDomain({...domain,dns:{...domain.dns!,status:'ready'}},'disabled')).toBe(false)
 })
 it('keeps unknown domain checks and legacy supply prices explicit',()=>{
  expect(domainConnectionState({...domain,dns:undefined})).toBe('unconfigured')
  expect(supplyRuleSummary(null).key).toBe('retailBase')
  expect(supplyRuleSummary({default_markup_percent:'1',overrides:[]}).percent).toBe('1')
 })
 it('shows actionable onboarding tasks',()=>{
  expect(resellerTasks({status:'active',settlement_status:'normal',site_name:'',domains:[domain]})).toEqual(['fixDomain','configureSite'])
  expect(resellerTasks({status:'pending_review',settlement_status:'normal'})).toEqual(['review'])
 })
})

describe('storefront product authorization', () => {
 it('preserves an explicit GPT-only allowlist and gives blocks priority', () => {
  expect(isSupplyProductAllowed({allowed_product_ids:[19]},19)).toBe(true)
  expect(isSupplyProductAllowed({allowed_product_ids:[19]},31)).toBe(false)
  expect(isSupplyProductAllowed({allowed_product_ids:[]},19)).toBe(false)
  expect(isSupplyProductAllowed({allowed_product_ids:[19],blocked_product_ids:[19]},19)).toBe(false)
  expect(isSupplyProductAllowed(null,31)).toBe(true)
 })
})
