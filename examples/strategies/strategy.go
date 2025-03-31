// filepath: internal/strategies/strategy.go
package strategies

import (
	"github.com/ezquant/azbot/azbot/model"
	"github.com/ezquant/azbot/azbot/plus/models"
	"github.com/ezquant/azbot/azbot/service"
	"github.com/ezquant/azbot/azbot/strategy"
)

// Strategy 是策略接口，所有策略必须实现这个接口
type Strategy interface {
	Timeframe() string
	WarmupPeriod() int
	GetD() *models.StrategyData
	Indicators(df *model.Dataframe) []strategy.ChartIndicator
	OnCandle(df *model.Dataframe, broker service.Broker)
}
