<template>
  <div class="xauth">
    <div class="xauth__wrap">
      <div class="xauth__bar">
        <RouterLink to="/" class="xauth__back">
          <ArrowLeft aria-hidden="true" /> {{ t('auth.login.backHome') }}
        </RouterLink>
        <span class="xauth__badge">{{ t('navbar.personalCenter') }}</span>
      </div>

      <div class="xauth__card">
        <div class="xauth__head">
          <span class="xauth__eyebrow">{{ brandSiteName }}</span>
          <h1 class="xauth__title">
            {{ step === 'totp' ? t('auth.login.totp.title') : t('auth.login.title') }}
          </h1>
          <p class="xauth__subtitle">
            {{ step === 'totp' ? t('auth.login.totp.subtitle') : t('auth.login.subtitle') }}
          </p>
        </div>

        <!-- 2FA step -->
        <form v-if="step === 'totp'" class="xauth__form" @submit.prevent="handleVerify2FA">
          <div class="xauth__countdown">
            {{ t('auth.login.totp.countdown', { seconds: challengeRemainingSeconds }) }}
          </div>

          <div v-if="totpMode === 'code'" class="xauth__field">
            <label class="xauth__label">{{ t('auth.login.totp.codeLabel') }}</label>
            <input
              v-model="totpCode"
              class="xck__input"
              inputmode="numeric"
              autocomplete="one-time-code"
              maxlength="6"
              style="text-align:center;letter-spacing:.4em"
              :placeholder="t('auth.login.totp.codePlaceholder')"
            />
          </div>
          <div v-else class="xauth__field">
            <label class="xauth__label">{{ t('auth.login.totp.recoveryLabel') }}</label>
            <input
              v-model="recoveryCode"
              class="xck__input"
              autocomplete="off"
              :placeholder="t('auth.login.totp.recoveryPlaceholder')"
            />
          </div>

          <button type="button" class="xauth__mode-link" @click="totpMode = totpMode === 'code' ? 'recovery' : 'code'">
            {{ totpMode === 'code' ? t('auth.login.totp.useRecovery') : t('auth.login.totp.useCode') }}
          </button>

          <div v-if="error" class="xck__alert is-error">{{ error }}</div>

          <button type="submit" class="x-btn x-btn--primary x-btn--block" :disabled="userAuthStore.loading">
            {{ userAuthStore.loading ? t('auth.login.totp.verifying') : t('auth.login.totp.submit') }}
          </button>

          <button type="button" class="xauth__mode-link" @click="cancel2FA">
            {{ t('auth.login.totp.cancel') }}
          </button>
        </form>

        <!-- Password login -->
        <form v-else class="xauth__form" @submit.prevent="handleLogin">
          <!-- Email -->
          <div class="xauth__field">
            <label class="xauth__label">
              <Mail aria-hidden="true" /> {{ t('auth.login.emailLabel') }}
            </label>
            <input
              v-model="email"
              type="email"
              required
              class="xck__input"
              :class="{ 'is-error': formValidation.hasError('email') }"
              :placeholder="t('auth.login.emailPlaceholder')"
              @blur="formValidation.touchField('email', email)"
            />
            <p v-if="formValidation.hasError('email')" class="xauth__field-err">
              {{ formValidation.getError('email') }}
            </p>
          </div>

          <!-- Password -->
          <div class="xauth__field">
            <label class="xauth__label">
              <Lock aria-hidden="true" /> {{ t('auth.login.passwordLabel') }}
            </label>
            <div class="xauth__input-wrap">
              <input
                v-model="password"
                :type="showPassword ? 'text' : 'password'"
                required
                class="xck__input"
                :class="{ 'is-error': formValidation.hasError('password') }"
                :placeholder="t('auth.login.passwordPlaceholder')"
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
          </div>

          <!-- Captcha -->
          <div v-if="loginCaptchaEnabled" class="xauth__field">
            <label class="xauth__label">
              <ShieldCheck aria-hidden="true" /> {{ t('auth.common.captchaLabel') }}
            </label>
            <ImageCaptcha
              v-if="captchaProvider === 'image'"
              ref="imageCaptchaRef"
              v-model="captchaPayload"
              :disabled="userAuthStore.loading"
              @config-stale="handleCaptchaConfigStale"
            />
            <TurnstileCaptcha
              v-else-if="captchaProvider === 'turnstile'"
              ref="turnstileRef"
              v-model="turnstileToken"
              :site-key="turnstileSiteKey"
            />
          </div>

          <!-- Remember / forgot -->
          <div class="xauth__meta">
            <label>
              <input v-model="rememberMe" type="checkbox" />
              {{ t('auth.login.rememberMe') }}
            </label>
            <RouterLink v-if="emailVerificationEnabled" to="/auth/forgot">
              {{ t('auth.login.forgot') }}
            </RouterLink>
          </div>

          <div v-if="info" class="xck__alert is-success-box">{{ info }}</div>
          <div v-if="error" class="xck__alert is-error">{{ error }}</div>

          <button type="submit" class="x-btn x-btn--primary x-btn--block" :disabled="userAuthStore.loading">
            <LogIn v-if="!userAuthStore.loading" aria-hidden="true" />
            {{ userAuthStore.loading ? t('auth.login.submitting') : t('auth.login.submit') }}
          </button>

          <!-- Third-party login -->
          <template v-if="showThirdPartyLogin">
            <div class="xauth__divider">{{ t('auth.login.socialOr') }}</div>
            <div class="xauth__social">
              <template v-if="showTelegramWidget">
                <div ref="telegramWidgetRef" style="display:flex;justify-content:center"></div>
                <p class="xauth__social-text">{{ t('auth.login.telegramHint') }}</p>
              </template>
              <template v-else-if="showTelegramOidc">
                <button type="button" class="x-btn x-btn--ghost x-btn--block" @click="startTelegramOidc">
                  {{ t('auth.login.telegramOidcButton') }}
                </button>
                <p class="xauth__social-text">{{ t('auth.login.telegramOidcHint') }}</p>
              </template>
              <template v-else-if="showMiniAppLoginHint">
                <p class="xauth__social-text">
                  {{ attemptingMiniAppLogin ? t('auth.login.telegramMiniAppLoggingIn') : t('auth.login.telegramMiniAppHint') }}
                </p>
              </template>
              <template v-if="showGoogleLogin">
                <GoogleIdentityButton
                  :client-id="googleClientID"
                  :locale="googleButtonLocale"
                  shape="pill"
                  :ux-mode="googleIdentityUXMode"
                  :login-uri="googleRedirectLoginURI"
                  :prepare-redirect="prepareGoogleRedirectLogin"
                  :disabled="userAuthStore.loading"
                  :loading-label="t('auth.login.googleLoading')"
                  @credential="handleGoogleCredential"
                  @error="handleGoogleScriptError"
                />
                <p class="xauth__social-text">{{ t('auth.login.googleHint') }}</p>
              </template>
            </div>
          </template>

          <template v-if="showTelegramMiniAppEntry">
            <p class="xauth__social-text">{{ t('auth.login.telegramMiniAppEntryHint') }}</p>
            <button type="button" class="x-btn x-btn--ghost x-btn--block" @click="openTelegramMiniAppEntry">
              {{ t('auth.login.telegramMiniAppEntryAction') }}
            </button>
          </template>
        </form>
      </div>

      <div v-if="registrationEnabled" class="xauth__footer">
        <RouterLink :to="registerLink">{{ t('auth.login.noAccount') }}</RouterLink>
        <p v-if="inviteOnlyEnabled" style="margin:6px 0 0;font-size:13px;color:var(--x-ink-4)">
          {{ t('auth.login.inviteOnlyHint') }}
        </p>
      </div>
    </div>
    <InviteCodeDialog @success="continueAfterLogin" />
  </div>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import { ArrowLeft, Eye, EyeOff, Lock, LogIn, Mail, ShieldCheck } from 'lucide-vue-next'
import ImageCaptcha from '../../../components/captcha/ImageCaptcha.vue'
import TurnstileCaptcha from '../../../components/captcha/TurnstileCaptcha.vue'
import GoogleIdentityButton from '../../../components/auth/GoogleIdentityButton.vue'
import InviteCodeDialog from '../../../components/InviteCodeDialog.vue'
import { useLogin } from '../../../composables/useLogin'
import '../styles/xgai-auth.css'

const { t } = useI18n()

const {
  userAuthStore, brandSiteName,
  email, password, showPassword, rememberMe,
  step, totpMode, totpCode, recoveryCode, challengeRemainingSeconds, handleVerify2FA, cancel2FA,
  error, info, formValidation,
  loginCaptchaEnabled, captchaProvider, captchaPayload, turnstileToken, turnstileSiteKey,
  imageCaptchaRef, turnstileRef, handleCaptchaConfigStale,
  registrationEnabled, emailVerificationEnabled, inviteOnlyEnabled, registerLink, continueAfterLogin,
  showTelegramWidget, telegramWidgetRef, showTelegramOidc, startTelegramOidc,
  showMiniAppLoginHint, attemptingMiniAppLogin, showTelegramMiniAppEntry, openTelegramMiniAppEntry,
  googleClientID, googleButtonLocale, googleIdentityUXMode, googleRedirectLoginURI,
  prepareGoogleRedirectLogin, showGoogleLogin, showThirdPartyLogin,
  handleGoogleCredential, handleGoogleScriptError,
  handleLogin,
} = useLogin()

void imageCaptchaRef
void turnstileRef
void telegramWidgetRef
</script>
