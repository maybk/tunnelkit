const EDITABLE_TAGS = new Set(['INPUT', 'TEXTAREA', 'SELECT'])

export function isEditableElement(target: EventTarget | null): boolean {
  if (!target || typeof target !== 'object' || !('tagName' in target)) {
    return false
  }
  const el = target as HTMLElement
  if (EDITABLE_TAGS.has(el.tagName)) return true
  if (el.isContentEditable) return true
  return typeof el.closest === 'function' && Boolean(el.closest('[contenteditable="true"]'))
}

export function hasOpenOverlay(root: Pick<Document, 'querySelector'> | null): boolean {
  if (!root || typeof root.querySelector !== 'function') return false
  return Boolean(
    root.querySelector(
      '[data-slot="dialog-content"][data-state="open"], [data-slot="alert-dialog-content"][data-state="open"], [data-slot="sheet-content"][data-state="open"]',
    ),
  )
}
