import { globalIgnores } from 'eslint/config'
import { defineConfigWithVueTs, vueTsConfigs } from '@vue/eslint-config-typescript'
import pluginVue from 'eslint-plugin-vue'
import pluginOxlint from 'eslint-plugin-oxlint'
import skipFormatting from 'eslint-config-prettier/flat'

// TunnelKit 前端 ESLint 配置，参照 Vue 官方 create-vue 的组合方式。
//
// 分工：oxlint 负责快速的基础检查，ESLint 负责 oxlint 覆盖不到的部分（主要是 Vue 模板规则），
// oxfmt 负责格式化。因此这里用 eslint-plugin-oxlint 依据 .oxlintrc.json 自动关闭重叠规则，
// 再用 eslint-config-prettier 关闭与格式化冲突的规则，避免同一处被报两遍。
export default defineConfigWithVueTs(
  {
    name: 'app/files-to-lint',
    files: ['**/*.{vue,ts,mts,tsx}'],
  },

  // bindings 由 Wails 自动生成，dist 是构建产物，都不参与检查。
  globalIgnores(['**/dist/**', '**/dist-ssr/**', '**/coverage/**', '**/bindings/**']),

  ...pluginVue.configs['flat/essential'],
  vueTsConfigs.recommended,

  ...pluginOxlint.buildFromOxlintConfigFile('.oxlintrc.json'),
  skipFormatting,

  {
    name: 'app/ui-components',
    files: ['src/components/ui/**/*.vue'],
    rules: {
      // shadcn-vue 生成的基础组件使用 Badge、Button、Card 这类单词名，是该体系约定，
      // 项目规则也要求保持生成结构，因此在这里关闭多词组件名要求。
      'vue/multi-word-component-names': 'off',
    },
  },
)
