# Sentinel 配置与部署说明

## 1. 配置文件建议

建议区分 3 份配置：

- `config.example.toml`：仓库内模板，可提交
- `config.local.toml`：本地开发配置，不提交
- `config.prod.toml`：服务器生产配置，不提交

配置优先级说明：

- 基础设施配置（MySQL、Redis、Telegram、Proxy）来自 TOML
- 账户列表、规则阈值运行时仍优先从 MySQL 读取

## 2. 本地开发

首次准备：

1. 复制模板
2. 生成本地配置
3. 填入本地数据库、Redis、Telegram、代理参数

建议命名：

- `cp config.example.toml config.local.toml`

本地运行：

- `go run ./cmd/risk-sentinel --config config.local.toml --dry-run`

## 3. 服务器生产

服务器上建议单独创建：

- `config.prod.toml`

内容与本地不同的通常是：

- `[system].env`
- `[system].http_proxy`
- `[database]`
- `[clickhouse]`
- `[redis]`
- `[kafka]`
- `[telegram]`

建议：

- 本地配置连接本地中间件
- 生产配置连接服务器内网地址或云数据库地址
- 不把真实密码、Token、Chat ID 提交到 Git

## 4. 部署方式

### 方式 A：服务器拉代码

适合持续迭代。

服务器操作顺序：

1. `git pull`
2. 确认 `config.prod.toml` 已存在
3. `go build -o risk-sentinel ./cmd/risk-sentinel`
4. `./risk-sentinel --config config.prod.toml`

### 方式 B：本地编译后二进制上传

适合服务器不想装完整 Go 环境。

本地：

1. `go build -o risk-sentinel ./cmd/risk-sentinel`
2. 上传 `risk-sentinel`
3. 上传 `config.prod.toml`

服务器：

1. `chmod +x risk-sentinel`
2. `./risk-sentinel --config config.prod.toml`

## 5. 推荐运行方式

生产环境不要直接前台跑，建议用 `systemd` 守护。

示例：

```ini
[Unit]
Description=Risk Sentinel
After=network.target

[Service]
WorkingDirectory=/opt/cex-risk
ExecStart=/opt/cex-risk/risk-sentinel --config /opt/cex-risk/config.prod.toml
Restart=always
RestartSec=5
User=root

[Install]
WantedBy=multi-user.target
```

## 6. 推荐目录结构

服务器上建议：

```text
/opt/cex-risk/
  risk-sentinel
  config.prod.toml
  logs/
```

## 7. 安全建议

如果当前 `config.toml` 里已经有真实密码或 Token：

1. 逐步迁移到 `config.local.toml` / `config.prod.toml`
2. 后续仓库内只保留模板配置
3. 已暴露的敏感信息建议尽快轮换