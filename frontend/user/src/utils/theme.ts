import { computed, ref } from 'vue'
export type ThemeMode = 'light' | 'dark' | 'system'
const key = 'dujiao_theme'
const system = window.matchMedia('(prefers-color-scheme: dark)')
const mode = ref<ThemeMode>('system')
try {
    const value = localStorage.getItem(key)
    if (value === 'light' || value === 'dark' || value === 'system') mode.value = value
} catch { /* Storage unavailable. */ }
const systemDark = ref(system.matches)
const theme = computed(() => mode.value === 'system' ? (systemDark.value ? 'dark' : 'light') : mode.value)
const apply = () => {
    document.documentElement.classList.toggle('dark', theme.value === 'dark')
    document.documentElement.style.colorScheme = theme.value
}
const setTheme = (value: ThemeMode) => {
    mode.value = value
    apply()
    try { localStorage.setItem(key, value) } catch { /* Keep in-memory choice. */ }
}
apply()
system.addEventListener('change', (event) => {
    systemDark.value = event.matches
    apply()
})
export const useTheme = () => ({ theme, mode, setTheme, toggleTheme: () => setTheme(theme.value === 'dark' ? 'light' : 'dark') })
