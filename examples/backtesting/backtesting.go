package backtesting

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"math"
	"os"
	"regexp"
	"strconv"
	"strings"

	"github.com/ezquant/azbot/azbot"
	"github.com/ezquant/azbot/azbot/exchange"
	"github.com/ezquant/azbot/azbot/plot"
	"github.com/ezquant/azbot/azbot/plus/localkv"
	"github.com/ezquant/azbot/azbot/plus/models"
	"github.com/ezquant/azbot/azbot/storage"
	"github.com/ezquant/azbot/examples/strategies"

	log "github.com/sirupsen/logrus"
)

// RunBacktesting runs the backtesting logic

const initialBalance = 10000.0 // 设置初始资金总额

// 用于在goroutine间传递结果的结构体
type summaryResult struct {
	sharpeRatio float64
	err         error
}

// Run executes the backtesting process with the given configuration and database path.
// Parameters:
// - config: The configuration for the backtesting process.
// - databasePath: The path to the database file.

func Run(config *models.Config, databasePath *string) (float64, *plot.Chart) {
	var (
		ctx   = context.Background()
		pairs = make([]string, 0, len(config.AssetWeights))
	)

	for pair := range config.AssetWeights {
		pairs = append(pairs, pair)
	}

	settings := azbot.Settings{
		Pairs: pairs,
	}

	// initialize local KV store for strategies
	kv, err := localkv.NewLocalKV(databasePath)
	if err != nil {
		log.Fatal(err)
	}

	var strategy strategies.Strategy
	switch config.Strategy {
	case "CrossEMA":
		strategy, err = strategies.NewCrossEMA(config, kv)
	default:
		log.Fatalf("Unknown strategy: %s", config.Strategy)
	}

	if err != nil {
		log.Fatal(err)
	}

	pairFeed := make([]exchange.PairFeed, 0, len(config.AssetWeights))

	for pair := range config.AssetWeights {
		pairFeed = append(pairFeed, exchange.PairFeed{
			Pair:      pair,
			File:      fmt.Sprintf("testdata/%s-%s.csv", pair, strategy.Timeframe()),
			Timeframe: strategy.Timeframe(),
		})
	}

	csvFeed, err := exchange.NewCSVFeed(strategy.Timeframe(), pairFeed...)
	if err != nil {
		log.Fatal(err)
	}

	storage, err := storage.FromMemory()
	if err != nil {
		log.Fatal(err)
	}

	wallet := exchange.NewPaperWallet(
		ctx,
		"USDT",
		exchange.WithPaperAsset("USDT", config.BacktestConfig.InitialBalance),
		exchange.WithDataFeed(csvFeed),
	)

	chart, err := plot.NewChart(plot.WithPaperWallet(wallet))
	if err != nil {
		log.Fatal(err)
	}
	bot, err := azbot.NewBot(
		ctx,
		settings,
		wallet,
		strategy,
		azbot.WithBacktest(wallet),
		azbot.WithStorage(storage),
		azbot.WithCandleSubscription(chart),
		azbot.WithOrderSubscription(chart),
		azbot.WithLogLevel(log.WarnLevel),
	)
	if err != nil {
		log.Fatal(err)
	}

	kv.RemoveDB()

	// 重定向错误输出到空设备（关闭回测进度条）
	//oldStderr := os.Stderr
	//devNull, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
	//if err != nil {
	//	log.Fatal(err)
	//}
	//os.Stderr = devNull

	// 运行回测
	if err := bot.Run(ctx); err != nil {
		return 0, chart
	}

	// 恢复错误输出
	//os.Stderr = oldStderr

	// 创建管道用于捕获输出
	r, w, err := os.Pipe()
	if err != nil {
		return 0, chart
	}

	// 保存原始的标准输出
	oldStdout := os.Stdout
	os.Stdout = w

	// 创建一个通道用于同步
	done := make(chan string)

	// 启动 goroutine 读取输出
	go func() {
		var output strings.Builder
		reader := bufio.NewReader(r)
		for {
			line, err := reader.ReadString('\n')
			if err != nil {
				if err != io.EOF {
					log.Printf("读取输出时发生错误: %v", err)
				}
				break
			}
			output.WriteString(line)
		}
		done <- output.String()
	}()

	bot.Summary()

	// 恢复标准输出并关闭写入端
	os.Stdout = oldStdout
	w.Close()

	// 等待读取完成并获取输出
	output := <-done
	r.Close()
	//println("--> output", output)

	// 使用正则表达式解析结果
	startPortfolioRe := regexp.MustCompile(`START PORTFOLIO\s*=\s*([\d.]+)`)
	finalPortfolioRe := regexp.MustCompile(`FINAL PORTFOLIO\s*=\s*([\d.]+)`)
	maxDrawdownRe := regexp.MustCompile(`MAX DRAWDOWN = (-[\d.]+)`)

	matches := startPortfolioRe.FindStringSubmatch(output)
	if matches == nil {
		log.Println("无法解析起始资金数据")
		return 0, chart
	}
	startPortfolio, _ := strconv.ParseFloat(matches[1], 64)

	matches = finalPortfolioRe.FindStringSubmatch(output)
	if matches == nil {
		log.Println("无法解析最终资金数据")
		return 0, chart
	}
	finalPortfolio, _ := strconv.ParseFloat(matches[1], 64)

	matches = maxDrawdownRe.FindStringSubmatch(output)
	if matches == nil {
		log.Println("无法解析最大回撤数据")
		return 0, chart
	}
	maxDrawdown, _ := strconv.ParseFloat(matches[1], 64)

	// 计算夏普率
	riskFreeRate := 0.02
	returns := (finalPortfolio - startPortfolio) / startPortfolio
	volatility := math.Abs(maxDrawdown / 100.0)
	if volatility == 0 {
		volatility = 0.0001 // 避免除以零
	}
	sharpeRatio := (returns - riskFreeRate) / volatility

	// 打印原始输出
	fmt.Print(output)

	var printDetails bool = false
	if printDetails {
		totalEquity := 0.0
		fmt.Printf("REAL ASSETS VALUE\n")

		for pair := range strategy.GetD().AssetWeights {
			asset, _, err := wallet.Position(pair)
			if err != nil {
				log.Fatal(err)
			}

			assetValue := asset * strategy.GetD().LastClose[pair]
			volume := strategy.GetD().Volume[pair]
			profitPerc := (assetValue - volume) / volume * 100
			fmt.Printf("%s = %.2f USDT, Asset Qty = %f, Profit = %.2f%%\n", pair, assetValue, asset, profitPerc)
			totalEquity += assetValue
		}

		totalVolume := 0.0
		for _, volume := range strategy.GetD().Volume {
			totalVolume += volume
		}

		totalProfit := totalEquity - totalVolume
		totalProfitPerc := totalProfit / totalVolume * 100
		fmt.Printf("TOTAL EQUITY = %.2f USDT, Profit = %.2f = %.2f%%\n--------------\n", totalEquity, totalProfit, totalProfitPerc)
	}

	return sharpeRatio, chart
}
