import test from 'node:test'
import assert from 'node:assert/strict'
import {domainStage,isStorefrontReady,estimatedMargin} from '../src/utils/resellerWorkspace.ts'

test('DNS readiness requires an enabled verified domain and valid certificate',()=>{
 const d={type:'custom',status:'active',verification_status:'verified',dns:{status:'pending_dns'}}
 assert.equal(domainStage(d),1)
 assert.equal(isStorefrontReady(d),false)
 assert.equal(domainStage({...d,dns:{status:'pending_tls'}}),2)
 assert.equal(isStorefrontReady({...d,dns:{status:'ready'}}),true)
 assert.equal(isStorefrontReady({...d,status:'disabled',dns:{status:'ready'}}),false)
 assert.equal(isStorefrontReady({...d,verification_status:'pending',dns:{status:'ready'}}),false)
})
test('margin display uses exact cents and never substitutes unknown supply',()=>{
 assert.equal(estimatedMargin('118.00','110.33'),'7.67')
 assert.equal(estimatedMargin('0.30','0.10'),'0.20')
 assert.equal(estimatedMargin('99.00','100.00'),'-1.00')
 assert.equal(estimatedMargin('118.00',''),'—')
})
