// filepath: internal/strategies/strategy.go
package strategies

import (
	"github.com/ezquant/azbot/azbot/model"
	"github.com/ezquant/azbot/azbot/plus/models"
	"github.com/ezquant/azbot/azbot/service"
	"github.com/ezquant/azbot/azbot/strategy"
)

type Strategy interface {
	Timeframe() string
	WarmupPeriod() int
	GetD() *models.StrategyData
	Indicators(df *model.Dataframe) []strategy.ChartIndicator
	OnCandle(df *model.Dataframe, broker service.Broker)
}
