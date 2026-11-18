import { ref } from 'vue'
type Theme = 'light' | 'dark'
const key = 'dujiao_theme'
const system = window.matchMedia('(prefers-color-scheme: dark)')
let saved: Theme | null = null
try {
    const value = localStorage.getItem(key)
    if (value === 'light' || value === 'dark') saved = value
} catch { /* Storage unavailable. */ }
const theme = ref<Theme>(saved ?? (system.matches ? 'dark' : 'light'))
const apply = () => {
    document.documentElement.classList.toggle('dark', theme.value === 'dark')
    document.documentElement.style.colorScheme = theme.value
}
apply()
system.addEventListener('change', (event) => {
    if (saved) return
    theme.value = event.matches ? 'dark' : 'light'
    apply()
})
export const useTheme = () => ({
    theme,
    toggleTheme: () => {
        theme.value = theme.value === 'dark' ? 'light' : 'dark'
        saved = theme.value
        apply()
        try { localStorage.setItem(key, theme.value) } catch { /* Keep in-memory choice. */ }
    },
})
