# 校验失败说明

## 1. 概述

硬校验（Hard Validation）在解析 ZIP 或登记元数据时执行。响应形如：

```json
{
  "code": 400,
  "msg": "template.json 缺少 kind（必须为 template）",
  "error": { "field": "kind", "rule": "required" }
}
```

`field` 为字段名，`rule` 为规则码，`msg` 为中文说明。按「原因 → 改法」处理，不要绕过校验。

## 2. 目录结构相关失败

| field | rule | 原因 | 改法 |
| --- | --- | --- | --- |
| file | required | 未上传或空文件 | 选择 ZIP |
| file | require_zip | 不是 ZIP | 重新打包 |
| file | max_size | 超过 20 MiB | 压缩或删减资源 |
| file | zip_layout | 穿越 / 符号链接 / 空包 / 仅 __MACOSX | 按打包规范重建 |
| plugin.json / template.json | require_manifest | 找不到对应清单 | 放到根目录或一层子目录 |
| plugin.json / template.json | json / encoding | 非法 JSON 或非 UTF-8 | 另存为 UTF-8 JSON |

## 3. 清单字段失败

| field | rule | 原因 | 改法 |
| --- | --- | --- | --- |
| kind | required | template.json 未写 kind | 增加 `"kind": "template"` |
| kind | mismatch | 清单与 kind 交叉（plugin.json 写 template，或相反） | 清单文件与 kind 对齐 |
| kind | invalid | kind 不是 plugin / template | 按包类型填写 |
| id | required / format | 缺 id 或格式非法 | 2–59 位小写字母、数字、连字符 |
| name / version / description / author | required | 缺必填 | 按字段表补全 |
| schemaVersion | required / format | 模板不是 1 | 写数字 `1` |
| scripts | forbidden | 模板含 scripts | 删除字段 |
| category | kind / format / unknown | 跨 kind 或未配置 | 插件用插件分类；模板用 home-template 或省略以自动填充 |
| sha256 | — | 非 64 位十六进制 | 重算 |
| downloadUrl / templateUrl | — | 协议或路径非法 | 插件必须 HTTPS |

## 4. kind / 分类绑定

- 模板缺 kind：**拒绝**（breaking）。
- 模板缺 category：**自动 `home-template`**，不是错误。
- 自定义插件分类（如 id=`template`、名称「模板」）必须先在源站「管理分类」添加，再用于 plugin.json / 登记。未配置会报「未知分类」。
- 该 extras 出现在商店筛选页签；不要与 `home-template` 混淆。

## 5. 完整示例

错误响应对照：

| msg | 改法 |
| --- | --- |
| `template.json 缺少 kind（必须为 template）` | 写入 kind |
| `该分类属于首页模板，不能用于插件包` | 插件改用 payment/realname/other/extras |
| `该分类属于插件，不能用于首页模板包` | 模板改用 home-template 或省略 |

## 6. 开发者提交流程

校验失败时状态不会前进。先修正 ZIP 或表单，再重新解析 / 保存草稿 / 提交审核。

## 7. 常见错误与排查

优先看 `error.field` + `error.rule`，再对照第 3 节。复制 starter 示例是最快的基线。

## 8. 变更记录

| 日期 | 变更 | 兼容性 |
| --- | --- | --- |
| 2026-09-20 | template.json 缺少 kind 返回 field=kind, rule=required | **Breaking** |
