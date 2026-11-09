import { mount } from '@vue/test-utils'
import { defineComponent, h, nextTick, ref } from 'vue'
import { describe, expect, it, vi } from 'vite-plus/test'
import AppButton from '@/components/base/button/AppButton.vue'
import AppInput from '@/components/base/AppInput.vue'
import AppTextarea from '@/components/base/AppTextarea.vue'
import AppDialogContent from '@/components/base/AppDialogContent.vue'
import AppDropdownMenuContent from '@/components/base/AppDropdownMenuContent.vue'
import AppDropdownMenuItem from '@/components/base/AppDropdownMenuItem.vue'
import { DropdownMenu, DropdownMenuTrigger } from '@/components/ui/dropdown-menu'
import AppTabsList from '@/components/base/AppTabsList.vue'
import AppTabsTrigger from '@/components/base/AppTabsTrigger.vue'
import { Dialog, DialogTitle } from '@/components/ui/dialog'
import { Tabs, TabsContent } from '@/components/ui/tabs'

describe('project base component contracts', () => {
  it('preserves button asChild semantics and disabled state without nesting buttons', async () => {
    const click = vi.fn()
    const wrapper = mount(AppButton, {
      props: { asChild: true },
      attrs: { onClick: click },
      slots: { default: '<a href="#test">链接</a>' },
    })
    expect(wrapper.findAll('button')).toHaveLength(0)
    await wrapper.get('a').trigger('click')
    expect(click).toHaveBeenCalledOnce()
    wrapper.unmount()
    const disabled = mount(AppButton, { attrs: { disabled: true }, slots: { default: '禁用' } })
    expect(disabled.get('button').attributes('disabled')).toBeDefined()
    disabled.unmount()
  })

  it.each([AppInput, AppTextarea])(
    'delegates initial values, model updates and native attributes to the official field',
    async (component) => {
      const value = ref('initial')
      const Host = defineComponent({
        setup: () => () =>
          h(component, {
            modelValue: value.value,
            'onUpdate:modelValue': (next: string | number) => {
              value.value = String(next)
            },
            'aria-invalid': true,
            autocomplete: 'off',
            class: 'custom-field',
          }),
      })
      const wrapper = mount(Host)
      const field = wrapper.get('input, textarea')
      expect((field.element as HTMLInputElement).value).toBe('initial')
      expect(field.attributes('aria-invalid')).toBe('true')
      expect(field.classes()).toContain('custom-field')
      await field.setValue('edited')
      expect(value.value).toBe('edited')
      value.value = 'server'
      await nextTick()
      expect((field.element as HTMLInputElement).value).toBe('server')
      wrapper.unmount()
    },
  )

  it('preserves menu select cancellation and disabled semantics through the official item', async () => {
    const select = vi.fn((event: Event) => event.preventDefault())
    const Host = defineComponent({
      setup: () => () =>
        h(
          DropdownMenu,
          { defaultOpen: true },
          {
            default: () => [
              h(DropdownMenuTrigger, {}, { default: () => '菜单' }),
              h(
                AppDropdownMenuContent,
                { 'data-testid': 'menu' },
                {
                  default: () => [
                    h(AppDropdownMenuItem, { onSelect: select }, { default: () => '保留菜单' }),
                    h(AppDropdownMenuItem, { disabled: true }, { default: () => '禁用项' }),
                  ],
                },
              ),
            ],
          },
        ),
    })
    const wrapper = mount(Host, { attachTo: document.body })
    await nextTick()
    const item = document.querySelector('[role="menuitem"]')!
    item.dispatchEvent(new MouseEvent('click', { bubbles: true }))
    await nextTick()
    expect(select).toHaveBeenCalledOnce()
    expect(document.querySelector('[data-testid="menu"]')).not.toBeNull()
    expect(document.querySelector('[aria-disabled="true"]')).not.toBeNull()
    wrapper.unmount()
  })

  it('retains tabs state, scoped root slot and hidden form drafts using the official root', async () => {
    const Host = defineComponent({
      components: { Tabs, TabsContent, AppTabsList, AppTabsTrigger },
      template: `
      <Tabs default-value="first" :unmount-on-hide="false" v-slot="{ modelValue }">
        <span data-value>{{ modelValue }}</span>
        <AppTabsList aria-label="配置分区">
          <AppTabsTrigger value="first">一</AppTabsTrigger>
          <AppTabsTrigger value="second">二</AppTabsTrigger>
          <AppTabsTrigger value="disabled" disabled>禁用</AppTabsTrigger>
        </AppTabsList>
        <TabsContent value="first"><input value="draft" /></TabsContent>
        <TabsContent value="second">内容二</TabsContent>
      </Tabs>`,
    })
    const wrapper = mount(Host, { attachTo: document.body })
    const draft = wrapper.get('input')
    await draft.setValue('unsaved')
    await wrapper.findAll('[role="tab"]')[1]!.trigger('mousedown', { button: 0, ctrlKey: false })
    expect(wrapper.get('[data-value]').text()).toBe('second')
    expect((draft.element as HTMLInputElement).value).toBe('unsaved')
    expect(wrapper.findAll('[role="tab"]')[2]!.attributes('disabled')).toBeDefined()
    expect(wrapper.findAll('[role="tab"]')[1]!.attributes('aria-selected')).toBe('true')
    wrapper.unmount()
  })

  it('forwards portal content attributes and cancellable escape events, and supports hiding close', async () => {
    const escape = vi.fn((event: KeyboardEvent) => event.preventDefault())
    const Host = defineComponent({
      setup: () => () =>
        h(
          Dialog,
          { defaultOpen: true },
          {
            default: () =>
              h(
                AppDialogContent,
                { showCloseButton: false, 'data-testid': 'content', onEscapeKeyDown: escape },
                {
                  default: () => h(DialogTitle, {}, { default: () => '标题' }),
                },
              ),
          },
        ),
    })
    const wrapper = mount(Host, { attachTo: document.body })
    await nextTick()
    const content = document.querySelector('[data-testid="content"]')!
    expect(content).not.toBeNull()
    expect(content.querySelector('[data-slot="dialog-close"]')).toBeNull()
    content.dispatchEvent(
      new KeyboardEvent('keydown', { key: 'Escape', bubbles: true, cancelable: true }),
    )
    expect(escape).toHaveBeenCalledOnce()
    await nextTick()
    expect(document.querySelector('[role="dialog"]')).not.toBeNull()
    wrapper.unmount()
  })
})
