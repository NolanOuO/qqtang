# 探险秒杀助手（Adventure Assist）

本地测试/GM 辅助功能：**进入探险地图后，自动击败该地图的全部怪物与 BOSS**。
默认关闭，只影响你自己架设的本地服务器。

> 适用范围：QQTang-Local 本地怀旧服。请勿将其用于你不拥有、不运维的服务器。

## 效果

- 每张探险地图载入（`GAME_BEGIN` / `GAME_NEXTMAP`）约 2 秒后，服务端按
  `configs/adventure-rules.json` 的怪物编组逐个结算该地图全部怪物实例，
  小怪在前、BOSS 在最后。
- 客户端会收到与正常击杀相同的 `0x0FA7 NOTIFY_PLAYER_DIE` 推送，
  怪物依次倒下、掉落与结算照常进行。
- 勇气值按怪物配置正常计入全队；非终图仍需走传送门进下一图，
  终图清怪后走正常的 10 秒拾取宽限再胜利结算。

## 开启方式

### 方式一：配置文件（重启生效）

```json
"adventure_assist": {
  "enabled": true,
  "delay_ms": 2000
}
```

- `enabled`：总开关，默认 `false`。
- `delay_ms`：进图后等待毫秒数再清图（0–30000，`0` 或省略表示默认 2000）。
  等待是为了让客户端先消费完地图数据并完成战场初始化。
- `disable_visual_deaths`（可选）：`true` 时只做服务端计数与勇气结算，
  不推送逐个死亡动画（协议实验用）。

### 方式二：GM 网页（即时生效）

打开 GM 工具（默认 `http://127.0.0.1:18100/gm/`），右上角「秒杀助手」
可随时开启/关闭，以及「立即清图」（马上清空所有进行中探险房间的当前地图，
不受开关与延迟限制）。

### 方式三：命令行（即时生效）

```bash
go run ./cmd/qqt-adventure-assist --action status
go run ./cmd/qqt-adventure-assist --action on
go run ./cmd/qqt-adventure-assist --action off
go run ./cmd/qqt-adventure-assist --action trigger
```

默认连接 `127.0.0.1:18100`，可用 `--gm host:port` 指定；
远程 GM 需要 `--username` 与 `--password-stdin`。

GM HTTP 接口（与网页/命令行共用）：

- `GET /gm/api/adventure-assist` → `{enabled, delay_ms, disable_visual_deaths, pending}`
- `POST /gm/api/adventure-assist` with `{"enabled": true|false}`
- `POST /gm/api/adventure-assist/trigger` → `{triggered: n}`

## 实现原理

- **服务端权威**：复用正式击杀路径 `Battle.RecordNPCDeath`，
  不修改客户端、不伪造伤害，勇气/掉落/结算与手动通关一致。
- **单飞调度**：每场战斗（稳定 GameID）同时最多一个待触发计时器；
  进下一图会取代上一图的计时器，战斗结束/结算会取消计时器，
  计时器触发时还会再次校验开关、地图与战斗状态。
- **不断分**：触发前若玩家已手动击杀部分怪物，助手按序号跳过已击杀数，
  不会重复计分。
- **终图复用正式胜利流程**：`BeginFinalStageVictory` +
  10 秒拾取宽限 + `GAME_OVER` 结算。

相关代码：

- `internal/game/adventureassist/plan.go` — 按怪物编组展开确定性击杀顺序
- `internal/server/probe/adventure_assist.go` — 调度/取消/执行/终图胜利
- `internal/server/probe/match_start_dispatch.go`、
  `internal/server/probe/adventure_next_map_dispatch.go` — 进图挂钩
- `internal/server/probe/adventure_battles.go` — 战斗移除时取消计时器
- `internal/server/gm/adventure_assist.go` — GM 控制桥
- `cmd/qqt-adventure-assist/main.go` — 命令行工具

## 验证

```bash
go build ./...
go vet ./...
go test ./internal/game/adventureassist/ ./internal/game/match/ ./internal/server/probe/ ./internal/server/gm/
```

本地联调：开启助手后进任意探险图，约 2 秒后全图怪物依次倒下，
小地图/结算中击杀数与勇气正常增长；终图等待 10 秒宽限后正常胜利结算。
服务端日志中可搜索 `adventure_assist_stage_cleared` 确认每次清图。
