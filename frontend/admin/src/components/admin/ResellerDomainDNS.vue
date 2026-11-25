<script setup lang="ts">
import { reactive, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { adminAPI } from '@/api/admin'
import type { AdminResellerDomain } from '@/api/types'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { notifyError, notifySuccess } from '@/utils/notify'
const props = defineProps<{ domain: AdminResellerDomain }>()
const emit = defineEmits<{ saved: [] }>()
const { t } = useI18n()
const key = 'admin.resellerProfileDetail.dns'
const saving = ref(false)
const form = reactive({ cname: '', record_name: '@', txt_host: '', txt_value: '', status: 'pending_dns', updated_at: '' })
watch(() => props.domain, domain => Object.assign(form, { cname: '', record_name: '@', txt_host: '', txt_value: '', status: 'pending_dns', updated_at: '' }, domain.dns || {}), { immediate: true })
async function save() {
  saving.value = true
  try {
    const { updated_at: _updatedAt, ...payload } = form
    await adminAPI.updateResellerDomainDNS(props.domain.id, payload)
    notifySuccess(t(`${key}.saved`))
    emit('saved')
  } catch (error) { notifyError(error instanceof Error ? error.message : t('admin.common.saveFailed')) } finally { saving.value = false }
}
</script>
<template>
  <form class="space-y-3 rounded-xl border border-border bg-card p-4" @submit.prevent="save">
    <h3 class="font-semibold">{{ t(`${key}.title`) }} · {{ domain.domain }}</h3>
    <p class="text-xs text-muted-foreground">{{ t(`${key}.hint`) }}</p>
    <div class="grid gap-3 md:grid-cols-2">
      <div v-for="field in (['cname', 'record_name', 'txt_host', 'txt_value'] as const)" :key="field" class="space-y-1">
        <Label :for="`dns-${domain.id}-${field}`">{{ t(`${key}.${({ cname: 'cname', record_name: 'name', txt_host: 'txtHost', txt_value: 'txtValue' })[field]}`) }}</Label>
        <Input :id="`dns-${domain.id}-${field}`" v-model.trim="form[field]" :required="field === 'cname' || field === 'record_name'" :maxlength="field === 'txt_value' ? 512 : 253" />
      </div>
    </div>
    <Label>{{ t(`${key}.status`) }}</Label>
    <Select v-model="form.status"><SelectTrigger><SelectValue /></SelectTrigger><SelectContent>
      <SelectItem value="pending_dns">{{ t(`${key}.pendingDNS`) }}</SelectItem><SelectItem value="pending_tls">{{ t(`${key}.pendingTLS`) }}</SelectItem><SelectItem value="ready">{{ t(`${key}.ready`) }}</SelectItem>
    </SelectContent></Select>
    <Button type="submit" :disabled="saving || !form.cname">{{ t(`${key}.save`) }}</Button>
  </form>
</template>
