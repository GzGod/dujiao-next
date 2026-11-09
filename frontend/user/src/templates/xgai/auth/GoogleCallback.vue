<template>
  <div class="xauth__center">
    <div class="xauth__callback">
      <template v-if="loading">
        <span class="xauth__spin" aria-hidden="true" />
        <p>{{ t('auth.googleCallback.processing') }}</p>
      </template>
      <template v-else-if="errMsg">
        <AlertCircle class="xauth__err-icon" />
        <p>{{ errMsg }}</p>
        <RouterLink :to="retryPath" class="x-btn x-btn--primary">{{ t('auth.googleCallback.back') }}</RouterLink>
      </template>
    </div>
    <InviteCodeDialog @success="continueAfterLogin" @cancel="cancelInvite" />
  </div>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import { AlertCircle } from 'lucide-vue-next'
import InviteCodeDialog from '../../../components/InviteCodeDialog.vue'
import { useGoogleRedirectCallback } from '../../../composables/useGoogleRedirectCallback'
import '../styles/xgai-auth.css'

const { t } = useI18n()
const { loading, errMsg, retryPath, continueAfterLogin, cancelInvite } = useGoogleRedirectCallback()
</script>
