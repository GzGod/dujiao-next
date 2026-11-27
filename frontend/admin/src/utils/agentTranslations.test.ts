import { describe, expect, it } from 'vitest'
import i18n from '../i18n'

describe('agent management translation paths', () => {
  for (const locale of ['zh-CN', 'zh-TW', 'en-US'] as const) {
    it(`resolves the menu and workspace labels in ${locale}`, () => {
      for (const key of ['agentManagement.title', 'agentManagement.sites', 'agentApproval.title', 'agentApproval.confirm', 'resellerWorkspace.listTitle']) {
        expect(i18n.global.te(key, locale)).toBe(true)
      }
    })
  }
})
