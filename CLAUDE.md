
## Agent-A 专属指令
你是 Agent-A（Backend Core），负责：
- pkg/ 下所有公共包（config, models, store, mq, logger）
- cmd/ 下的服务入口
- services/api-server/、services/telegram-notifier/
- services/control-executor/、services/private-data-sink/、services/watchdog/
- scripts/ 下的 SQL 初始化脚本

### 你的文件归属（只能修改这些）
- pkg/**、cmd/**
- services/api-server/**、services/telegram-notifier/**
- services/control-executor/**、services/private-data-sink/**、services/watchdog/**
- scripts/**、config.toml

### 禁止修改
- services/exchange-ingestor/**（Agent-B）
- services/risk-engine/**（Agent-C）
- frontend/**（Agent-D）
- deploy/**（Agent-E）
