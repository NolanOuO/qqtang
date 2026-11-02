# 探险掉落助手

探险掉落助手是本地 GM 页面中的只读查询工具，使用服务端已经加载的 `configs/adventure-rules.json`，不另行维护一份掉落表。

## 使用入口

启动本地服务端后打开：

```text
http://127.0.0.1:18100/gm/
```

在页面底部的“探险掉落助手”区域输入道具名称或 ID，并选择全部来源、怪物或隐藏墙。查询只读取当前服务端加载的规则，不会修改账号、库存、掉落配置或游戏状态。

## HTTP 接口

```http
GET /gm/api/adventure/drop-sources?q=草药&kind=all
GET /gm/api/adventure/drop-sources?q=30007&kind=npc
GET /gm/api/adventure/drop-sources?item_id=30007&kind=wall
```

- `q`：道具名称片段或数字 ID。
- `item_id`：可选的精确道具 ID。
- `kind`：`all`、`npc` 或 `wall`。省略时查询全部来源。
- 同时提供 `item_id` 与名称查询时，两者按 AND 关系过滤。

接口只允许 GET。空查询、非法 ID 和未知来源类型返回 HTTP 400；服务端没有加载探险规则时返回 HTTP 503。GM 身份验证沿用现有管理后台认证。

## 概率字段解释

怪物来源展示的是该 NPC 类型的候选库存生成概率。道具权重表示它在该候选库存内部的抽样权重；服务端按 NPC 类型生成候选库存，同类型的怪物实例使用该类型配置。它不是指定道具每次击杀的精确掉落概率，也不代表最终掉落一定会包含该道具。

隐藏墙来源展示地图的最多抽取次数和每次材料池抽取概率。成功抽取后从该地图的材料候选池中选择，不能将每次抽取概率解释成指定材料的掉落率。

资料置信度与备注直接来自规则池元数据。历史资料未确认的怪物逐实例映射不会由助手猜测或补造。查询无结果只表示当前已加载的规则没有匹配项，不代表原版历史版本从未出现该道具。

## 实现位置

- 查询与来源整理：`internal/game/mapdata/adventure_guide.go`
- GM API：`internal/server/gm/server.go`
- GM 页面：`internal/server/gm/web/index.html`、`app.js`、`app.css`
- 测试：`internal/game/mapdata/adventure_guide_test.go`、`internal/server/gm/adventure_guide_test.go`
