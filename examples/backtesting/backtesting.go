package backtesting

import (
	"context"

	"github.com/ezquant/azbot/azbot"
	"github.com/ezquant/azbot/azbot/exchange"
	"github.com/ezquant/azbot/azbot/plot"
	"github.com/ezquant/azbot/azbot/plot/indicator"
	"github.com/ezquant/azbot/azbot/storage"
	"github.com/ezquant/azbot/azbot/tools/log"
	"github.com/ezquant/azbot/examples/strategies"
)

// RunBacktesting runs the backtesting logic
func RunBacktesting() error {
	ctx := context.Background()

	// bot settings (eg: pairs, telegram, etc)
	settings := azbot.Settings{
		Pairs: []string{
			"BTCUSDT",
			"ETHUSDT",
		},
	}

	// initialize your strategy
	strategy := new(strategies.CrossEMA)

	// load historical data from CSV files
	csvFeed, err := exchange.NewCSVFeed(
		strategy.Timeframe(),
		exchange.PairFeed{
			Pair:      "BTCUSDT",
			File:      "testdata/btc-1h.csv",
			Timeframe: "1h",
		},
		exchange.PairFeed{
			Pair:      "ETHUSDT",
			File:      "testdata/eth-1h.csv",
			Timeframe: "1h",
		},
	)
	if err != nil {
		return err
	}

	// initialize a database in memory
	storage, err := storage.FromMemory()
	if err != nil {
		return err
	}

	// create a paper wallet for simulation, initializing with 10.000 USDT
	wallet := exchange.NewPaperWallet(
		ctx,
		"USDT",
		exchange.WithPaperAsset("USDT", 10000),
		exchange.WithDataFeed(csvFeed),
	)

	// create a chart with indicators from the strategy and a custom additional RSI indicator
	chart, err := plot.NewChart(
		plot.WithStrategyIndicators(strategy),
		plot.WithCustomIndicators(
			indicator.RSI(14, "purple"),
		),
		plot.WithPaperWallet(wallet),
	)
	if err != nil {
		return err
	}

	// initialize Azbot with the objects created before
	bot, err := azbot.NewBot(
		ctx,
		settings,
		wallet,
		strategy,
		azbot.WithBacktest(wallet), // Required for Backtest mode
		azbot.WithStorage(storage),

		// connect bot feed (candle and orders) to the chart
		azbot.WithCandleSubscription(chart),
		azbot.WithOrderSubscription(chart),
		azbot.WithLogLevel(log.WarnLevel),
	)
	if err != nil {
		return err
	}

	// Initialize simulation
	err = bot.Run(ctx)
	if err != nil {
		return err
	}

	// Print bot results
	bot.Summary()

	// Display candlesticks chart in local browser
	return chart.Start()
}
