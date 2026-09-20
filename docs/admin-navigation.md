# 管理后台侧栏

侧栏以「系统 → 菜单管理」为唯一来源。

- 构建默认 `VITE_ACCESS_MODE=backend`。登录后前端请求 `GET /api/system/menus`，按返回树渲染侧栏并注册动态路由。
- 改导航（标题、排序、隐藏、父子分组）请在菜单管理里改，刷新后台即可。不要靠改 `frontend/src/router/modules/*` 调整产品信息架构。
- 可用菜单管理改上级；默认应用商店/在线更新为一级菜单。
- `router/modules` 仍用于注册页面组件；frontend 模式只留给本地对照 Art Design Pro 模板演示（`frontend/.env.development` 临时设 `VITE_ACCESS_MODE=frontend`）。
- 已有部署升级后，首次拉取菜单会按 `menus.name` upsert 工作流分组（授权 → 代理 → 源站 → 风控 → 客户服务 → 接入 → 系统），不会重复插行。超级管理员会补齐缺失的 `role_menus`。之后再改标题/排序会保留，不会被每次请求覆盖。
