import { ref } from 'vue'

type Theme = 'light' | 'dark'

// 雪糕AI 只提供浅色（蓝 + 白）主题：固定为 light，并清掉旧版本存下的 dark 状态。
const theme = ref<Theme>('light')

if (typeof document !== 'undefined') {
    document.documentElement.classList.remove('dark')
    try {
        localStorage.removeItem('dujiao_theme')
    } catch {
        // 隐私模式下 localStorage 不可用时忽略
    }
}

export const useTheme = () => {
    // 保留函数签名供既有组件调用；主题固定为浅色，切换不再生效。
    const toggleTheme = () => {}

    return {
        theme,
        toggleTheme
    }
}
