package strategies

import (
	"github.com/ezquant/azbot/azbot"
	"github.com/ezquant/azbot/azbot/indicator"
	"github.com/ezquant/azbot/azbot/service"
	"github.com/ezquant/azbot/azbot/strategy"
	"github.com/ezquant/azbot/azbot/tools/log"
)

// https://www.investopedia.com/articles/trading/08/turtle-trading.asp
type Turtle struct{}

func (e Turtle) Timeframe() string {
	return "4h"
}

func (e Turtle) WarmupPeriod() int {
	return 40
}

func (e Turtle) Indicators(df *azbot.Dataframe) []strategy.ChartIndicator {
	df.Metadata["max40"] = indicator.Max(df.Close, 40)
	df.Metadata["low20"] = indicator.Min(df.Close, 20)

	return nil
}

func (e *Turtle) OnCandle(df *azbot.Dataframe, broker service.Broker) {
	closePrice := df.Close.Last(0)
	highest := df.Metadata["max40"].Last(0)
	lowest := df.Metadata["low20"].Last(0)

	assetPosition, quotePosition, err := broker.Position(df.Pair)
	if err != nil {
		log.Error(err)
		return
	}

	// If position already open wait till it will be closed
	if assetPosition == 0 && closePrice >= highest {
		_, err := broker.CreateOrderMarketQuote(azbot.SideTypeBuy, df.Pair, quotePosition/2)
		if err != nil {
			log.Error(err)
		}
		return
	}

	if assetPosition > 0 && closePrice <= lowest {
		_, err := broker.CreateOrderMarket(azbot.SideTypeSell, df.Pair, assetPosition)
		if err != nil {
			log.Error(err)
		}
	}
}
