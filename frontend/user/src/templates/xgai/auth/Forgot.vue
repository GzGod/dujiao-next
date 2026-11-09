<template>
  <div class="xauth">
    <div class="xauth__wrap">
      <div class="xauth__bar">
        <RouterLink to="/" class="xauth__back">
          <ArrowLeft aria-hidden="true" /> {{ t('auth.login.backHome') }}
        </RouterLink>
        <span class="xauth__badge">{{ t('auth.forgot.title') }}</span>
      </div>

      <div class="xauth__card">
        <div class="xauth__head">
          <span class="xauth__eyebrow">{{ brandSiteName }}</span>
          <h1 class="xauth__title">{{ t('auth.forgot.title') }}</h1>
          <p class="xauth__subtitle">{{ t('auth.forgot.subtitle') }}</p>
        </div>

        <div v-if="!emailVerificationEnabled" class="xck__alert is-error" style="text-align:center">
          <p style="margin:0;font-weight:600">{{ t('auth.forgot.disabled') }}</p>
          <RouterLink to="/auth/login" class="xauth__back" style="margin-top:12px">{{ t('auth.forgot.backLogin') }}</RouterLink>
        </div>

        <form v-else class="xauth__form" @submit.prevent="handleReset">
          <!-- Email -->
          <div class="xauth__field">
            <label class="xauth__label">
              <Mail aria-hidden="true" /> {{ t('auth.forgot.emailLabel') }}
            </label>
            <input
              v-model="email"
              type="email"
              required
              class="xck__input"
              :placeholder="t('auth.forgot.emailPlaceholder')"
            />
          </div>

          <!-- Captcha -->
          <div v-if="sendCodeCaptchaEnabled" class="xauth__field">
            <label class="xauth__label">
              <ShieldCheck aria-hidden="true" /> {{ t('auth.common.captchaLabel') }}
            </label>
            <ImageCaptcha
              v-if="captchaProvider === 'image'"
              ref="imageCaptchaRef"
              v-model="captchaPayload"
              :disabled="sending || countdown > 0"
              @config-stale="handleCaptchaConfigStale"
            />
            <TurnstileCaptcha
              v-else-if="captchaProvider === 'turnstile'"
              ref="turnstileRef"
              v-model="turnstileToken"
              :site-key="turnstileSiteKey"
            />
          </div>

          <!-- Code -->
          <div class="xauth__field">
            <label class="xauth__label">
              <ShieldCheck aria-hidden="true" /> {{ t('auth.forgot.codeLabel') }}
            </label>
            <div class="xauth__code-row">
              <input
                v-model="code"
                type="text"
                required
                class="xck__input"
                :placeholder="t('auth.forgot.codePlaceholder')"
              />
              <button
                type="button"
                class="xauth__send-btn"
                :disabled="sending || countdown > 0"
                @click="handleSendCode"
              >
                {{ countdown > 0 ? t('auth.common.countdown', { seconds: countdown }) : t('auth.common.sendCode') }}
              </button>
            </div>
          </div>

          <!-- New password -->
          <div class="xauth__field">
            <label class="xauth__label">
              <KeyRound aria-hidden="true" /> {{ t('auth.forgot.newPasswordLabel') }}
            </label>
            <input
              v-model="newPassword"
              type="password"
              required
              class="xck__input"
              :placeholder="t('auth.forgot.newPasswordPlaceholder')"
            />
          </div>

          <div v-if="error" class="xck__alert is-error">{{ error }}</div>

          <button type="submit" class="x-btn x-btn--primary x-btn--block" :disabled="userAuthStore.loading">
            <RotateCw v-if="!userAuthStore.loading" aria-hidden="true" />
            {{ userAuthStore.loading ? t('auth.forgot.submitting') : t('auth.forgot.submit') }}
          </button>
        </form>
      </div>

      <div class="xauth__footer">
        <RouterLink to="/auth/login">{{ t('auth.forgot.backLogin') }}</RouterLink>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import { ArrowLeft, KeyRound, Mail, RotateCw, ShieldCheck } from 'lucide-vue-next'
import ImageCaptcha from '../../../components/captcha/ImageCaptcha.vue'
import TurnstileCaptcha from '../../../components/captcha/TurnstileCaptcha.vue'
import { useForgot } from '../../../composables/useForgot'
import '../styles/xgai-auth.css'

const { t } = useI18n()

const {
  userAuthStore, brandSiteName, emailVerificationEnabled,
  email, code, newPassword, error, sending, countdown,
  captchaPayload, turnstileToken, imageCaptchaRef, turnstileRef,
  captchaProvider, sendCodeCaptchaEnabled, turnstileSiteKey,
  handleCaptchaConfigStale, handleSendCode, handleReset,
} = useForgot()

void imageCaptchaRef
void turnstileRef
</script>
