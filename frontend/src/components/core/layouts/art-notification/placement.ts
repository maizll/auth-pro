export interface Box {
  top: number
  right: number
  bottom: number
  left: number
}

export interface PanelPlacement {
  top: number
  left: number
  width: number
  maxHeight: number
}

const MARGIN = 8
const GAP = 6
const PREFERRED_WIDTH = 360
const PREFERRED_MAX_HEIGHT = 480
const MIN_HEIGHT = 80

function clamp(value: number, min: number, max: number) {
  if (max < min) return min
  return Math.min(Math.max(value, min), max)
}

function intersect(a: Box, b: Box): Box {
  return {
    top: Math.max(a.top, b.top),
    right: Math.min(a.right, b.right),
    bottom: Math.min(a.bottom, b.bottom),
    left: Math.max(a.left, b.left)
  }
}

function inset(box: Box, margin: number): Box {
  return {
    top: box.top + margin,
    right: box.right - margin,
    bottom: box.bottom - margin,
    left: box.left + margin
  }
}

/**
 * 把通知面板固定在铃铛按钮旁，并限制在视口与站点主区域的交集内。
 * 优先贴在按钮正下方、右缘与按钮对齐；下方不够再翻到按钮上方。
 */
export function placeNotificationPanel(
  anchor: Box | null,
  viewport: Box,
  frame?: Box | null
): PanelPlacement {
  const bounds = inset(intersect(viewport, frame ?? viewport), MARGIN)
  const availableWidth = Math.max(0, bounds.right - bounds.left)
  const width = Math.min(PREFERRED_WIDTH, availableWidth)
  const minLeft = bounds.left
  const maxLeft = Math.max(bounds.left, bounds.right - width)

  if (!anchor) {
    const maxHeight = Math.max(
      MIN_HEIGHT,
      Math.min(PREFERRED_MAX_HEIGHT, bounds.bottom - bounds.top)
    )
    return { top: bounds.top, left: maxLeft, width, maxHeight }
  }

  const left = clamp(anchor.right - width, minLeft, maxLeft)
  const belowTop = anchor.bottom + GAP
  const spaceBelow = bounds.bottom - belowTop
  const spaceAbove = anchor.top - GAP - bounds.top

  let top = belowTop
  let maxHeight = Math.min(PREFERRED_MAX_HEIGHT, spaceBelow)
  if (spaceBelow < 160 && spaceAbove > spaceBelow) {
    maxHeight = Math.min(PREFERRED_MAX_HEIGHT, spaceAbove)
    top = anchor.top - GAP - maxHeight
  }

  top = clamp(top, bounds.top, Math.max(bounds.top, bounds.bottom - MIN_HEIGHT))
  maxHeight = Math.max(MIN_HEIGHT, Math.min(maxHeight, bounds.bottom - top))
  return { top, left, width, maxHeight }
}
