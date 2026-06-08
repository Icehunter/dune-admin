// copyText copies to the clipboard. navigator.clipboard only exists in a secure
// context (HTTPS or localhost); dune-admin is commonly served over plain HTTP on
// a LAN IP, where it's undefined — so fall back to a hidden textarea +
// document.execCommand('copy'), which works in insecure contexts.
export async function copyText(text: string): Promise<boolean> {
  if (window.isSecureContext && navigator.clipboard?.writeText) {
    try {
      await navigator.clipboard.writeText(text)
      return true
    }
    catch { /* fall through to the legacy path */ }
  }
  try {
    const ta = document.createElement('textarea')
    ta.value = text
    ta.setAttribute('readonly', '')
    ta.style.position = 'fixed'
    ta.style.top = '-9999px'
    document.body.appendChild(ta)
    ta.select()
    const ok = document.execCommand('copy')
    document.body.removeChild(ta)
    return ok
  }
  catch {
    return false
  }
}
