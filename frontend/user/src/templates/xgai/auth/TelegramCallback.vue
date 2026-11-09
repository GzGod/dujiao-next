<template>
  <div class="xauth__center">
    <div class="xauth__callback">
      <template v-if="loading">
        <span class="xauth__spin" aria-hidden="true" />
        <p>{{ t('auth.telegramCallback.processing') }}</p>
      </template>
      <template v-else-if="errMsg">
        <AlertCircle class="xauth__err-icon" />
        <p>{{ errMsg }}</p>
        <RouterLink to="/auth/login" class="x-btn x-btn--primary">{{ t('auth.telegramCallback.backToLogin') }}</RouterLink>
      </template>
    </div>
    <InviteCodeDialog @success="continueAfterLogin" @cancel="cancelInvite" />
  </div>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import { AlertCircle } from 'lucide-vue-next'
import InviteCodeDialog from '../../../components/InviteCodeDialog.vue'
import { useTelegramCallback } from '../../../composables/useTelegramCallback'
import '../styles/xgai-auth.css'

const { t } = useI18n()
const { loading, errMsg, continueAfterLogin, cancelInvite } = useTelegramCallback()
</script>
