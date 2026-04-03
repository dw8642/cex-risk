# 告警受众分类与通知路由设计

## 0. 文档说明

本文档定义告警受众分类体系和通知路由规则，作为规则库 V3.0 的补充设计。

**核心变更：**
- 重新定义 L1/L2/L3 告警级别语义
- 引入四类告警受众（Audience）
- 建立级别包含关系：L3 ⊃ L2 ⊃ L1
- 告警受众在 Dashboard 中可配置

---

## 1. 告警级别重新定义

### 1.1 级别语义

| 级别 | 通道 | 语义 | 行为 |
|------|------|------|------|
| **L1** | Dashboard 展示 | 信息性预警，日常巡检用 | 仅写入 Dashboard 告警面板，不推送外部通知 |
| **L2** | Dashboard + Telegram 文本 | 需关注的风险事件 | L1 行为 + 向配置的受众群发送 Telegram 文本消息 |
| **L3** | Dashboard + Telegram 文本 + Telegram 语音 | 紧急风险事件，需立即响应 | L2 行为 + 向配置的受众发起 Telegram 语音通话 |

### 1.2 级别包含关系

```
L3 触发时执行:
  ├── L1 动作: 写入 Dashboard 告警面板
  ├── L2 动作: 发送 Telegram 文本消息到受众群
  └── L3 动作: 发起 Telegram 语音通话到受众

L2 触发时执行:
  ├── L1 动作: 写入 Dashboard 告警面板
  └── L2 动作: 发送 Telegram 文本消息到受众群

L1 触发时执行:
  └── L1 动作: 写入 Dashboard 告警面板
```

### 1.3 分级告警级别处理

部分规则配置为 "L1/L2 分级" 或 "L2/L3 分级"，含义：

- **L1/L2 分级**：低阈值触发 L1，高阈值触发 L2
- **L2/L3 分级**：低阈值触发 L2，高阈值触发 L3
- **L1/L2/L3 分级**：三级阈值递进触发

每个级别独立路由到各自的受众配置。同一规则在不同级别可以有不同的受众。

---

## 2. 告警受众分类

### 2.1 四类受众

| 受众标识 | 名称 | 典型角色 | 关注维度 |
|---------|------|---------|---------|
| **ops** | 系统运维 | DevOps / SRE / 系统管理员 | 系统健康、基础设施、连接状态 |
| **trader** | 交易员 | 策略负责人 / 交易执行团队 | 行为异常、市场变化、执行质量、资金与敞口 |
| **risk** | 风控负责人 | 老板 / 风控主管 | 清算风险、权限安全、合规、重大敞口 |
| **all** | 全员 | 以上所有人 | 致命风险、全局紧急状况 |

### 2.2 Telegram 群组映射

每类受众对应一个 Telegram 群组（或频道），在系统配置中维护：

| 受众 | Telegram 群 | 用途 |
|------|------------|------|
| ops | `@cex_risk_ops` | 系统级告警推送 |
| trader | `@cex_risk_trader` | 交易/市场/敞口相关告警 |
| risk | `@cex_risk_boss` | 权限/合规/清算类告警 |
| all | `@cex_risk_all` | 全员紧急告警（所有人必须在此群） |

> 实际群 ID 在 Dashboard 系统配置页面维护。

### 2.3 受众分派原则

- **系统类规则**（S-Class 为主）→ 默认 `ops`，系统问题由运维团队首先响应
- **行为/市场/敞口类规则**（B/M/E/BASE-Class）→ 默认 `trader`，交易相关问题由交易员首先响应
- **权限/合规/清算类规则**（P/C/L-Class）→ 默认 `risk`，高风险事件需老板/风控直接知晓
- **致命/全局规则**（系统失明 S-004、全面失明相关的 L3）→ 默认 `all`，所有人必须知道

同一规则在不同告警级别可以有不同受众。例如 L-001 在 L2 级别通知 `trader`，在 L3 级别通知 `all`。

---

## 3. 全量规则 × 受众映射表

### 3.1 一级：权限类（P-Class）— 7 条

| 编号 | 名称 | 告警级别 | L1 受众 | L2 受众 | L3 受众 |
|------|------|---------|--------|--------|--------|
| P-001 | 提币权限异常开启 | L2 | — | risk | — |
| P-002 | 非授权交易范围开放 | L1 | risk | — | — |
| P-003 | 高危 API Key 未及时停用 | L2 | — | risk | — |
| P-004 | 多账户权限过度扩散风险 | L1 | risk | — | — |
| **P-005** | 大额/异常提币实时监控 | **L3** | — | — | **all** |
| P-006 | 异常资金划转监控 | L2 | — | risk | — |
| P-007 | API Key IP 白名单变更监控 | L2 | — | risk | — |

> 权限类全部面向 `risk`。**P-005 评审调整为直接 L3**，面向 `all`。

### 3.2 一级：行为类（B-Class）— 12 条

| 编号 | 名称 | 告警级别 | L1 受众 | L2 受众 | L3 受众 |
|------|------|---------|--------|--------|--------|
| B-001 | 异常成交放量 | L1 | trader | — | — |
| B-002 | 单边成交比例异常 | L1 | trader | — | — |
| B-003 | 单笔订单规模异常 | L1 | trader | — | — |
| B-004 | 非授权交易 | L2 | — | risk | — |
| ~~B-005~~ | ~~高成交低净仓变化~~ | — | — | — | — |
| B-006 | 异常滑点/执行质量劣化 | L1/L2 分级 | trader | trader | — |
| **B-007** | 手工单异常 | **L3** | — | — | **risk** |
| B-008 | 对敲/刷量嫌疑 | L1 | trader | — | — |
| B-009 | Wash Trade / Self-Trade | L2 | — | risk | — |
| B-010 | 挂撤单比例（OTR）异常 | L1/L2 分级 | trader | trader | — |
| B-011 | 多账户高相似执行行为 | L1 | trader | — | — |
| B-012 | 单腿暴露（Leg Risk）监控 | L2 | — | trader | — |

> 行为类以 `trader` 为主。**B-007 评审提升为 P0/L3**（手工单异常需语音通话立即通知 `risk`）。

### 3.3 一级：系统与接入类（S-Class）— 14 条

> ⚠️ **评审调整**：所有 S-Class 规则最低 L2，预警对象统一为 `ops`（系统管理员）。S-014 由原 M-002 移入。

| 编号 | 名称 | 告警级别 | L1 受众 | L2 受众 | L3 受众 |
|------|------|---------|--------|--------|--------|
| S-001 | 公共数据订阅断联/延迟 | L2/L3 分级 | — | ops | ops |
| S-002 | 私有数据订阅断联/延迟 | L2/L3 分级 | — | ops | ops |
| S-003 | 数据陈旧/快照过期检测 | L2/L3 分级 | — | ops | ops |
| S-004 | 风控系统整体失明风险 | L3 | — | — | ops |
| S-005 | 风控只读 API 频控使用率 | L2 | — | ops | — |
| S-006 | 数据真相源不一致 | L2 | — | ops | — |
| S-007 | 指标计算延迟过高 | L2 | — | ops | — |
| S-008 | 规则引擎运行异常 | L2 | — | ops | — |
| S-009 | Watchdog 主风控链路异常 | L3 | — | — | ops |
| S-010 | 交易所/Symbol 临时不可交易 | L2/L3 分级 | — | ops | ops |
| S-011 | 群组控制失效 | L2 | — | ops | — |
| S-012 | 同源策略批量异常 | L2 | — | ops | — |
| S-013 | API 契约漂移与静默变更 | L2 | — | ops | — |
| **S-014** | 交易所接口健康恶化（原M-002） | L2 | — | ops | — |

> 系统类全部 `ops`，不再通知其他角色。系统问题由运维首先响应，必要时运维手动升级。

### 3.4 一级：市场与微观结构类（M-Class）— 5 条有效

> ⚠️ **评审调整**：M-002 移至 S-Class（→S-014）；M-004 合并至 E-008。

| 编号 | 名称 | 告警级别 | L1 受众 | L2 受众 | L3 受众 |
|------|------|---------|--------|--------|--------|
| M-001 | 波动率突升 | L1 | trader | — | — |
| ~~M-002~~ | ~~交易所接口健康恶化~~ → S-014 | — | — | — | — |
| ~~M-003~~ | ~~市场异常波动参与度收敛~~ | — | — | — | — |
| ~~M-004~~ | ~~资金费率异常波动~~ → E-008 | — | — | — | — |
| M-005 | 深度骤降 | L1 | trader | — | — |
| M-006 | 点差急剧扩张 | L1 | trader | — | — |
| M-007 | 跨所价格偏离 | L1 | trader | — | — |
| M-008 | 毒性订单流/逆向选择恶化 | L1 | trader | — | — |
| M-009 | 微观结构恶化 | — | — | — | — |

> 市场类全部 L1，面向 `trader`。M-009 降级为 Dashboard 视图。

### 3.5 一级：平台合规类（C-Class）— 4 条

| 编号 | 名称 | 告警级别 | L1 受众 | L2 受众 | L3 受众 |
|------|------|---------|--------|--------|--------|
| C-001 | 多账户关联执行合规风险 | L1 | risk | — | — |
| C-002 | 多账户规避平台限制风险 | L1 | risk | — | — |
| C-003 | 做市资格/激励资格受损 | L1/L2 分级 | trader | risk | — |
| **C-004** | 账户被平台限制/降权 | **L3** | — | — | **risk** |

> **C-004 评审提升为 P0/L3**（账户被限制是严重事件，需语音通话通知 `risk`）。

### 3.6 一级：敞口与对账类（E-Class）— 9 条（7 条 MVP，2 条延后）

> ⚠️ **评审调整**：E-001 改监控变化率；E-005 升 P0/分钟级；E-006 仅现货；E-007/E-009 移出 MVP；E-008 合并 M-004 升 P0 含结算周期 L3。

| 编号 | 名称 | 告警级别 | L1 受众 | L2 受众 | L3 受众 | MVP |
|------|------|---------|--------|--------|--------|-----|
| E-001 | 净 Delta **变化率**异常 | L1/L2 分级 | trader | trader | — | ✅ |
| E-002 | 单市场暴露过大 | L1 | trader | — | — | ✅ |
| E-003 | 对冲缺口扩大 | L1/L2 分级 | trader | trader | — | ⏸️ |
| E-004 | 总绝对敞口上限 | L1 | trader | — | — | ✅ |
| E-005 | 仓位变动速率异常 | L1/L2/L3 分级 | trader | trader | all | ✅ P0 |
| E-006 | 账户可用资金过低（仅现货） | L1/L2 分级 | trader | risk | — | ✅ |
| E-007 | 风险预算综合监控 | L1/L2 分级 | trader | risk | — | ⏸️ 需回测 |
| **E-008** | **资金费率综合监控** | **L1/L2/L3 分级** | trader | trader | **all** | ✅ P0 |
| E-009 | 资金周期对账差异 | L1 | trader | — | — | ⏸️ |

> E-008 合并原 M-004，新增结算周期变更预警维度（直接 L3→all）。

### 3.7 一级：清算与生存类（L-Class）— 5 条

| 编号 | 名称 | 告警级别 | L1 受众 | L2 受众 | L3 受众 |
|------|------|---------|--------|--------|--------|
| L-001 | 维持保证金占比过高 | L2/L3 分级 | — | trader | all |
| L-002 | 爆仓距离过近 | L2/L3 分级 | — | trader | all |
| **L-003** | ADL 风险升高 | **L2** | — | trader | — |
| L-004 | 单边 OI 占比过高 | L1 | trader | — | — |
| L-005 | 联合保证金/抵押物折价双杀 | L1/L2 分级 | trader | risk | — |

> L-001 阈值需对齐交易所实际清算线。**L-003 评审调整为 L2，cooldown 降至小时级**。

### 3.8 二级：执行基础规则库（BASE）— 5 条 ⛔ MVP 之后

> ⚠️ **评审调整**：BASE 规则整体延后至 MVP 之后，与策略引擎同步实施。

| 编号 | 名称 | 告警级别 | L1 受众 | L2 受众 | L3 受众 | MVP |
|------|------|---------|--------|--------|--------|-----|
| BASE-001 | 策略运行时间窗口检查 | L1 | trader | — | — | ⛔ |
| BASE-002 | 单笔订单规模/支出范围检查 | L2 | — | trader | — | ⛔ |
| BASE-003 | 下单价格偏离限制 | L1 | trader | — | — | ⛔ |
| BASE-004 | 下单间隔与节奏检查 | L1 | trader | — | — | ⛔ |
| BASE-005 | 执行账户/IP 分发异常检查 | L1 | trader | — | — | ⛔ |

### 3.9 三级：策略专属规则（STR-策略）— 10 条

| 编号 | 名称 | 告警级别 | L1 受众 | L2 受众 | L3 受众 |
|------|------|---------|--------|--------|--------|
| STG-001 | 策略心跳超时 | L2 | — | ops | — |
| ARB-002 | 套利价差净收益检查 | L1 | trader | — | — |
| LIQ-001 | 挂单跨越中价检查 | L1 | trader | — | — |
| LIQ-002 | 网格结构偏离检查 | L1 | trader | — | — |
| LIQ-003 | 每 tick 最大资本投放控制 | L1 | trader | — | — |
| MAN-001 | 理想价格偏离控制 | L1 | trader | — | — |
| MAN-002 | 操纵策略方向集中度控制 | L1 | trader | — | — |
| ACCDIS-001 | 周期累计成交总额/总量上限 | L2 | — | trader | — |
| ACCDIS-002 | 策略价格带偏离检查 | L1 | trader | — | — |

> STG-001（策略心跳）面向 `ops`（系统层面问题）。其余策略类全部面向 `trader`。

---

## 4. 受众统计汇总

### 4.1 按受众统计（评审后）

| 受众 | 作为 L2 受众 | 作为 L3 受众 | 涉及规则数（去重） |
|------|------------|------------|-----------------|
| ops | 14 | 5 | 14 |
| trader | 12 | 0 | 26 |
| risk | 5 | 3 | 9 |
| all | 0 | 4 | 4 |

### 4.2 L3 语音通话告警规则清单（12 条）

| 编号 | 名称 | L3 受众 | 触发场景 |
|------|------|--------|---------|
| **P-005** | 大额/异常提币实时监控 | all | 异常大额提币，可能被盗 |
| **B-007** | 手工单异常 | risk | 非策略手工操作，需老板确认 |
| **C-004** | 账户被平台限制/降权 | risk | 账户被交易所限制 |
| S-001 | 公共数据订阅断联/延迟 | ops | 多数据类型同时断联 |
| S-002 | 私有数据订阅断联/延迟 | ops | 私有流完全断联 + REST 回退失败 |
| S-003 | 数据陈旧/快照过期检测 | ops | 关键数据维度过期 |
| S-004 | 风控系统整体失明风险 | ops | 关键数据源 ≥ 50% 异常 |
| S-009 | Watchdog 主风控链路异常 | ops | 主风控链路断裂 |
| S-010 | 交易所/Symbol 临时不可交易 | ops | 交易所层面不可交易 |
| E-005 | 仓位变动速率异常 | all | 仓位剧烈变动 |
| **E-008** | 资金费率结算周期变更 | all | 结算周期突变（如 4h→1h） |
| L-001 | 维持保证金占比过高 | all | 濒临强平 |
| L-002 | 爆仓距离过近 | all | 极度接近爆仓价 |

---

## 5. 通知路由架构

### 5.1 通知流水线

```
规则引擎触发 RiskEvent
       │
       ▼
  ┌─────────────┐
  │ 告警级别判定 │  ← 基于 trigger_value vs threshold(s)
  └──────┬──────┘
         │
         ▼
  ┌──────────────────┐
  │ 级别包含展开     │  L3 → [L1, L2, L3]
  │                  │  L2 → [L1, L2]
  │                  │  L1 → [L1]
  └──────┬───────────┘
         │
         ▼ （对每个展开级别）
  ┌──────────────────┐
  │ 受众路由查询     │  ← rule_code × level → audience
  │  (DB配置优先     │     优先查 DB 配置，无则用默认映射
  │   回退默认映射)  │
  └──────┬───────────┘
         │
         ▼
  ┌──────────────────────────┐
  │ 通知分发                  │
  │  L1: → Dashboard 写入     │
  │  L2: → Telegram 文本      │  → audience → group_id 查询
  │  L3: → Telegram 语音      │  → audience → user_ids 查询
  └──────────────────────────┘
```

### 5.2 Cooldown 去重

每个 `rule_code × scope × audience × level` 组合独立去重。

Redis Key: `cooldown:{rule_code}:{scope_type}:{scope_id}:{audience}:{level}`

这保证同一事件在不同受众和不同级别之间独立冷却，避免：
- L2 触发后 L3 被 cooldown 误吞
- 同一事件通知了 `trader` 群但 `risk` 群被吞

### 5.3 语音通话路由

L3 语音通话需要知道具体的用户 Telegram ID（不只是群 ID）：

```
audience → notification_config 表
         → telegram_voice_user_ids: [user_id_1, user_id_2, ...]
```

语音通话按受众配置的用户列表逐一拨打，支持：
- 顺序拨打（按优先级排序）
- 并行拨打（全部同时呼叫）

MVP 阶段采用**并行拨打**（确保所有人都能收到）。

---

## 6. 数据模型变更

### 6.1 RiskEvent 扩展字段

在现有 `RiskEvent` 契约基础上新增以下字段：

```go
// AlertLevel 实际触发的告警级别: L1 | L2 | L3
AlertLevel string `json:"alert_level"`

// Audience 默认告警受众: ops | trader | risk | all
// 运行时由通知路由模块根据 rule_code × alert_level 查询
// 可被 Dashboard 自定义配置覆盖
Audience string `json:"audience"`
```

### 6.2 新增数据库表

#### alert_audience_config（告警受众配置表）

Dashboard 可配置的受众映射覆盖。当此表有记录时，覆盖默认映射；无记录时回退到代码中的默认映射。

```sql
CREATE TABLE alert_audience_config (
    id           BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
    rule_code    VARCHAR(20)  NOT NULL COMMENT '规则编码，如 P-001',
    alert_level  VARCHAR(5)   NOT NULL COMMENT '告警级别: L1 | L2 | L3',
    audience     VARCHAR(20)  NOT NULL COMMENT '受众标识: ops | trader | risk | all',
    created_at   TIMESTAMP    DEFAULT CURRENT_TIMESTAMP,
    updated_at   TIMESTAMP    DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    UNIQUE KEY uk_rule_level (rule_code, alert_level)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='告警受众配置（Dashboard可配置）';
```

#### notification_channel_config（通知渠道配置表）

```sql
CREATE TABLE notification_channel_config (
    id                      BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
    audience                VARCHAR(20)  NOT NULL COMMENT '受众标识: ops | trader | risk | all',
    telegram_group_id       VARCHAR(50)  NOT NULL COMMENT 'Telegram 群组 ID（L2 文本消息目标）',
    telegram_voice_user_ids JSON         NOT NULL COMMENT 'Telegram 用户 ID 列表（L3 语音通话目标）',
    enabled                 TINYINT(1)   DEFAULT 1 COMMENT '是否启用',
    created_at              TIMESTAMP    DEFAULT CURRENT_TIMESTAMP,
    updated_at              TIMESTAMP    DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    UNIQUE KEY uk_audience (audience)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='通知渠道配置（受众→Telegram映射）';
```

### 6.3 种子数据

```sql
-- 通知渠道初始配置（Telegram 群/用户 ID 需部署时填入实际值）
INSERT INTO notification_channel_config (audience, telegram_group_id, telegram_voice_user_ids) VALUES
('ops',    'TBD_OPS_GROUP_ID',    '["TBD_OPS_USER_1"]'),
('trader', 'TBD_TRADER_GROUP_ID', '["TBD_TRADER_USER_1"]'),
('risk',   'TBD_RISK_GROUP_ID',   '["TBD_RISK_USER_1"]'),
('all',    'TBD_ALL_GROUP_ID',    '["TBD_ALL_USER_1", "TBD_ALL_USER_2"]');
```

---

## 7. Dashboard 受众配置功能

### 7.1 配置页面需求

在 Dashboard「系统配置」中新增「告警受众管理」页面，包含两个子模块：

**模块 A — 规则受众映射配置**

展示所有规则的受众映射，支持覆盖默认值：

| 列 | 说明 |
|----|------|
| 规则编码 | 如 P-001，不可编辑 |
| 规则名称 | 如"提币权限异常开启"，不可编辑 |
| 告警级别 | 该规则可触发的级别（L1/L2/L3） |
| 默认受众 | 代码中硬编码的默认值，灰色显示 |
| 当前受众 | 可编辑下拉，选项: ops / trader / risk / all |
| 状态 | "默认" 或 "已自定义"（有 DB 覆盖时显示） |

操作：
- 修改受众：选择新的受众类型，保存后写入 `alert_audience_config`
- 恢复默认：删除 `alert_audience_config` 中对应记录
- 批量操作：按类别（P/B/S/M/C/E/L/BASE）批量设置

**模块 B — 通知渠道配置**

| 列 | 说明 |
|----|------|
| 受众 | ops / trader / risk / all |
| Telegram 群组 ID | L2 文本消息发送目标 |
| 语音通话用户列表 | L3 语音通话拨打列表，支持增删 |
| 启用状态 | 开关 |

### 7.2 RBAC 权限

告警受众配置属于「系统配置」类操作，需要 `admin` 或 `risk_manager` 角色权限。
- `admin`：可修改所有配置
- `risk_manager`：可修改受众映射，不可修改通知渠道
- `trader` / `viewer`：只读

---

## 8. 告警文案模板更新

在告警文案前缀中加入受众标识，方便在混合群中区分：

```
L1: 【告警】【{audience}】【{rule_code}】{message}
L2: 【严重报警】【{audience}】【{rule_code}】{message}，请尽快处理
L3: 【紧急告警】【{audience}】【{rule_code}】{message}，请立即处理
```

语音通话文本（TTS）：
```
紧急风险告警。规则 {rule_code}，{rule_name}。{message}。请立即处理。
```

---

## 9. 实现路径

### 9.1 MVP 阶段

1. 实现 L1/L2/L3 级别包含逻辑
2. 实现四类受众的 Telegram 群组路由
3. 使用代码内默认映射（本文档 §3 定义）
4. Dashboard 展示受众映射（只读）
5. L3 语音通话基础能力

### 9.2 迭代增强

1. Dashboard 受众映射可编辑（写入 `alert_audience_config`）
2. 通知渠道配置可编辑
3. Cooldown 细粒度化（按受众独立去重）
4. 通知历史记录与统计
5. 告警升级/降级自动路由（连续触发自动提升级别）
