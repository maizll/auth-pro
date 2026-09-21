const MASK = '••••••••••••••••'

export function secretDisplay(
  value: string | null | undefined,
  visible: boolean,
  emptyText: string
): string {
  const text = String(value ?? '')
  if (!text.trim()) return emptyText
  return visible ? text : MASK
}

export async function copySecretText(
  value: string | null | undefined,
  writeText: (text: string) => Promise<void> = (text) => navigator.clipboard.writeText(text)
): Promise<boolean> {
  const text = String(value ?? '')
  if (!text.trim()) return false
  try {
    await writeText(text)
    return true
  } catch {
    return false
  }
}
