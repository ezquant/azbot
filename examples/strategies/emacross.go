package strategies

import (
	"github.com/ezquant/azbot/azbot"
	"github.com/ezquant/azbot/azbot/indicator"
	"github.com/ezquant/azbot/azbot/plus/localkv"
	"github.com/ezquant/azbot/azbot/plus/models"
	"github.com/ezquant/azbot/azbot/service"
	"github.com/ezquant/azbot/azbot/strategy"
	"github.com/ezquant/azbot/azbot/tools/log"
)

type CrossEMA struct {
	D              *models.StrategyData
	kv             *localkv.LocalKV
	positionSize   bool
	riskPerTrade   float64
	stopLoss       bool
	stopLossMult   float64
	takeProfit     bool
	takeProfitMult float64
	ema8Period     int
	sma21Period    int
}

// NewCrossEMA is used for backtesting
func NewCrossEMA(config *models.Config, kv *localkv.LocalKV) (*CrossEMA, error) {
	data, err := models.NewStrategyData(config)
	if err != nil {
		return nil, err
	}

	// 从参数列表中获取参数值
	var ema CrossEMA
	ema.D = data
	ema.kv = kv

	for _, param := range config.Parameters {
		switch param.Name {
		case "dynamic_position_size":
			ema.positionSize = param.Default.(bool)
		case "risk_per_trade":
			ema.riskPerTrade = param.Default.(float64)
		case "dynamic_stop_loss":
			ema.stopLoss = param.Default.(bool)
		case "stop_loss_multiplier":
			ema.stopLossMult = param.Default.(float64)
		case "dynamic_take_profit":
			ema.takeProfit = param.Default.(bool)
		case "take_profit_multiplier":
			ema.takeProfitMult = param.Default.(float64)
		case "ema8_period":
			ema.ema8Period = param.Default.(int)
		case "sma21_period":
			ema.sma21Period = param.Default.(int)
		}
	}

	return &ema, nil
}

// Timeframe 返回时间框架字符串。
// 默认时间框架为 "5m"（5分钟）。
// 默认值之前为 "4h"（4小时）。
func (e CrossEMA) Timeframe() string {
	return "5m" // default 4h
}

func (e CrossEMA) WarmupPeriod() int {
	return 60
}

func (d CrossEMA) GetD() *models.StrategyData {
	return d.D
}

func (e CrossEMA) Indicators(df *azbot.Dataframe) []strategy.ChartIndicator {
	// 通过2024年的数据测试，ema8和sma21的参数设置为12和31时，收益最高
	df.Metadata["ema8"] = indicator.EMA(df.Close, e.ema8Period)   // 12, 11, 10, 8
	df.Metadata["sma21"] = indicator.SMA(df.Close, e.sma21Period) // 31, 29, 27, 21

	return []strategy.ChartIndicator{
		{
			Overlay:   true,
			GroupName: "MA's",
			Time:      df.Time,
			Metrics: []strategy.IndicatorMetric{
				{
					Values: df.Metadata["ema8"],
					Name:   "EMA 8",
					Color:  "red",
					Style:  strategy.StyleLine,
				},
				{
					Values: df.Metadata["sma21"],
					Name:   "SMA 21",
					Color:  "blue",
					Style:  strategy.StyleLine,
				},
			},
		},
	}
}

// OnCandle 在每个蜡烛周期调用，用于根据EMA和SMA交叉信号执行交易操作。
// 如果EMA8上穿SMA21且quotePosition大于等于10，则买入资产。
// 如果EMA8下穿SMA21且持有资产，则卖出资产。
// 参数:
// - df: 包含蜡烛数据和技术指标的Dataframe对象
// - broker: 用于执行交易操作的Broker服务
func (e *CrossEMA) OnCandle(df *azbot.Dataframe, broker service.Broker) {
	closePrice := df.Close.Last(0)

	assetPosition, quotePosition, err := broker.Position(df.Pair)
	if err != nil {
		log.Error(err)
		return
	}

	if quotePosition >= 10 && // minimum quote position to trade
		df.Metadata["ema8"].Crossover(df.Metadata["sma21"]) { // trade signal (EMA8 > SMA21)

		// 每次买入可用余额的1/3
		weight := 0.3 // TODO: 优化参数
		amount := quotePosition * weight / closePrice
		_, err := broker.CreateOrderMarket(azbot.SideTypeBuy, df.Pair, amount)
		if err != nil {
			log.Error(err)
		}

		return
	}

	if assetPosition > 0 &&
		df.Metadata["ema8"].Crossunder(df.Metadata["sma21"]) { // trade signal (EMA8 < SMA21)

		_, err = broker.CreateOrderMarket(azbot.SideTypeSell, df.Pair, assetPosition)
		if err != nil {
			log.Error(err)
		}
	}
}

// 量加权Ema，计算方法为：过去（N根价格*量）的总和除以（过去N根量）总和
func VolumeWeightedEMA(df *azbot.Dataframe, period int) []float64 {
	ema := make([]float64, len(df.Close))
	var sumPriceVolume, sumVolume float64

	for i := 0; i < len(df.Close); i++ {
		if i >= period {
			sumPriceVolume -= df.Close[i-period] * df.Volume[i-period]
			sumVolume -= df.Volume[i-period]
		}

		sumPriceVolume += df.Close[i] * df.Volume[i]
		sumVolume += df.Volume[i]

		if i >= period-1 {
			ema[i] = sumPriceVolume / sumVolume
		} else {
			ema[i] = df.Close[i] // 初始值
		}
	}

	return ema
}
