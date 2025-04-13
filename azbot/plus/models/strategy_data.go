package models

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path"
	"strings"
	"sync"
	"time"

	"github.com/go-resty/resty/v2"
	log "github.com/sirupsen/logrus"
)

const (
	USDSymbol       = "USDT"
	coinNotFoundErr = "coin symbol \"%s\" not found"
	noCoinsListErr  = "no coins list in the JSON response"
	cacheDir        = "user_data/cache"
	cacheFile       = "coingecko_coins.json"
	cacheTimeFile   = "coingecko_cache_time.txt"
)

var coningeckoSlugs = map[string]string{
	"ADA":  "cardano",
	"AVAX": "avalanche-2",
	"BNB":  "binancecoin",
	"BTC":  "bitcoin",
	"DOGE": "dogecoin",
	"DOT":  "polkadot",
	"ETH":  "ethereum",
	"HBAR": "hedera-hashgraph",
	"LINK": "chainlink",
	"SOL":  "solana",
	"SUI":  "sui",
	"TON":  "the-open-network",
	"TRX":  "tron",
	"UNI":  "uniswap",
	"XLM":  "stellar",
	"XRP":  "ripple",
}

var (
	cacheMutex sync.Mutex
)

func init() {
	// 确保缓存目录存在
	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		panic(fmt.Sprintf("创建缓存目录失败: %v", err))
	}
}

type StrategyData struct {
	//MinimumBalance    float64 // for DCAOnSteroids
	//ExpectedPriceDrop float64 // for DCAOnSteroids
	AssetWeights map[string]float64
	LastClose    map[string]float64
	LastHigh     map[string]float64
	AssetStake   map[string]float64
	Volume       map[string]float64
	ATHTest      map[string]float64
	Slugs        map[string]string
}

func NewStrategyData(config *Config) (*StrategyData, error) {
	slugs, err := getSlugs(config.AssetWeights)
	if err != nil {
		return nil, err
	}

	return &StrategyData{
		//MinimumBalance:    config.MinimumBalance,
		//ExpectedPriceDrop: config.ExpectedPriceDrop,
		AssetWeights: config.AssetWeights,
		LastClose:    make(map[string]float64),
		LastHigh:     make(map[string]float64),
		AssetStake:   make(map[string]float64),
		Volume:       make(map[string]float64),
		// Last ATH
		/*ATHTest: map[string]float64{
			"BTCUSDT":  68972.0,
			"ADAUSDT":  3.1016,
			"ETHUSDT":  4886.0,
			"SOLUSDT":  259.0,
			"BNBUSDT":  692.2,
			"XRPUSDT":  1.9706,
			"DOTUSDT":  55.0,
			"UNIUSDT":  44.357,
			"AVAXUSDT": 146.76,
			"LINKUSDT": 53.08,
			"TRXUSDT":  0.1803,
			"TONUSDT":  10.0,
			"HBARUSDT": 0.57512,
			"XLMUSDT":  0.797,
			"SUIUSDT":  10.0,
		},*/
		ATHTest: map[string]float64{
			"BTCUSDT":  0.0,
			"ADAUSDT":  0.0,
			"ETHUSDT":  0.0,
			"SOLUSDT":  0.0,
			"BNBUSDT":  0.0,
			"XRPUSDT":  0.0,
			"DOTUSDT":  0.0,
			"UNIUSDT":  0.0,
			"AVAXUSDT": 0.0,
			"LINKUSDT": 0.0,
			"TRXUSDT":  0.0,
			"TONUSDT":  0.0,
			"HBARUSDT": 0.0,
			"XLMUSDT":  0.0,
			"SUIUSDT":  0.0,
		},
		Slugs: slugs,
	}, nil
}

// Get the slug (coin ID) values for each of the assets in the map
func getSlugs(assetWeights map[string]float64) (map[string]string, error) {
	cacheMutex.Lock()
	defer cacheMutex.Unlock()

	// 检查缓存是否过期
	needUpdate := true
	timeFilePath := path.Join(cacheDir, cacheTimeFile)
	if timeData, err := os.ReadFile(timeFilePath); err == nil {
		if cacheTime, err := time.Parse(time.RFC3339, string(timeData)); err == nil {
			if time.Since(cacheTime) <= time.Minute*5 {
				needUpdate = false
			}
		}
		log.Info("需要更新 coins list 缓存: ", needUpdate, "，上次更新时间: ", string(timeData))
	}

	if needUpdate {
		resp, err := resty.New().R().
			Get("https://api.coingecko.com/api/v3/coins/list")
		if err != nil {
			return nil, err
		}
		var jsonResp []map[string]string
		if err := json.Unmarshal(resp.Body(), &jsonResp); err != nil {
			return nil, err
		}
		if len(jsonResp) == 0 {
			return nil, errors.New(noCoinsListErr)
		}

		// 更新缓存
		cacheData, err := json.Marshal(jsonResp)
		if err != nil {
			return nil, err
		}

		// 保存缓存数据
		cacheFilePath := path.Join(cacheDir, cacheFile)
		if err := os.WriteFile(cacheFilePath, cacheData, 0644); err != nil {
			return nil, fmt.Errorf("写入缓存文件失败: %v", err)
		}

		// 保存缓存时间
		timeStr := time.Now().Format(time.RFC3339)
		if err := os.WriteFile(timeFilePath, []byte(timeStr), 0644); err != nil {
			return nil, fmt.Errorf("写入缓存时间文件失败: %v", err)
		}
	}

	// 从缓存文件读取数据
	var coinsData []map[string]string
	cacheFilePath := path.Join(cacheDir, cacheFile)
	if cacheData, err := os.ReadFile(cacheFilePath); err == nil {
		if err := json.Unmarshal(cacheData, &coinsData); err != nil {
			return nil, fmt.Errorf("解析缓存数据失败: %v", err)
		}
	} else {
		return nil, fmt.Errorf("读取缓存文件失败: %v", err)
	}

	slugs := make(map[string]string)
	for pair := range assetWeights {
		symbol := strings.Split(pair, USDSymbol)[0]
		found := false

		// 首先检查预定义的映射
		if slug, ok := coningeckoSlugs[symbol]; ok {
			slugs[pair] = slug
			found = true
			continue
		}

		// 如果预定义映射中没有，则在API返回的数据中查找
		for _, coin := range coinsData {
			if coin["symbol"] == strings.ToLower(symbol) {
				slugs[pair] = coin["id"]
				found = true
				break
			}
		}

		if !found {
			return nil, fmt.Errorf(coinNotFoundErr, symbol)
		}
	}

	return slugs, nil
}
