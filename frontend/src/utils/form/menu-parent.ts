export interface MenuTreeNode {
  id: number
  title?: string
  name?: string
  children?: MenuTreeNode[]
}

export interface MenuParentOption {
  id: number
  label: string
  children?: MenuParentOption[]
}

export function findMenuNode(nodes: MenuTreeNode[] | undefined, id: number): MenuTreeNode | undefined {
  if (!nodes?.length || !id) return undefined
  for (const node of nodes) {
    if (node.id === id) return node
    const child = findMenuNode(node.children, id)
    if (child) return child
  }
  return undefined
}

export function collectSelfAndDescendantIds(nodes: MenuTreeNode[], targetId: number): Set<number> {
  const ids = new Set<number>()
  const found = findMenuNode(nodes, targetId)
  if (!found) return ids
  const walk = (node: MenuTreeNode) => {
    ids.add(node.id)
    node.children?.forEach(walk)
  }
  walk(found)
  return ids
}

export function isInvalidMenuParent(
  menuId: number,
  parentId: number,
  menus: MenuTreeNode[]
): boolean {
  if (!parentId) return false
  if (parentId === menuId) return true
  return collectSelfAndDescendantIds(menus, menuId).has(parentId)
}

export function buildParentMenuOptions(
  menus: MenuTreeNode[],
  editingId = 0,
  labelOf?: (node: MenuTreeNode) => string
): MenuParentOption[] {
  const blocked = editingId ? collectSelfAndDescendantIds(menus, editingId) : new Set<number>()
  const label = labelOf ?? ((node) => node.title || node.name || String(node.id))
  const mapNodes = (nodes: MenuTreeNode[]): MenuParentOption[] =>
    nodes.flatMap((node) => {
      if (blocked.has(node.id)) return []
      const children = node.children?.length ? mapNodes(node.children) : []
      return [
        {
          id: node.id,
          label: label(node),
          children: children.length ? children : undefined
        }
      ]
    })
  return [{ id: 0, label: '无（顶级）', children: mapNodes(menus) }]
}
