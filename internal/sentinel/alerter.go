// alerter.go — Telegram 告警发送器
//
// 通过 Telegram Bot API 发送文本消息，支持 Redis 冷却去重。
// 所有 HTTP 请求通过可配置的 HTTP Proxy 发送。
package sentinel

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

// TelegramAlerter Telegram 告警发送器
type TelegramAlerter struct {
	botToken    string
	chatIDs     []string     // 目标群组/用户 ID
	httpClient  *http.Client // ⚠️ 已配置 Proxy
	redis       *redis.Client
	cooldownTTL time.Duration
	logger      *zap.Logger
}

// NewTelegramAlerter 创建 Telegram 告警器
// proxy: HTTP 代理地址，空字符串=不使用代理
func NewTelegramAlerter(botToken string, chatIDs []string, redisClient *redis.Client,
	cooldownTTL time.Duration, proxy string, logger *zap.Logger) *TelegramAlerter {
	return &TelegramAlerter{
		botToken:    botToken,
		chatIDs:     chatIDs,
		httpClient:  newProxiedHTTPClient(proxy, 10*time.Second), // ⚠️ Proxy
		redis:       redisClient,
		cooldownTTL: cooldownTTL,
		logger:      logger,
	}
}

// UpdateCooldownTTL 更新默认冷却时间（热重载时调用）
func (a *TelegramAlerter) UpdateCooldownTTL(ttl time.Duration) {
	a.cooldownTTL = ttl
}

// Send 发送告警
// 1. 检查 Redis 冷却
// 2. 格式化告警文案
// 3. 调用 Telegram sendMessage API
// 4. 设置冷却 key
// 返回 (true, nil) = 已发送, (false, nil) = 被冷却抑制
func (a *TelegramAlerter) Send(ctx context.Context, alert RiskAlert) (bool, error) {
	// 冷却去重检查
	cooldownKey := alert.CooldownKey
	if cooldownKey == "" {
		cooldownKey = fmt.Sprintf("sentinel:cooldown:%s:%s", alert.RuleCode, alert.AccountID)
	}
	cooldownTTL := a.cooldownTTL
	if alert.CooldownTTLSeconds > 0 {
		cooldownTTL = time.Duration(alert.CooldownTTLSeconds) * time.Second
	}
	if a.redis != nil {
		ok, err := a.redis.SetNX(ctx, cooldownKey, "1", cooldownTTL).Result()
		if err != nil {
			a.logger.Warn("Redis 冷却检查失败（仍发送）", zap.Error(err))
		} else if !ok {
			// key 已存在 → 冷却中
			a.logger.Debug("告警被冷却抑制",
				zap.String("rule", alert.RuleCode),
				zap.String("account", alert.AccountID))
			return false, nil
		}
	}

	// 格式化文案
	text := FormatAlert(alert)

	// 向每个 chatID 发送
	var lastErr error
	for _, chatID := range a.chatIDs {
		if err := a.sendMessage(ctx, chatID, text); err != nil {
			a.logger.Error("Telegram 发送失败",
				zap.String("chat_id", chatID),
				zap.Error(err))
			lastErr = err
		}
	}

	if lastErr != nil {
		return false, fmt.Errorf("telegram 发送部分失败: %w", lastErr)
	}

	a.logger.Info("告警已发送",
		zap.String("rule", alert.RuleCode),
		zap.String("account", alert.AccountLabel),
		zap.String("title", alert.Title))

	return true, nil
}

// sendMessage 调用 Telegram Bot API 发送消息
// POST https://api.telegram.org/bot{token}/sendMessage
func (a *TelegramAlerter) sendMessage(ctx context.Context, chatID, text string) error {
	apiURL := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", a.botToken)

	payload := map[string]string{
		"chat_id":    chatID,
		"text":       text,
		"parse_mode": "HTML",
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", apiURL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := a.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("http post: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("telegram status %d: %s", resp.StatusCode, string(respBody))
	}

	return nil
}

// FormatAlert 格式化告警文本（HTML 格式，Telegram 友好）
func FormatAlert(alert RiskAlert) string {
	emoji, _ := alertLevelDisplay(alert.Level)
	summary := alert.Title
	if summary == "" {
		summary = alert.RuleName
	}

	var parts []string
	// 第一行：emoji + 风控事件标题（直接展示风控内容）
	parts = append(parts,
		fmt.Sprintf("%s <b>%s</b>", emoji, summary),
	)

	// 第二行：规则编号 + 时间
	parts = append(parts,
		fmt.Sprintf("规则：%s | %s",
			alert.RuleCode,
			alert.Timestamp.Format("01-02 15:04:05")),
	)

	// 详情
	if strings.TrimSpace(alert.Message) != "" {
		parts = append(parts, alert.Message)
	}

	return strings.Join(parts, "\n\n")
}

func alertLevelDisplay(level string) (emoji, text string) {
	switch level {
	case "L3":
		return "🔴", "紧急"
	case "L2":
		return "🟠", "重要"
	case "L1":
		return "🟡", "提示"
	default:
		return "🔔", "通知"
	}
}

func alertActionHint(ruleCode string) string {
	switch ruleCode {
	case "P-001":
		return "1. 立即确认该 API Key 是否本应允许提币。\n2. 若不是预期配置，立刻关闭提币权限。\n3. 同时检查是否开启了 IP 白名单限制。"
	case "S-004":
		return "1. 先恢复数据采集链路。\n2. 检查代理、交易所接口、账户权限和网络连通性。\n3. 恢复后补查这段时间是否遗漏风险事件。"
	case "L-001":
		return "1. 立即查看账户保证金是否接近风险线。\n2. 评估是否需要减仓、补充保证金或降低杠杆。\n3. 若是 PM 账户状态异常，优先人工介入。"
	case "E-001":
		return "1. 检查是否刚进行了大额开平仓或对冲切换。\n2. 确认净方向暴露是否符合策略预期。\n3. 若非预期，及时降敞口。"
	case "E-005":
		return "1. 检查最近是否有集中调仓、批量平仓或异常成交。\n2. 确认仓位变化是否来自人工/策略正常操作。\n3. 若非预期，暂停相关策略继续下单。"
	case "E-008c":
		return "1. 关注相关币种资金费成本是否明显上升。\n2. 评估是否需要降低持仓时间或调整做多/做空策略。\n3. 将新的结算周期同步到监控基线。"
	default:
		return ""
	}
}

func formatAlertDetails(details map[string]interface{}) string {
	if len(details) == 0 {
		return ""
	}

	keys := make([]string, 0, len(details))
	for k := range details {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var lines []string
	for _, k := range keys {
		v := details[k]
		label := detailLabel(k)
		value := detailValue(k, v)
		if strings.TrimSpace(value) == "" {
			continue
		}
		if strings.Contains(value, "\n") {
			lines = append(lines, fmt.Sprintf("- %s：\n%s", label, value))
		} else {
			lines = append(lines, fmt.Sprintf("- %s：%s", label, value))
		}
	}

	return strings.Join(lines, "\n")
}

func detailLabel(key string) string {
	labels := map[string]string{
		"account_equity":    "账户权益",
		"account_status":    "账户状态",
		"account_type":      "账户类型",
		"actual_hours":      "实际结算周期(小时)",
		"affected_symbols":  "涉及币种",
		"change_rate":       "变化比例",
		"current_count":     "当前仓位数",
		"current_delta":     "当前净敞口",
		"delta_change":      "净敞口变化金额",
		"enable_withdraw":   "提币权限",
		"error":             "错误原因",
		"expected_hours":    "预期结算周期(小时)",
		"expected_interval": "预期结算周期(毫秒)",
		"ip_restrict":       "IP 白名单限制",
		"liquidation_line":  "参考风险线",
		"maint_margin":      "维持保证金",
		"margin_balance":    "保证金余额",
		"margin_ratio":      "维持保证金占比",
		"prev_count":        "上一轮仓位数",
		"prev_delta":        "上一轮净敞口",
		"summary_lines":     "异常摘要",
		"symbol_count":      "异常币种数量",
		"threshold":         "阈值",
		"total_change":      "仓位变动金额",
		"trigger_reason":    "触发原因",
		"uniMMR":            "uniMMR",
	}
	if label, ok := labels[key]; ok {
		return label
	}
	return key
}

func detailValue(key string, value interface{}) string {
	switch v := value.(type) {
	case bool:
		if v {
			return "是"
		}
		return "否"
	case string:
		return v
	case int:
		return fmt.Sprintf("%d", v)
	case int64:
		return fmt.Sprintf("%d", v)
	case float64:
		switch key {
		case "change_rate", "margin_ratio":
			return fmt.Sprintf("%.1f%%", v*100)
		case "account_equity", "maint_margin", "margin_balance", "current_delta", "prev_delta", "delta_change", "total_change", "threshold":
			return fmt.Sprintf("$%.2f", v)
		case "uniMMR", "liquidation_line":
			return fmt.Sprintf("%.4f", v)
		case "actual_hours", "expected_hours":
			return fmt.Sprintf("%.0f 小时", v)
		default:
			return fmt.Sprintf("%.2f", v)
		}
	case []string:
		return strings.Join(v, ", ")
	default:
		return fmt.Sprintf("%v", value)
	}
}
