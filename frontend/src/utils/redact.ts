const MARKER = '[REDACTED]'
const SENSITIVE_ASSIGNMENT =
  /\b([a-z0-9_.-]*(?:password|passwd|passphrase|credential|secret|token|cookie|otp|pinentry|authorization|api[_-]?key)[a-z0-9_.-]*)["']?\s*[=:]\s*/i
const NEXT_FIELD = /^["']?[a-zA-Z0-9_.-]+["']?[ \t]*[=:]/

function valueEnd(value: string): number {
  if (value.startsWith(MARKER)) return MARKER.length
  if (!value) return 0
  if (value[0] === '[' || value[0] === '{') return structuredEnd(value)
  const quote = value[0]
  if (quote === '"' || quote === "'") {
    for (let i = 1; i < value.length; i++) {
      if (value[i] === '\\') {
        i++
        continue
      }
      if (value[i] === quote) return i + 1
    }
    return value.length
  }
  for (let i = 0; i < value.length; i++) {
    if ('\r\n&;,)}]<>"\''.includes(value[i]!)) return i
    if ((value[i] === ' ' || value[i] === '\t') && NEXT_FIELD.test(value.slice(i).trimStart())) {
      return i
    }
  }
  return value.length
}

function structuredEnd(value: string): number {
  const stack: string[] = []
  let quote = ''
  for (let i = 0; i < value.length; i++) {
    const char = value[i]!
    if (quote) {
      if (char === '\\') i++
      else if (char === quote) quote = ''
      continue
    }
    if (char === '"' || char === "'") quote = char
    else if (char === '[' || char === '{') stack.push(char)
    else if (char === ']' || char === '}') {
      const opening = stack.pop()
      if ((char === ']' && opening !== '[') || (char === '}' && opening !== '{'))
        return value.length
      if (!stack.length) return i + 1
    }
  }
  return value.length
}

export function redactSensitiveText(text: string): string {
  let line = text.replace(/\r\n/g, '\n')
  let output = ''
  while (true) {
    const match = SENSITIVE_ASSIGNMENT.exec(line)
    if (!match) return output + line
    const start = match.index + match[0].length
    const key = match[1]!.toLowerCase()
    output += line.slice(0, start)
    line = line.slice(start)
    const unquoted = line[0] !== '"' && line[0] !== "'"
    const fullHeader = key.includes('authorization') || key === 'cookie' || key === 'set-cookie'
    let end = valueEnd(line)
    if (unquoted && fullHeader) {
      const newline = line.search(/[\r\n]/)
      end = newline < 0 ? line.length : newline
    }
    output += unquoted ? MARKER : line[0] + MARKER + line[0]
    line = line.slice(end)
  }
}
