export type CatalogItemAction = 'approve' | 'reject' | 'shelf' | 'unshelf' | 'deprecate'
export type CatalogVersionAction = 'approve' | 'reject' | 'latest' | 'deprecate'

export function catalogItemActions(status: string): CatalogItemAction[] {
  const actions: CatalogItemAction[] = []
  if (status === 'review') {
    actions.push('approve', 'reject')
  }
  if (status === 'draft' || status === 'review' || status === 'approved' || status === 'hidden') {
    actions.push('shelf')
  }
  if (status === 'published') {
    actions.push('unshelf')
  }
  if (status === 'published' || status === 'hidden' || status === 'approved') {
    actions.push('deprecate')
  }
  return actions
}

export function canCatalogItemAction(status: string, action: CatalogItemAction): boolean {
  return catalogItemActions(status).includes(action)
}

export function catalogVersionActions(status: string, isLatest = false): CatalogVersionAction[] {
  const actions: CatalogVersionAction[] = []
  if (status === 'draft' || status === 'pending') {
    actions.push('approve')
  }
  if (status === 'pending') {
    actions.push('reject')
  }
  if (status === 'published' && !isLatest) {
    actions.push('latest')
  }
  if (status === 'published') {
    actions.push('deprecate')
  }
  return actions
}

export function canCatalogVersionAction(
  status: string,
  action: CatalogVersionAction,
  isLatest = false
): boolean {
  return catalogVersionActions(status, isLatest).includes(action)
}

export function isDeprecatedCatalogStatus(status: string): boolean {
  return status === 'deprecated'
}
