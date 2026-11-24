import test from 'node:test'
import assert from 'node:assert/strict'
import { normalizePendingOrders, rememberPendingOrder, readPendingOrders, reconcilePendingOrder } from '../src/utils/pendingOrders.ts'
const now = Date.now()
const row = { orderNo: 'TEST-1', guest: true, owner: '', expiresAt: new Date(now + 3600000).toISOString(), createdAt: now }
test('pending references reject expired, stale, malformed and duplicate orders, and strip secrets', () => {
  const result = normalizePendingOrders([{...row, password:'secret'},row,{...row,orderNo:'EXPIRED',expiresAt:new Date(now-1).toISOString()},{...row,orderNo:'STALE',createdAt:now-8*86400000},{...row,orderNo:'bad/path'},null],now)
  assert.deepEqual(result,[row])
  assert.equal(JSON.stringify(result).includes('secret'),false)
})
test('persisted reminders survive reload and disappear after confirmed payment or cancellation', () => {
  const values = new Map<string,string>()
  const events: Event[] = []
  Object.defineProperty(globalThis,'localStorage',{configurable:true,value:{getItem:(key:string)=>values.get(key)||null,setItem:(key:string,value:string)=>values.set(key,value)}})
  Object.defineProperty(globalThis,'window',{configurable:true,value:{dispatchEvent:(event:Event)=>events.push(event)}})
  try {
    rememberPendingOrder(row)
    assert.deepEqual(readPendingOrders(),[row])
    reconcilePendingOrder(row.orderNo,{status:'pending_payment'})
    assert.equal(readPendingOrders().length,1)
    reconcilePendingOrder(row.orderNo,{status:'paid'})
    assert.equal(readPendingOrders().length,0)
    rememberPendingOrder(row)
    reconcilePendingOrder(row.orderNo,{status:'canceled'})
    assert.equal(readPendingOrders().length,0)
    assert.ok(events.length>=4)
  } finally { Reflect.deleteProperty(globalThis,'localStorage'); Reflect.deleteProperty(globalThis,'window') }
})
