<template>
 <main class="redeem-page" :class="theme.redemption_style==='simple'?'is-simple':''" :style="{'--redeem-accent':theme.primary_color||'#0071e3'}">
  <header><h1>{{ theme.redemption_title||copy.title }}</h1><p>{{ theme.redemption_intro||copy.intro }}</p></header>
  <section class="redeem-panel">
   <label>{{ copy.code }}<input v-model="code" autocomplete="off" maxlength="80" :disabled="busy" @input="verified=null;session='';consent=false" /></label>
   <div class="redeem-actions"><button :disabled="busy||!code" @click="verify">{{ copy.verify }}</button><button :disabled="busy||!code" @click="progress">{{ copy.progress }}</button></div>
   <template v-if="verified">
    <label>{{ copy.session }}<textarea v-model="session" rows="7" autocomplete="off" spellcheck="false" :disabled="busy" /></label>
    <p v-if="email">{{ copy.account }} {{ email }}</p>
    <label class="redeem-consent"><input v-model="consent" type="checkbox" />{{ copy.consent }}</label>
    <button :disabled="busy||!consent||!email" @click="submit">{{ copy.submit }}</button>
   </template>
   <p role="status" aria-live="polite">{{ message }}</p>
  </section>
  <aside><h2>{{ copy.help }}</h2><p>{{ copy.instructions }}</p><a v-if="support" :href="support" target="_blank" rel="noopener noreferrer">{{ copy.support }}</a></aside>
 </main>
</template>
<script setup lang="ts">
import {computed,ref,onBeforeUnmount} from 'vue'
import {useI18n} from 'vue-i18n'
import {useAppStore} from '@/stores/app'
const store=useAppStore();const {locale}=useI18n()
const theme=computed(()=>store.config?.reseller_theme||{})
const resources={
'zh-CN':{title:'兑换核销',intro:'验证核销码，确认账号后提交办理；随时查询原码进度。',code:'核销码',verify:'验证核销码',progress:'查询进度',session:'账号 Session JSON',account:'本次办理账号：',consent:'我确认拥有该账号，并同意仅将本次资料用于办理所购服务。',submit:'确认账号并提交',help:'办理说明',instructions:'在 ChatGPT 官方网站登录自己的账号，按订单教程获取完整 Session。不要把密码或 Session 发到聊天中。提交后先查询原码进度，不要重复提交。',support:'联系客服',invalid:'核销码或 Session 格式不正确',pending:'资料已接收，尚不代表套餐已生效。请查询原码进度。',unknown:'结果无法确认，请查询原码进度，不要重复提交。',unavailable:'当前核销码不能提交，请查询进度或联系客服。',states:{unused:'未使用',inactive:'可办理',processing:'办理中',completed:'已完成，请登录账号核对',needs_action:'需要处理，请联系客服',destroyed:'核销码已销毁'}},
'zh-TW':{title:'兌換核銷',intro:'驗證核銷碼，確認帳號後提交辦理；隨時查詢原碼進度。',code:'核銷碼',verify:'驗證核銷碼',progress:'查詢進度',session:'帳號 Session JSON',account:'本次辦理帳號：',consent:'我確認擁有該帳號，並同意僅將本次資料用於辦理所購服務。',submit:'確認帳號並提交',help:'辦理說明',instructions:'在 ChatGPT 官方網站登入自己的帳號，依訂單教學取得完整 Session。不要將密碼或 Session 傳到聊天。提交後先查詢原碼進度，勿重複提交。',support:'聯絡客服',invalid:'核銷碼或 Session 格式不正確',pending:'資料已接收，尚不代表套餐已生效。請查詢原碼進度。',unknown:'結果無法確認，請查詢原碼進度，勿重複提交。',unavailable:'目前核銷碼不能提交，請查詢進度或聯絡客服。',states:{unused:'未使用',inactive:'可辦理',processing:'辦理中',completed:'已完成，請登入帳號核對',needs_action:'需要處理，請聯絡客服',destroyed:'核銷碼已銷毀'}},
'en-US':{title:'Redeem your code',intro:'Verify a code, confirm the account and submit. Check progress with the original code.',code:'Redemption code',verify:'Verify code',progress:'Check progress',session:'Account Session JSON',account:'Account to process:',consent:'I own this account and consent to using these details only to process this purchased service.',submit:'Confirm account and submit',help:'Instructions',instructions:'Sign in at the official ChatGPT website and follow your order instructions to obtain the complete Session. Never send passwords or Session data in chat. Check the original code before any retry.',support:'Contact support',invalid:'Invalid code or Session format',pending:'Details received; this does not confirm activation. Check progress.',unknown:'Result unknown. Check progress with the original code; do not resubmit.',unavailable:'This code cannot be submitted. Check progress or contact support.',states:{unused:'Unused',inactive:'Available',processing:'Processing',completed:'Completed; sign in to confirm',needs_action:'Action required; contact support',destroyed:'Code destroyed'}}}
const copy=computed(()=>resources[locale.value as keyof typeof resources]||resources['en-US'])
const support=computed(()=>{const value=String(store.config?.contact?.telegram||'');return /^https:\/\/(t\.me|telegram\.me)\/[A-Za-z0-9_]+$/.test(value)?value:''})
const code=ref(''),session=ref(''),consent=ref(false),busy=ref(false),message=ref(''),verified=ref<any>(null)
const email=computed(()=>{try{return JSON.parse(session.value).user?.email||''}catch{return ''}})
let controller:AbortController|undefined
onBeforeUnmount(()=>{controller?.abort();session.value=''})
async function request(operation:string,payload:object){
 controller=new AbortController();const timer=setTimeout(()=>controller?.abort(),30000)
 try{const response=await fetch('/api/v1/public/redemptions/'+operation,{method:'POST',credentials:'omit',redirect:'error',referrerPolicy:'no-referrer',cache:'no-store',headers:{'Content-Type':'application/json'},body:JSON.stringify(payload),signal:controller.signal});const result=await response.json();if(!response.ok||result.status_code!==0)throw Error(result.msg||copy.value.unknown);return result.data}finally{clearTimeout(timer)}
}
async function run(action:()=>Promise<void>){if(busy.value)return;busy.value=true;message.value='';try{code.value=code.value.trim().toUpperCase();if(!/^[A-Z0-9-]{24,80}$/.test(code.value))throw Error(copy.value.invalid);await action()}catch(e:any){message.value=e.message||copy.value.unknown}finally{busy.value=false}}
function verify(){return run(async()=>{verified.value=null;session.value='';consent.value=false;const data=await request('verify',{code:code.value});if(data.can_submit!==true||!['unused','inactive'].includes(data.status)||!Number.isInteger(data.version)||data.version<1||data.destroyed===true)throw Error(copy.value.unavailable);verified.value=data})}
function progress(){return run(async()=>{const data=await request('progress',{code:code.value});message.value=copy.value.states[data.status as keyof typeof copy.value.states]||copy.value.unknown})}
function submit(){return run(async()=>{if(!verified.value||!consent.value)throw Error(copy.value.invalid);let parsed;try{parsed=JSON.parse(session.value)}catch{throw Error(copy.value.invalid)}if(new TextEncoder().encode(session.value).length>65536||typeof parsed.accessToken!=='string'||parsed.accessToken.length<20||typeof parsed.user?.email!=='string'||!parsed.user.email.includes('@'))throw Error(copy.value.invalid);const payload={code:code.value,version:verified.value.version,session:session.value,email:parsed.user.email,consent:true};verified.value=null;try{const data=await request('submit',payload);message.value=data?.submitted===true?copy.value.pending:copy.value.unknown}catch{message.value=copy.value.unknown}finally{session.value='';consent.value=false}})}
</script>
<style scoped>
.redeem-page{max-width:920px;margin:48px auto;padding:24px;color:var(--ui-text-primary)}header{margin-bottom:28px}h1{font-size:32px;font-weight:700}p{line-height:1.8;margin:12px 0}.redeem-panel,aside{padding:28px;border:1px solid var(--ui-border);border-radius:24px;background:var(--ui-bg-elevated);box-shadow:0 12px 40px #00000008}.is-simple .redeem-panel{box-shadow:none;border-radius:12px}aside{margin-top:24px}label{display:block;margin-bottom:18px}input:not([type=checkbox]),textarea{display:block;width:100%;padding:12px;margin-top:8px;border:1px solid var(--ui-border-strong);border-radius:12px;background:var(--ui-bg-page);color:inherit}button{padding:12px 20px;border-radius:12px;background:var(--redeem-accent);color:white;font-weight:600}button:disabled{opacity:.5}.redeem-actions{display:flex;gap:12px;margin-bottom:24px;flex-wrap:wrap}.redeem-consent{display:flex;gap:10px;align-items:flex-start}a{color:var(--redeem-accent)}
</style>
