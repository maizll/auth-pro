# 1. 模板目录结构

首页包含内置默认模板和可扩展的声明式模板。主项目中的相关文件如下：

```text
frontend/src/views/user-panel/login/
├── DefaultHomeTemplate.vue    # 内置默认模板
├── RemoteHomeTemplate.vue     # 声明式模板共享渲染器
├── home-template.ts           # 模板文档类型、预设白名单和前端校验
└── index.vue                  # 首页入口及默认模板回退逻辑
```

新增可扩展模板时，不新增 Vue 组件，而是在独立模板仓库中增加清单项和 JSON 模板文件。当前独立仓库的实际结构为：

```text
auth-pro-home-templates/
├── .gitattributes
├── index.json
├── index.local.json
├── README.md
├── scripts/
│   └── mock-api.mjs
└── templates/
    ├── cartoon-blue.json
    └── fintech-gold.json
```

- `index.json`：Git 软件源清单。后端固定读取仓库根目录下的该文件。
- `index.local.json`：当前仓库用于 HTTP 静态服务的清单，通过软件源 URL 直接指定；该名称不是后端强制要求。
- `templates/`：当前仓库存放声明式 schema v1 模板 JSON 的目录。
- `scripts/mock-api.mjs`：本地预览辅助脚本，不会被远程模板机制执行。

模板启用后，后端将校验通过的内容保存为运行时文件 `home-templates/<数据库模板 ID>/<SHA256 前 16 位>/template.json`。该目录由系统生成，不属于模板开发目录。

# 2. 文件命名规范

- Git 软件源清单必须命名为 `index.json`，并位于仓库根目录。
- HTTP JSON 软件源没有固定清单文件名；当前仓库使用 `index.local.json`。
- 当前模板文件使用 `<模板 id>.json` 命名，例如 `cartoon-blue.json` 和 `fintech-gold.json`。代码未强制文件名与 `id` 一致，也未强制 `.json` 扩展名；清单中的 `templatePath` 或 `templateUrl` 必须准确指向模板文件。
- 模板 `id` 必须匹配 `^[a-z0-9][a-z0-9-]{1,58}$`：长度为 2～59 个字符，只能包含小写字母、数字和连字符，首字符不能是连字符；同一清单内不得重复。
- Git 清单中的 `templatePath` 必须是仓库内相对路径，不能越过仓库目录，也不能通过符号链接指向仓库外部。当前仓库统一使用 `templates/<模板 id>.json`。
- HTTP 清单中的 `templateUrl` 可以是 HTTP(S) 绝对地址，也可以是相对于清单 URL 的地址。当前仓库统一使用 `templates/<模板 id>.json`。

# 3. 模板插件说明

模板插件在软件源清单的 `homeTemplates` 数组中声明。清单根对象当前实际定义以下字段：

| 字段            | 类型     | 必填/选填 | 当前示例                     | 说明                                                 |
| --------------- | -------- | --------- | ---------------------------- | ---------------------------------------------------- |
| `name`          | `string` | 选填      | `Auth Pro UI Home Templates` | 软件源名称；后端当前不校验是否为空。它不是模板名称。 |
| `plugins`       | `array`  | 选填      | `[]`                         | 普通插件列表；首页模板仓库当前为空数组。             |
| `homeTemplates` | `array`  | 选填      | 包含 2 项                    | 首页模板元数据列表；缺省时按空列表处理。             |

`homeTemplates` 中每一项的实际元数据字段如下：

| 字段            | 类型     | 必填/选填 | 当前示例                                                           | 说明                                                                                                              |
| --------------- | -------- | --------- | ------------------------------------------------------------------ | ----------------------------------------------------------------------------------------------------------------- |
| `id`            | `string` | 必填      | `cartoon-blue`                                                     | 模板唯一标识，必须符合第 2 节的格式，并在同一清单内唯一。                                                         |
| `name`          | `string` | 必填      | `圆趣蓝白红`                                                       | 模板展示名称；应用商店以该字段显示模板名称。                                                                      |
| `description`   | `string` | 选填      | `蓝、白、红为主的圆润活泼首页模板，仅使用原创几何视觉元素。`       | 模板说明；缺省时按空字符串处理。                                                                                  |
| `version`       | `string` | 必填      | `1.0.0`                                                            | 模板版本号；代码只校验非空，未强制语义化版本格式。                                                                |
| `previewUrl`    | `string` | 选填      | 当前未配置                                                         | 模板预览图片地址。后端当前不校验其格式；字段会由接口返回，但当前两份清单未填写，应用商店页面也未渲染该图片。      |
| `schemaVersion` | `number` | 必填      | `1`                                                                | 模板 schema 版本；当前只接受整数 `1`。                                                                            |
| `sha256`        | `string` | 必填      | `b69bd727ac4cfba6aac1b734e6016634bed687158c5b549e7e69de806406a8c3` | 模板文件原始字节的 SHA256，必须是 64 位十六进制字符串。                                                           |
| `templateUrl`   | `string` | 条件必填  | `templates/cartoon-blue.json`                                      | HTTP 模板文件地址。`templateUrl` 与 `templatePath` 至少填写一个。                                                 |
| `templatePath`  | `string` | 条件必填  | `templates/cartoon-blue.json`                                      | Git 仓库内模板相对路径。`templateUrl` 与 `templatePath` 至少填写一个；两者同时存在时后端优先使用 `templatePath`。 |

当前 Git 清单中的实际条目示例：

```json
{
  "id": "cartoon-blue",
  "name": "圆趣蓝白红",
  "description": "蓝、白、红为主的圆润活泼首页模板，仅使用原创几何视觉元素。",
  "version": "1.0.0",
  "schemaVersion": 1,
  "sha256": "b69bd727ac4cfba6aac1b734e6016634bed687158c5b549e7e69de806406a8c3",
  "templatePath": "templates/cartoon-blue.json"
}
```

需求关注的元数据现状如下：

| 信息           | 实际字段        | 当前状态                                                                                                                          |
| -------------- | --------------- | --------------------------------------------------------------------------------------------------------------------------------- |
| 作者信息       | 无              | 清单结构和模板文档均未定义作者字段，当前无法声明作者。                                                                            |
| 图片           | `previewUrl`    | 元数据字段已定义但当前清单未配置；应用商店当前不显示该字段。                                                                      |
| 首页主视觉图片 | `hero.imageUrl` | 这是模板文档字段，不是插件元数据；类型为可选 `string`。相对地址按当前站点解析，解析后的协议必须为 HTTP(S)；当前两份模板均未配置。 |
| 版本号         | `version`       | 已定义且必填，当前两个模板均为 `1.0.0`。                                                                                          |
| 插件名称       | 无              | 首页模板没有独立的插件名称字段；应用商店使用必填字段 `name` 作为模板展示名称。                                                    |
