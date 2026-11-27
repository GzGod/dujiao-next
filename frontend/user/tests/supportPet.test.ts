import test from 'node:test'
import assert from 'node:assert/strict'
import {supportPetFrameSize} from '../src/utils/supportPet.ts'
test('standard Petdex sheet preserves Jiyi frame geometry',()=>{
 assert.deepEqual(supportPetFrameSize(1536,1872),{frame_width:192,frame_height:208})
 assert.deepEqual(supportPetFrameSize(2048,2304),{frame_width:256,frame_height:256})
})
test('ordinary images and unsupported or oversized sheets cannot be interpreted as standard frames',()=>{
 for(const [w,h] of [[0,0],[1254,1254],[1536,1871],[16384,18432],[1536.1,1872]])assert.equal(supportPetFrameSize(w,h),null)
})
