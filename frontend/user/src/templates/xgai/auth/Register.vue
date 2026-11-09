<template>
  <div class="xauth">
    <div class="xauth__wrap xauth__wrap--wide">
      <div class="xauth__bar">
        <RouterLink to="/" class="xauth__back">
          <ArrowLeft aria-hidden="true" /> {{ t('auth.login.backHome') }}
        </RouterLink>
        <span class="xauth__badge">{{ t('auth.register.title') }}</span>
      </div>

      <div class="xauth__card">
        <div v-if="!registrationEnabled" class="xauth__disabled">
          <p>{{ t('auth.register.registrationDisabled') }}</p>
          <RouterLink to="/auth/login">{{ t('auth.register.hasAccount') }}</RouterLink>
        </div>

        <template v-else>
          <div class="xauth__head">
            <span class="xauth__eyebrow">{{ brandSiteName }}</span>
            <h1 class="xauth__title">{{ t('auth.register.title') }}</h1>
            <p class="xauth__subtitle">{{ t('auth.register.subtitle') }}</p>
          </div>

          <form class="xauth__form" @submit.prevent="handleRegister">
            <!-- Invite code -->
            <div v-if="inviteOnlyEnabled" class="xauth__field">
              <label class="xauth__label">
                <Ticket aria-hidden="true" /> {{ t('auth.invite.codeLabel') }}
              </label>
              <input
                v-model="inviteCode"
                type="text"
                required
                autocomplete="off"
                class="xck__input"
                :placeholder="t('auth.invite.codePlaceholder')"
              />
              <p class="xauth__hint">{{ t('auth.invite.registerHint') }}</p>
            </div>

            <!-- Email -->
            <div class="xauth__field">
              <label class="xauth__label">
                <Mail aria-hidden="true" /> {{ t('auth.register.emailLabel') }}
              </label>
              <div v-if="emailDomainSelectionRequired" class="xauth__domain-row">
                <input
                  v-model="emailLocalPart"
                  type="text"
                  required
                  autocomplete="username"
                  class="xck__input"
                  :class="{ 'is-error': formValidation.hasError('email') }"
                  :placeholder="t('auth.register.emailLocalPlaceholder')"
                  @blur="touchRegistrationEmail"
                />
                <select
                  v-model="selectedEmailDomain"
                  class="xauth__domain-select"
                  :class="{ 'is-error': formValidation.hasError('email') }"
                  @change="touchRegistrationEmail"
                >
                  <option v-for="domain in allowedEmailDomains" :key="domain" :value="domain">@{{ domain }}</option>
                </select>
              </div>
              <input
                v-else
                v-model="email"
                type="email"
                required
                class="xck__input"
                :class="{ 'is-error': formValidation.hasError('email') }"
                :placeholder="t('auth.register.emailPlaceholder')"
                @blur="touchRegistrationEmail"
              />
              <p v-if="formValidation.hasError('email')" class="xauth__field-err">
                {{ formValidation.getError('email') }}
              </p>
              <p v-else-if="emailDomainSelectionRequired" class="xauth__hint">
                {{ t('auth.register.emailDomainSelectHint') }}
              </p>
              <p v-else-if="emailDomainAllowlistEnabled" class="xauth__hint">
                {{ allowedEmailDomains.length > 0
                  ? t('auth.register.allowedEmailDomainsHint', { domains: allowedEmailDomainsText })
                  : t('auth.register.noAllowedEmailDomainsHint') }}
              </p>
            </div>

            <!-- Password -->
            <div class="xauth__field">
              <label class="xauth__label">
                <Lock aria-hidden="true" /> {{ t('auth.register.passwordLabel') }}
              </label>
              <div class="xauth__input-wrap">
                <input
                  v-model="password"
                  :type="showPassword ? 'text' : 'password'"
                  required
                  class="xck__input"
                  :class="{ 'is-error': formValidation.hasError('password') }"
                  :placeholder="t('auth.register.passwordPlaceholder')"
                  @blur="formValidation.touchField('password', password)"
                />
                <button
                  type="button"
                  class="xauth__eye"
                  :aria-label="showPassword ? t('auth.common.hidePassword') : t('auth.common.showPassword')"
                  @click="showPassword = !showPassword"
                >
                  <EyeOff v-if="showPassword" aria-hidden="true" /><Eye v-else aria-hidden="true" />
                </button>
              </div>
              <p v-if="formValidation.hasError('password')" class="xauth__field-err">
                {{ formValidation.getError('password') }}
              </p>
              <div v-if="password && !formValidation.hasError('password')" class="xauth__strength">
                <div class="xauth__strength-bars">
                  <span :class="`is-${passwordStrength}`" />
                  <span :class="passwordStrength !== 'weak' ? `is-${passwordStrength}` : ''" />
                  <span :class="passwordStrength === 'strong' ? 'is-strong' : ''" />
                </div>
                <span class="xauth__strength-label" :class="`is-${passwordStrength}`">
                  {{ t(`formValidation.passwordStrength.${passwordStrength}`) }}
                </span>
              </div>
            </div>

            <!-- Captcha -->
            <div v-if="emailVerificationEnabled && sendCodeCaptchaEnabled" class="xauth__field">
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

            <!-- Email code -->
            <div v-if="emailVerificationEnabled" class="xauth__field">
              <label class="xauth__label">
                <ShieldCheck aria-hidden="true" /> {{ t('auth.register.codeLabel') }}
              </label>
              <div class="xauth__code-row">
                <input
                  v-model="code"
                  type="text"
                  required
                  class="xck__input"
                  :placeholder="t('auth.register.codePlaceholder')"
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

            <!-- Agreement -->
            <label class="xauth__agree">
              <input v-model="agreed" type="checkbox" />
              <span>
                {{ t('auth.register.agreementPrefix') }}
                <RouterLink to="/privacy" target="_blank" rel="noopener noreferrer">{{ t('footer.privacy') }}</RouterLink>
                {{ t('auth.register.agreementAnd') }}
                <RouterLink to="/terms" target="_blank" rel="noopener noreferrer">{{ t('footer.terms') }}</RouterLink>
              </span>
            </label>

            <div v-if="error" class="xck__alert is-error">{{ error }}</div>

            <button type="submit" class="x-btn x-btn--primary x-btn--block" :disabled="userAuthStore.loading || !agreed">
              <UserPlus v-if="!userAuthStore.loading" aria-hidden="true" />
              {{ userAuthStore.loading ? t('auth.register.creating') : t('auth.register.create') }}
            </button>
          </form>
        </template>
      </div>

      <div class="xauth__footer">
        <RouterLink :to="loginLink">{{ t('auth.register.hasAccount') }}</RouterLink>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import { ArrowLeft, Eye, EyeOff, Lock, Mail, ShieldCheck, Ticket, UserPlus } from 'lucide-vue-next'
import ImageCaptcha from '../../../components/captcha/ImageCaptcha.vue'
import TurnstileCaptcha from '../../../components/captcha/TurnstileCaptcha.vue'
import { useRegister } from '../../../composables/useRegister'
import '../styles/xgai-auth.css'

const { t } = useI18n()

const {
  userAuthStore, brandSiteName,
  email, emailLocalPart, selectedEmailDomain, password, showPassword, code, agreed, inviteCode, inviteOnlyEnabled, loginLink,
  passwordStrength, error, sending, countdown,
  captchaPayload, turnstileToken, imageCaptchaRef, turnstileRef,
  captchaProvider, sendCodeCaptchaEnabled, turnstileSiteKey,
  registrationEnabled, emailVerificationEnabled,
  emailDomainAllowlistEnabled, allowedEmailDomains, allowedEmailDomainsText, emailDomainSelectionRequired,
  touchRegistrationEmail, formValidation, handleCaptchaConfigStale, handleSendCode, handleRegister,
} = useRegister()

void imageCaptchaRef
void turnstileRef
</script>
