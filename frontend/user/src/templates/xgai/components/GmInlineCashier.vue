<template>
  <section class="gm-inline" :aria-label="t('payment.inlineGmCashier')">
    <div class="gm-inline__controls">
      <div class="gm-inline__network-picker">
        <button type="button" class="gm-inline__network-trigger" :aria-expanded="networkMenu" @click="networkMenu = !networkMenu"><span class="gm-inline__chain-dot" :style="{background: chainColor(network)}" />{{ networkName(network) }}<ChevronDown :size="13" /></button>
        <div v-if="networkMenu" class="gm-inline__network-menu"><button v-for="asset in assets" :key="asset.network" type="button" :aria-pressed="network === asset.network" @click="pickNetwork(asset.network)"><span class="gm-inline__chain-dot" :style="{background:chainColor(asset.network)}" />{{ networkName(asset.network) }}<Check v-if="network===asset.network" :size="13" /></button></div>
      </div>
      <div class="gm-inline__tokens" :aria-label="t('gmInline.token')"><button v-for="item in tokens" :key="item" type="button" :aria-pressed="token===item" :disabled="busy" @click="token=item"><img :src="'/images/payments/'+item.toLowerCase()+'.svg'" alt="" />{{ item }}</button></div>
    </div>
    <Transition name="gm-content" mode="out-in">
      <div v-if="selected && network===selected.network && token===selected.token" key="payment" class="gm-inline__payment">
        <div class="gm-inline__amount"><span>{{ t('gmInline.exactAmount') }}</span><strong>{{ selected.actual_amount }}<small>{{ selected.token }}</small></strong></div>
        <div class="gm-inline__qr-wrap"><img v-if="qr" class="gm-inline__qr" :src="qr" :alt="t('gmInline.qr')" /><span class="gm-inline__scan-line" aria-hidden="true" /></div>
        <p class="gm-inline__route"><span class="gm-inline__chain-dot" :style="{background:chainColor(selected.network)}" />{{ networkName(selected.network) }} · {{ selected.token }}</p>
        <button type="button" class="gm-inline__address" @click="copy" :aria-label="t('payment.copyWalletAddress')"><code>{{ selected.receive_address }}</code><Check v-if="copied" :size="15" /><Copy v-else :size="15" /></button>
        <div class="gm-inline__source"><button type="button" :aria-pressed="source==='wallet'" @click="source='wallet'">{{ t('gmInline.wallet') }}</button><span>·</span><button type="button" :aria-pressed="source==='exchange'" @click="source='exchange'">{{ t('gmInline.exchange') }}</button></div>
        <p class="gm-inline__note" :class="{'gm-inline__note--exchange':source==='exchange'}">{{ t(source==='exchange'?'gmInline.exchangeHint':'gmInline.walletHint') }}</p>
      </div>
      <div v-else key="selection" class="gm-inline__intro"><div class="gm-inline__coin-art"><img src="/images/payments/usdt.svg" alt="USDT" /><img src="/images/payments/usdc.svg" alt="USDC" /></div><h3>{{ t('gmInline.selectTitle') }}</h3><p>{{ t('gmInline.selectHint') }}</p><button type="button" class="x-btn x-btn--primary gm-inline__confirm" :disabled="busy || !network || !token" @click="choose">{{ t(busy?'common.loading':'gmInline.getAddress') }}<ArrowRight :size="15" /></button></div>
    </Transition>

    <p v-if="error" role="alert" class="gm-inline__error">{{ error }} <button type="button" @click="load">{{ t('gmInline.retry') }}</button></p>
  </section>
</template>

<script setup lang="ts">
import { computed, onMounted, ref, onBeforeUnmount } from 'vue'
import { useI18n } from 'vue-i18n'
import QRCode from 'qrcode'
import { ChevronDown, Check, Copy, ArrowRight } from 'lucide-vue-next'
import { gmCashierAPI } from '../../../api/order'
const props = defineProps<{orderNo:string;guest:boolean;email:string;password:string}>()
const emit = defineEmits<{ready:[]}>()
const { t } = useI18n()
let disposed = false
onBeforeUnmount(() => { disposed = true })
const assets = ref<{network:string;tokens:string[]}[]>([])
const network=ref(''), token=ref(''), busy=ref(false),error=ref(''),qr=ref(''),copied=ref(false)
const selected=ref<any>(null), source=ref('wallet')
const networkMenu=ref(false)
const pickNetwork=(value:string)=>{network.value=value;token.value=tokens.value.includes(token.value)?token.value:(tokens.value[0] || '');networkMenu.value=false}
const chainColor=(value:string)=>({base:'#0052ff',binance:'#efb90b',bsc:'#efb90b',ethereum:'#7289da',tron:'#e84142',polygon:'#8247e5',arbitrum:'#28a0f0',optimism:'#ff0420'}[value] || '#7e8ba5')
const tokens=computed(()=>assets.value.find(a=>a.network===network.value)?.tokens || [])
const auth=()=>props.guest?{email:props.email,order_password:props.password}:undefined
const networkName=(n:string)=>({binance:'BNB Smart Chain',bsc:'BNB Smart Chain',base:'Base',ethereum:'Ethereum',eth:'Ethereum',tron:'TRON',polygon:'Polygon',arbitrum:'Arbitrum',optimism:'Optimism'}[n] || n)
const apply=async(data:any)=>{
 assets.value=(data.supported_assets || []).map((a:any)=>({...a,tokens:(a.tokens||[]).filter((v:string)=>['USDT','USDC'].includes(v))})).filter((a:any)=>a.tokens.length)
 if (disposed) return
 const o=data.order
 if (o && o.status===1 && o.receive_address && Number(o.actual_amount)>0 && ['USDT','USDC'].includes(o.token)) {
  const image=await QRCode.toDataURL(String(o.receive_address),{width:280,margin:2,errorCorrectionLevel:'M'})
  if (disposed) return
  selected.value=o;qr.value=image;network.value=o.network;token.value=o.token
 } else {selected.value=null;qr.value='';network.value=assets.value[0]?.network || '';token.value=tokens.value[0] || ''}
}
const load=async()=>{if(busy.value)return;busy.value=true;error.value='';try{await apply((await gmCashierAPI.load(props.orderNo,auth())).data.data)}catch{error.value=t('gmInline.failed')}finally{busy.value=false;emit('ready')}}
const choose=async()=>{if(busy.value)return;busy.value=true;error.value='';selected.value=null;qr.value='';try{await apply((await gmCashierAPI.select(props.orderNo,network.value,token.value,auth())).data.data)}catch{error.value=t('gmInline.failed')}finally{busy.value=false;emit('ready')}}
const copy=async()=>{try{await navigator.clipboard.writeText(selected.value.receive_address);copied.value=true}catch{copied.value=false}}
onMounted(load)
</script>
<style scoped>
.gm-inline{display:flex;flex:1;min-height:0;width:100%;flex-direction:column;align-items:stretch;gap:10px;padding:4px 0;color:var(--x-ink);position:relative}
.gm-inline__controls{display:flex;align-items:center;justify-content:space-between;gap:8px;position:relative;z-index:4}
.gm-inline__network-picker{position:relative;min-width:0}.gm-inline__network-trigger{display:flex;align-items:center;gap:7px;border:0;background:transparent;padding:8px 0;font-size:12px;color:var(--x-ink);font-weight:600;cursor:pointer;white-space:nowrap}.gm-inline__chain-dot{display:inline-block;width:8px;height:8px;border-radius:50%;flex:none}
.gm-inline__network-menu{position:absolute;top:100%;left:0;min-width:185px;max-height:210px;overflow:auto;padding:5px;background:var(--x-bg);border:1px solid var(--x-line);border-radius:12px;box-shadow:0 12px 30px #0002}.gm-inline__network-menu button{display:flex;align-items:center;gap:8px;width:100%;padding:10px;border:0;background:transparent;text-align:left;font-size:12px;color:var(--x-ink);border-radius:7px;cursor:pointer}.gm-inline__network-menu button:hover,.gm-inline__network-menu button[aria-pressed=true]{background:var(--x-blue-soft)}.gm-inline__network-menu svg{margin-left:auto}
.gm-inline__tokens{display:flex;gap:2px;border-bottom:1px solid var(--x-line);flex:none}.gm-inline__tokens button{display:flex;gap:5px;align-items:center;font-size:11px;font-weight:600;padding:8px 7px;border:0;border-bottom:2px solid transparent;background:transparent;color:var(--x-ink-4);cursor:pointer}.gm-inline__tokens img{width:17px;height:17px}.gm-inline__tokens button[aria-pressed=true]{border-bottom-color:var(--x-blue);color:var(--x-ink)}
.gm-inline__payment{display:flex;flex-direction:column;align-items:center;gap:8px}.gm-inline__amount{display:grid;text-align:center;gap:2px;margin-top:4px}.gm-inline__amount>span{font-size:10px;color:var(--x-ink-4)}.gm-inline__amount strong{font-size:30px;line-height:1.3;letter-spacing:-1.1px;font-variant-numeric:tabular-nums;font-weight:650}.gm-inline__amount small{font-size:12px;letter-spacing:0;margin-left:6px;font-weight:550;color:var(--x-ink-3)}
.gm-inline__qr-wrap{position:relative;display:grid;place-items:center;background:#fff;padding:8px;border-radius:10px}.gm-inline__qr{display:block;width:min(145px,22dvh);height:auto}.gm-inline__route{display:flex;align-items:center;gap:5px;font-size:10px;color:var(--x-ink-3);margin:0}.gm-inline__address{display:flex;align-items:center;gap:10px;max-width:100%;width:100%;border:0;border-bottom:1px solid var(--x-line);padding:7px 0 10px;background:transparent;color:var(--x-ink-2);cursor:pointer}.gm-inline__address code{font-size:10px;overflow-wrap:anywhere;min-width:0;flex:1;line-height:1.5;text-align:center}.gm-inline__address svg{flex:none;color:var(--x-blue)}
.gm-inline__source{display:flex;align-items:center;gap:8px;font-size:10px;color:var(--x-ink-4);margin-top:2px}.gm-inline__source button{border:0;background:transparent;color:var(--x-ink-4);font-size:10px;padding:3px;cursor:pointer}.gm-inline__source button[aria-pressed=true]{color:var(--x-ink);font-weight:650}.gm-inline__note{font-size:10px;line-height:1.6;margin:0;text-align:center;color:var(--x-ink-4)}.gm-inline__note--exchange{color:#b87816;font-weight:600}.gm-inline__error{color:#c74747;font-size:12px}
.gm-inline__intro{display:flex;flex:1;min-height:270px;flex-direction:column;align-items:center;justify-content:center;text-align:center;gap:12px}.gm-inline__intro h3{font-size:18px;margin:8px 0 0;font-weight:600;letter-spacing:-.3px}.gm-inline__intro p{font-size:12px;line-height:1.7;color:var(--x-ink-4);max-width:220px;margin:0}.gm-inline__coin-art{display:flex;padding-right:12px;align-items:center}.gm-inline__coin-art img{width:54px;height:54px;filter:drop-shadow(0 8px 12px #0001)}.gm-inline__coin-art img+img{margin-left:-12px;transform:translateY(8px)}.gm-inline__confirm{margin-top:12px;display:flex;align-items:center;justify-content:center;gap:16px;font-size:12px;width:100%;min-height:40px}.gm-inline__update{position:absolute;bottom:0;left:0;right:0;font-size:12px;z-index:3}
.gm-content-enter-active,.gm-content-leave-active{transition:opacity .24s ease,transform .24s ease}.gm-content-enter-from{opacity:0;transform:translateY(8px)}.gm-content-leave-to{opacity:0;transform:translateY(-5px)}
@media(max-height:720px){.gm-inline__payment{gap:5px}.gm-inline__amount strong{font-size:25px}.gm-inline__qr{width:140px}.gm-inline__intro{min-height:240px}}
@media(prefers-reduced-motion:reduce){.gm-content-enter-active,.gm-content-leave-active{transition:none}}
</style>

<style scoped>
.gm-inline { flex: 1 1 0%; min-height: 0; gap: 7px; }
.gm-inline__controls { flex: none; }
.gm-inline__payment { flex: 1 1 0%; min-height: 0; gap: 6px; }
.gm-inline__amount, .gm-inline__route, .gm-inline__address, .gm-inline__source, .gm-inline__note { flex: none; }
.gm-inline__amount { margin-top: 0; }
.gm-inline__amount strong { font-size: 28px; }
.gm-inline__qr-wrap { flex: 1 1 130px; min-height: 72px; max-height: 160px; width: 160px; background: transparent; padding: 0; }
.gm-inline__qr { width: auto; height: 100%; max-width: 100%; object-fit: contain; border-radius: 8px; }
.gm-inline__address { padding: 5px 0 7px; }
.gm-inline__source { margin: 0; }
.gm-inline__note { width: 100%; line-height: 1.55; font-size: 11px; padding-top: 2px; }
.gm-inline__intro { min-height: 0; }
</style>

<style scoped>
.gm-inline__qr-wrap { position:relative; overflow:hidden; isolation:isolate; flex:1 1 140px; width:100%; min-height:72px; max-height:160px; }
.gm-inline__qr { position:absolute; inset:0; display:block; width:100%; height:100%; max-width:100%; max-height:100%; object-fit:contain; border-radius:0; }
.gm-inline__route { position:relative; margin:0; padding-top:2px; line-height:16px; }
.gm-inline__address { position:relative; z-index:1; min-height:32px; flex:none; }
</style>

<style scoped>
.gm-inline__qr-wrap { min-height:128px; }
</style>

<style scoped>
/* Reserve the same two-line footprint for both sources, keeping the QR stable. */
.gm-inline__note { color:var(--x-ink-4); font-size:11px; line-height:17px; min-height:36px; height:36px; padding-top:2px; margin:0; display:flex; align-items:flex-start; justify-content:center; text-wrap:pretty; }
.gm-inline__note--exchange { color:#dc3545; font-weight:600; }
.gm-inline__qr-wrap { flex:0 0 140px; min-height:140px; max-height:140px; }

</style>
